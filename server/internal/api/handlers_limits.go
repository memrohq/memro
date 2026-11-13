package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/store"
)

// Organisation plans (ADR 0028), in Memro Cloud's public units: stored tokens
// and queries, shared by all of an organisation's users. The public tiers
// (Builder, Pro, Scale) are bought through Memro Cloud's checkout as the
// organisation and enforced by its quota policy, so they need no row here.
// This table holds what isn't sold there: the free trial, with hard limits,
// and Enterprise or custom contracts, whose overage is billed rather than
// refused. Limits replace the server's quota policy for that tenant's agents.

const (
	trialLength   = 30 * 24 * time.Hour
	trialReadOnly = 60 * 24 * time.Hour // after the trial ends, then data is deleted
	warnAtPercent = 80
)

func ptr[T any](v T) *T { return &v }

// planPreset returns a plan's default limits, or false for an unknown plan.
func planPreset(plan string) (store.TenantLimits, bool) {
	switch plan {
	case "trial": // the public Free tier's allowances, for 30 days
		ends := time.Now().UTC().Add(trialLength)
		return store.TenantLimits{Plan: plan, MaxStoredTokensTotal: ptr[int64](100_000), MaxQueriesMonthly: ptr[int64](5_000),
			MaxMembers: ptr(3), TrialEndsAt: &ends}, true
	case "enterprise", "custom":
		return store.TenantLimits{Plan: plan, AllowImages: true}, true
	}
	return store.TenantLimits{}, false
}

// tenantLimits returns an organisation's limits, or nil when the server's
// default policy applies.
func (s *Server) tenantLimits(ctx context.Context, tenantID uuid.UUID) (*store.TenantLimits, error) {
	if tenantID == domain.DefaultTenantID {
		return nil, nil
	}
	return s.store.GetTenantLimits(ctx, tenantID)
}

func trialEnded(l *store.TenantLimits) bool {
	return l.Plan == "trial" && l.TrialEndsAt != nil && time.Now().After(*l.TrialEndsAt)
}

// refuse writes a limit refusal: 403, not 429, because waiting doesn't lift
// it and the SDKs retry 429s.
func refuse(w http.ResponseWriter, kind, msg string) {
	w.Header().Set("Memro-Limit", kind)
	httpError(w, http.StatusForbidden, msg)
}

// allowWriteUnderPlan applies an organisation's plan to a memory write before
// its size is known; set reports whether the tenant has a plan, which then
// replaces the unclaimed cap and the quota policy. Sizes are checked in
// gatePlan.
func (s *Server) allowWriteUnderPlan(w http.ResponseWriter, r *http.Request, a Agent) (ok, set bool) {
	l, err := s.tenantLimits(r.Context(), a.TenantID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to check quota")
		return false, true
	}
	if l == nil {
		return true, false
	}
	if trialEnded(l) {
		refuse(w, "trial_ended", fmt.Sprintf("Your organisation's trial ended on %s. Memories can still be read and deleted until %s; contact Memro to keep saving.",
			l.TrialEndsAt.Format("2 Jan 2006"), l.TrialEndsAt.Add(trialReadOnly).Format("2 Jan 2006")))
		return false, true
	}
	return true, true
}

// gatePlan applies an organisation's plan to amount of metric. set reports
// whether the tenant has a plan, which then replaces the quota policy's gate.
func (s *Server) gatePlan(w http.ResponseWriter, r *http.Request, a Agent, metric string, amount int64) (ok, set bool) {
	ctx := r.Context()
	l, err := s.tenantLimits(ctx, a.TenantID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to check usage limits")
		return false, true
	}
	if l == nil {
		return true, false
	}
	check := func(kind string, max *int64, used func() (int64, error), msg string) bool {
		if max == nil || amount <= 0 {
			return true
		}
		u, err := used()
		if err != nil {
			httpError(w, http.StatusInternalServerError, "Failed to check usage limits")
			return false
		}
		if u+amount > *max {
			refuse(w, kind, fmt.Sprintf(msg, u, *max))
			return false
		}
		if pct := (u + amount) * 100 / *max; pct >= warnAtPercent {
			w.Header().Add("Memro-Limit-Warning", fmt.Sprintf("%s=%d%%", kind, pct))
			s.warnOwners(a.TenantID, kind, pct)
		}
		return true
	}
	switch metric {
	case extension.MetricStoredTokens:
		return check("stored_tokens_per_user", l.MaxStoredTokensPerUser, func() (int64, error) { return s.store.AgentStoredTokens(ctx, a.ID) },
			"Storage limit reached: this user stores %d of the %d tokens your organisation's plan allows per user. Delete old memories or contact Memro to raise the limit.") &&
			check("stored_tokens_total", l.MaxStoredTokensTotal, func() (int64, error) { return s.store.TenantStoredTokens(ctx, a.TenantID) },
				"Storage limit reached: your organisation's users store %d of the %d tokens they share. Delete old memories or contact Memro to raise the limit."), true
	case extension.MetricQueries:
		return check("queries_monthly", l.MaxQueriesMonthly, func() (int64, error) { return s.store.TenantMonthUsage(ctx, a.TenantID, extension.MetricQueries) },
			"Query limit reached: your organisation has used %d of %d queries this month. It resets on the 1st (UTC); contact Memro to raise it."), true
	}
	return true, true
}

// allowImages refuses image analysis when the organisation's plan excludes it.
func (s *Server) allowImages(w http.ResponseWriter, r *http.Request, a Agent) bool {
	l, err := s.tenantLimits(r.Context(), a.TenantID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to check usage limits")
		return false
	}
	if l != nil && !l.AllowImages {
		refuse(w, "images", "Image analysis isn't included in your organisation's plan. Contact Memro to add it.")
		return false
	}
	return true
}

// warnOwners emails the organisation's owners the first time a limit passes
// 80%, in the background so the request isn't held up.
func (s *Server) warnOwners(tenantID uuid.UUID, kind string, pct int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		first, err := s.store.MarkWarned(ctx, tenantID)
		if err != nil || !first {
			return
		}
		what := strings.ReplaceAll(kind, "_", " ")
		s.notifyOwners(ctx, tenantID, fmt.Sprintf("Your organisation has used %d%% of its %s limit", pct, what),
			fmt.Sprintf("Your organisation on Memro has used %d%% of its %s limit. Once it's reached, Memro refuses more until the limit is raised. Contact Memro to upgrade.", pct, what))
	}()
}

// notifyOwners emails every owner; without email set up it only logs.
func (s *Server) notifyOwners(ctx context.Context, tenantID uuid.UUID, subject, text string) {
	slog.Info("organisation notice", "tenant", tenantID, "subject", subject)
	if !s.email.Configured() {
		return
	}
	owners, err := s.store.OwnerEmails(ctx, tenantID)
	if err != nil {
		slog.Warn("owner emails", "tenant", tenantID, "err", err)
		return
	}
	for _, to := range owners {
		if err := s.email.SendNotice(to, subject, text, s.cfg.AppURL); err != nil {
			slog.Warn("send organisation notice", "tenant", tenantID, "err", err)
		}
	}
}

// seatsLeft writes 403 and returns false when the organisation has used all
// the seats its plan includes.
func (s *Server) seatsLeft(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) bool {
	l, err := s.tenantLimits(r.Context(), tenantID)
	if err == nil && (l == nil || l.MaxMembers == nil) {
		return true
	}
	n := 0
	if err == nil {
		n, err = s.store.Seats(r.Context(), tenantID)
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return false
	}
	if n >= *l.MaxMembers {
		refuse(w, "members", fmt.Sprintf("Your plan includes %d seats and all are taken by members and pending invitations. Remove someone or contact Memro for more.", *l.MaxMembers))
		return false
	}
	return true
}

// memoryLimitFor is the per-agent memory count /usage reports: none for an
// organisation with a plan (its limits are in tokens), else the quota
// policy's.
func (s *Server) memoryLimitFor(r *http.Request, a Agent) int {
	if l, err := s.tenantLimits(r.Context(), a.TenantID); err == nil && l != nil {
		return 0
	}
	return s.quota.SoftLimit(r.Context(), a.TenantID, a.ID)
}

// GET /tenant/{id}/limits — the plan and how much of it is used. Members and
// staff; limits is null when the server's default policy applies.
func (s *Server) getLimits(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.Roles...)
	if !ok {
		return
	}
	ctx := r.Context()
	l, err := s.store.GetTenantLimits(ctx, tid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	tokens, err1 := s.store.TenantStoredTokens(ctx, tid)
	queries, err2 := s.store.TenantMonthUsage(ctx, tid, extension.MetricQueries)
	seats, err3 := s.store.Seats(ctx, tid)
	if err1 != nil || err2 != nil || err3 != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	res := map[string]any{"limits": l, "stored_tokens": tokens, "queries_this_month": queries, "seats": seats}
	if l != nil && l.Plan == "trial" && l.TrialEndsAt != nil {
		res["data_deleted_at"] = l.TrialEndsAt.Add(trialReadOnly)
	}
	writeJSON(w, http.StatusOK, res)
}

// optional decodes a JSON field that may be absent (keep the preset), null
// (no limit) or a value.
func optional[T any](raw json.RawMessage, preset *T) (*T, error) {
	if len(raw) == 0 {
		return preset, nil
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// PUT /tenant/{id}/limits {"plan", and optionally "max_stored_tokens_per_user",
// "max_stored_tokens_total", "max_queries_monthly", "max_members",
// "allow_images", "trial_ends_at"}: the plan's preset with any field given
// replacing it (null = no limit / no end). DELETE returns the tenant to the
// default policy. Platform admins only.
func (s *Server) setLimits(w http.ResponseWriter, r *http.Request) {
	// Staff only, even when they are also members of this organisation.
	a := agentFrom(r)
	if !s.requirePlatformAdmin(w, r, a) {
		return
	}
	tid, err := uuid.Parse(r.PathValue("id"))
	if err != nil || tid == domain.DefaultTenantID {
		httpError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}
	if t, err := s.store.GetTenant(r.Context(), tid); err != nil || t == nil {
		httpError(w, http.StatusNotFound, "Tenant not found")
		return
	}
	resource := tid.String()
	if r.Method == http.MethodDelete {
		if err := s.store.DeleteTenantLimits(r.Context(), tid); err != nil {
			httpError(w, http.StatusInternalServerError, "Database error")
			return
		}
		s.audit(r, a, "tenant_limits.reset", &resource)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var req struct {
		Plan                   string          `json:"plan"`
		MaxStoredTokensPerUser json.RawMessage `json:"max_stored_tokens_per_user"`
		MaxStoredTokensTotal   json.RawMessage `json:"max_stored_tokens_total"`
		MaxQueriesMonthly      json.RawMessage `json:"max_queries_monthly"`
		MaxMembers             json.RawMessage `json:"max_members"`
		AllowImages            *bool           `json:"allow_images"`
		TrialEndsAt            json.RawMessage `json:"trial_ends_at"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	l, ok := planPreset(req.Plan)
	if !ok {
		httpError(w, http.StatusBadRequest, "plan must be trial, enterprise or custom (Builder, Pro and Scale are bought through checkout)")
		return
	}
	positive := func(name string, raw json.RawMessage, dst **int64) bool {
		v, err := optional(raw, *dst)
		if err != nil || (v != nil && *v <= 0) {
			httpError(w, http.StatusBadRequest, name+" must be a positive integer or null (no limit)")
			return false
		}
		*dst = v
		return true
	}
	if !positive("max_stored_tokens_per_user", req.MaxStoredTokensPerUser, &l.MaxStoredTokensPerUser) ||
		!positive("max_stored_tokens_total", req.MaxStoredTokensTotal, &l.MaxStoredTokensTotal) ||
		!positive("max_queries_monthly", req.MaxQueriesMonthly, &l.MaxQueriesMonthly) {
		return
	}
	if l.MaxMembers, err = optional(req.MaxMembers, l.MaxMembers); err != nil || (l.MaxMembers != nil && *l.MaxMembers <= 0) {
		httpError(w, http.StatusBadRequest, "max_members must be a positive integer or null (no limit)")
		return
	}
	if req.AllowImages != nil {
		l.AllowImages = *req.AllowImages
	}
	if l.TrialEndsAt, err = optional(req.TrialEndsAt, l.TrialEndsAt); err != nil {
		httpError(w, http.StatusBadRequest, "trial_ends_at must be a time like 2026-12-31T00:00:00Z, or null (no end)")
		return
	}
	if err := s.store.SetTenantLimits(r.Context(), tid, l, a.UserID); err != nil {
		slog.Error("set tenant limits", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	saved, _ := s.store.GetTenantLimits(r.Context(), tid)
	if b, err := json.Marshal(saved); err == nil {
		resource += " " + string(b)
	}
	s.audit(r, a, "tenant_limits.set", &resource)
	writeJSON(w, http.StatusOK, map[string]any{"limits": saved})
}

// trialLimits are what a new self-service organisation starts on.
func trialLimits() store.TenantLimits {
	l, _ := planPreset("trial")
	return l
}

// --- Hourly jobs ---------------------------------------------------------------------

// RunHourlyJobs runs the organisation jobs every hour until ctx ends:
// re-checking verified domains (ADR 0027), recording stored tokens for
// billing, and moving ended trials towards deletion (ADR 0028).
func (s *Server) RunHourlyJobs(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.recheckDomains(ctx)
		if err := s.store.SnapshotStoredTokens(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("stored tokens snapshot", "err", err)
		}
		s.advanceTrials(ctx)
	}
}

// Trial notices in the order they are sent, with how long before deletion
// (trial end + 60 days) each is due.
var trialNotices = []struct {
	name   string
	before time.Duration
}{
	{"ended", trialReadOnly},
	{"d14", 14 * 24 * time.Hour},
	{"d7", 7 * 24 * time.Hour},
	{"d1", 24 * time.Hour},
	{"deleted", 0},
}

func noticeRank(n *string) int {
	for i, t := range trialNotices {
		if n != nil && t.name == *n {
			return i
		}
	}
	return -1
}

// advanceTrials emails owners as an ended trial nears deletion and, 60 days
// after the end, deletes every agent in the organisation with its memories
// and files. The organisation, its members and keys stay, so it can still be
// upgraded. Only organisations still on the trial plan are touched.
func (s *Server) advanceTrials(ctx context.Context) {
	trials, err := s.store.EndedTrials(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("ended trials", "err", err)
		}
		return
	}
	now := time.Now()
	for _, t := range trials {
		deleteAt := t.TrialEndsAt.Add(trialReadOnly)
		due := -1 // the latest notice whose time has come; only that one is sent
		for i, n := range trialNotices {
			if !now.Before(deleteAt.Add(-n.before)) {
				due = i
			}
		}
		if due <= noticeRank(t.Notice) {
			continue
		}
		name := trialNotices[due].name
		if name == "deleted" && !s.deleteTenantData(ctx, t.TenantID) {
			continue // retried next hour
		}
		// Recorded before emailing so a crash can't send a notice twice;
		// false means staff changed the trial meanwhile.
		if ok, err := s.store.SetTrialNotice(ctx, t.TenantID, t.TrialEndsAt, name); err != nil || !ok {
			continue
		}
		subject, text := trialEmail(t.Name, name, t.TrialEndsAt, deleteAt)
		s.notifyOwners(ctx, t.TenantID, subject, text)
	}
}

func trialEmail(org, notice string, ended, deleteAt time.Time) (subject, text string) {
	day := func(t time.Time) string { return t.Format("2 Jan 2006") }
	switch notice {
	case "ended":
		return "Your Memro trial has ended", fmt.Sprintf("%s's free trial ended on %s. Your users' memories are now read-only: they can still be searched and deleted, but nothing new is saved. Unless you choose a plan, they will be deleted on %s.", org, day(ended), day(deleteAt))
	case "deleted":
		return "Your Memro trial data was deleted", fmt.Sprintf("%s's trial ended on %s and its memories have now been deleted, as announced. Your organisation, members and keys remain; choose a plan to start again.", org, day(ended))
	}
	return "Your Memro trial data will be deleted soon", fmt.Sprintf("%s's trial ended on %s. Unless you choose a plan, your users' memories will be deleted on %s.", org, day(ended), day(deleteAt))
}

// deleteTenantData deletes every agent in a tenant with its memories and
// files; it reports whether all of them went.
func (s *Server) deleteTenantData(ctx context.Context, tenantID uuid.UUID) bool {
	agents, err := s.store.TenantAgents(ctx, tenantID)
	if err != nil {
		slog.Error("trial deletion: list agents", "tenant", tenantID, "err", err)
		return false
	}
	ok := true
	for _, id := range agents {
		if err := s.deleteAgentData(ctx, id); err != nil {
			slog.Error("trial deletion", "tenant", tenantID, "agent", id, "err", err)
			ok = false
		}
	}
	slog.Info("trial data deleted", "tenant", tenantID, "agents", len(agents), "complete", ok)
	return ok
}
