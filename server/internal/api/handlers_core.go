package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/auth"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/internal/extract"
	"github.com/memrohq/memro/server/internal/hooks"
	"github.com/memrohq/memro/server/internal/metrics"
)

const maxContentChars = 10_000

func (s *Server) audit(r *http.Request, a Agent, action string, resourceID *string) {
	ip := s.clientIP(r)
	if err := s.store.Audit(r.Context(), a.ID, a.TenantID, action, resourceID, &ip); err != nil {
		slog.Error("audit log write failed", "action", action, "err", err)
	}
}

// --- Health & stats ---------------------------------------------------------------

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	db, overall := "ok", "ok"
	if err := s.store.Ping(r.Context()); err != nil {
		slog.Error("health check DB probe failed", "err", err)
		db, overall = "error", "degraded"
	}
	vector := "unavailable"
	if db == "ok" && s.store.VectorReady(r.Context()) {
		vector = "ok"
	}
	embeddings := "disabled"
	if s.embed.Enabled() {
		embeddings = "enabled"
	}
	status := http.StatusOK
	if overall != "ok" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]string{"status": overall, "db": db, "vector_store": vector, "embeddings": embeddings})
}

func (s *Server) cacheStats(w http.ResponseWriter, r *http.Request) {
	if s.embed == nil {
		writeJSON(w, http.StatusOK, map[string]any{"query_embedding_cache": map[string]any{
			"size": 0, "max_size": 0, "hits": 0, "misses": 0,
			"hit_rate_percent": "0.00%", "ttl_seconds": 3600,
		}})
		return
	}
	st := s.embed.CacheStats()
	rate := 0.0
	if total := st.Hits + st.Misses; total > 0 {
		rate = float64(st.Hits) / float64(total) * 100
	}
	writeJSON(w, http.StatusOK, map[string]any{"query_embedding_cache": map[string]any{
		"size": st.Size, "max_size": st.MaxSize, "hits": st.Hits, "misses": st.Misses,
		"hit_rate_percent": fmt.Sprintf("%.2f%%", rate), "ttl_seconds": st.TTLSeconds,
	}})
}

// --- Identity -----------------------------------------------------------------------

// identitiesPerHour limits unauthenticated agent registration per client
// address. Registering needs no account (SDK self-owned agents, ADR 0009),
// so this is what stops one address filling the table.
const identitiesPerHour = 20

func (s *Server) createIdentity(w http.ResponseWriter, r *http.Request) {
	if !s.identityLimiter.Allow(s.clientIP(r)) {
		metrics.RateLimitHits.Inc()
		httpError(w, http.StatusTooManyRequests, "Too many agents registered from this address. Try again later.")
		return
	}
	var req struct {
		PublicKey string `json:"public_key"`
		Pow       *struct {
			Timestamp int64  `json:"timestamp"`
			Nonce     string `json:"nonce"`
		} `json:"pow"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := auth.ParsePublicKey(req.PublicKey); err != nil {
		httpError(w, http.StatusBadRequest, "public_key must be a hex-encoded 32-byte ed25519 key (64 hex chars)")
		return
	}
	// Proof-of-work (ADR 0013): a proof that is sent is always checked;
	// registering without one works until IdentityPowRequired is switched on.
	if req.Pow == nil {
		if s.cfg.IdentityPowRequired {
			httpError(w, http.StatusBadRequest, "A proof-of-work is required to register an agent. Update your SDK, or see GET /identity/pow.")
			return
		}
	} else if err := auth.VerifyPow(req.PublicKey, req.Pow.Timestamp, req.Pow.Nonce, s.cfg.IdentityPowBits, time.Now()); err != nil {
		httpError(w, http.StatusBadRequest, "Invalid proof-of-work: "+err.Error()+". See GET /identity/pow.")
		return
	}
	// A public key belongs to at most one agent (ADR 0013): it cannot become
	// a new agent while it is (or was) another agent's key.
	if owner, err := s.store.KeyAgent(r.Context(), req.PublicKey); err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	} else if owner != "" && owner != req.PublicKey {
		httpError(w, http.StatusConflict, "This key belongs to another agent")
		return
	}
	if err := s.store.CreateIdentity(r.Context(), req.PublicKey, nil, domain.DefaultTenantID); err != nil {
		slog.Error("create identity", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to create identity")
		return
	}
	metrics.IdentitiesCreated.Inc()
	s.audit(r, Agent{ID: req.PublicKey, TenantID: domain.DefaultTenantID}, "identity.create", nil)
	writeJSON(w, http.StatusOK, map[string]string{"agent_id": req.PublicKey})
}

// GET /identity/pow is public: what a registration proof must look like, so
// SDKs follow the server's difficulty instead of hard-coding it.
func (s *Server) identityPow(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"algorithm":       "sha256",
		"message":         auth.PowPrefix + ":<public_key>:<timestamp>:<nonce>",
		"difficulty":      s.cfg.IdentityPowBits,
		"required":        s.cfg.IdentityPowRequired,
		"max_age_seconds": int(auth.PowMaxAge / time.Second),
		"nonce":           "1-64 letters or digits",
	})
}

// GET /identity/{id} is public: it says whether an agent is registered. Who
// owns it and its tenant are not shown (ADR 0012); an agent sees its own
// through /auth/whoami.
func (s *Server) getIdentity(w http.ResponseWriter, r *http.Request) {
	id, err := s.store.GetIdentity(r.Context(), r.PathValue("id"))
	switch {
	case err != nil:
		slog.Error("get identity", "err", err)
		httpError(w, http.StatusInternalServerError, "Internal error")
	case id == nil:
		httpError(w, http.StatusNotFound, "Identity not found")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"id": id.ID, "created_at": id.CreatedAt})
	}
}

func (s *Server) deleteIdentity(w http.ResponseWriter, r *http.Request) {
	a, id := agentFrom(r), r.PathValue("id")
	if a.ID != id {
		httpError(w, http.StatusForbidden, "Agent ID mismatch")
		return
	}
	// A key leaked from a tool's config must not be able to destroy the agent
	// (ADR 0012): deleting takes a dashboard session or the agent's signature.
	if a.Via == "api_key" {
		httpError(w, http.StatusForbidden, "API keys cannot delete an agent. Sign in to the dashboard, or sign the request with the agent's private key.")
		return
	}
	if err := s.store.DeleteIdentity(r.Context(), id); err != nil {
		slog.Error("delete identity", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to delete identity")
		return
	}
	// Files are not tied to the identity by a foreign key (memories, which
	// reference them, are gone now); remove them and their bytes on disk so
	// /assets stops serving them.
	paths, err := s.store.DeleteFilesForAgent(r.Context(), id)
	if err != nil {
		slog.Error("delete files of deleted identity", "agent_id", id, "err", err)
	}
	for _, p := range paths {
		if err := s.files.Remove(p); err != nil {
			slog.Warn("remove file", "err", err)
		}
	}
	s.engine.Cache().InvalidateAgent(id)
	s.audit(r, a, "identity.delete", &id)
	w.WriteHeader(http.StatusNoContent)
}

// --- Memories -----------------------------------------------------------------------

type createMemoryRequest struct {
	AgentID    string          `json:"agent_id"`
	Content    string          `json:"content"`
	MemoryType *string         `json:"memory_type"`
	Visibility *string         `json:"visibility"`
	EventDate  *string         `json:"event_date"`
	SessionID  *string         `json:"session_id"`
	IsAtomic   *bool           `json:"is_atomic"`
	Metadata   json.RawMessage `json:"metadata"`
}

// isAutoType reports a memory_type that asks the server to choose.
func isAutoType(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto", "unknown":
		return true
	}
	return false
}

func (s *Server) createMemory(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	var req createMemoryRequest
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.AgentID != "" && req.AgentID != a.ID {
		httpError(w, http.StatusForbidden, "agent_id does not match authenticated identity")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		httpError(w, http.StatusBadRequest, "content cannot be empty")
		return
	}
	if utf8.RuneCountInString(req.Content) > maxContentChars {
		httpError(w, http.StatusBadRequest, "content exceeds 10,000 character limit")
		return
	}

	// A valid type from the caller always wins; the classifier only fills in
	// when none was given.
	memType := domain.MemoryType("")
	if req.MemoryType == nil || isAutoType(*req.MemoryType) {
		memType = engine.Classify(req.Content)[0].Type
	} else if t, ok := domain.ParseMemoryType(*req.MemoryType); ok {
		memType = t
	} else {
		httpError(w, http.StatusBadRequest, fmt.Sprintf("Invalid memory_type: '%s'", *req.MemoryType))
		return
	}
	vis := domain.Private
	if req.Visibility != nil && *req.Visibility != "" {
		v, ok := domain.ParseVisibility(*req.Visibility)
		if !ok {
			httpError(w, http.StatusBadRequest, fmt.Sprintf("Invalid visibility: '%s'", *req.Visibility))
			return
		}
		vis = v
	}

	now := time.Now().UTC()
	m := &domain.Memory{
		ID: uuid.New(), AgentID: a.ID, TenantID: a.TenantID, Content: req.Content,
		MemoryType: memType, Visibility: vis, CreatedAt: now, DocumentDate: now, LastAccessedAt: now,
		Confidence: 1, EntityTags: extract.Entities(req.Content), RelationshipTypes: []string{},
		Metadata: req.Metadata,
	}
	if req.EventDate != nil {
		if t, err := time.Parse(time.RFC3339, *req.EventDate); err == nil {
			t = t.UTC()
			m.EventDate = &t
		}
	} else {
		m.EventDate = extract.EventDate(req.Content, now)
	}
	if req.SessionID != nil {
		if sid, err := uuid.Parse(*req.SessionID); err == nil {
			m.SessionID = &sid
		}
	}
	if req.IsAtomic != nil {
		m.IsAtomic = *req.IsAtomic
	}
	if !s.saveMemory(w, r, a, m, "memory.create") {
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// saveMemory enforces the soft cap, stores m, queues it for embedding and
// fires side effects. It writes the error response and returns false on failure.
func (s *Server) saveMemory(w http.ResponseWriter, r *http.Request, a Agent, m *domain.Memory, action string) bool {
	ctx := r.Context()
	if !s.allowWrite(w, r, a, 1) {
		return false
	}
	if !s.gate(w, r, a, extension.MetricStoredTokens, extension.EstimateTokens(m.Content)) {
		return false
	}
	if m.EntityTags == nil {
		m.EntityTags = extract.Entities(m.Content)
	}
	if err := s.store.InsertMemory(ctx, m); err != nil {
		slog.Error("store memory", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to store memory")
		return false
	}
	s.afterMemoryCreated(r, a, m, action)
	return true
}

// allowWrite asks the quota policy whether the agent may add n memories and
// writes the error response when it may not.
func (s *Server) allowWrite(w http.ResponseWriter, r *http.Request, a Agent, n int) bool {
	current, err := s.store.CountMemories(r.Context(), a.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to store memory")
		return false
	}
	if max := s.cfg.UnclaimedMaxMemories; a.UserID == nil && max > 0 && current+n > max && !s.exemptFromUnclaimedCaps(r, a) {
		httpError(w, http.StatusForbidden, fmt.Sprintf("Unclaimed agents can store up to %d memories. Ask your person to claim this agent to lift the limit.", max)+s.unclaimedCapHint(a))
		return false
	}
	if err := s.quota.AllowMemoryWrite(r.Context(), a.TenantID, a.ID, current, n); err != nil {
		if errors.Is(err, extension.ErrQuotaExceeded) {
			httpError(w, http.StatusForbidden, err.Error())
		} else {
			slog.Error("quota check", "err", err)
			httpError(w, http.StatusInternalServerError, "Failed to check quota")
		}
		return false
	}
	return true
}

// exemptFromUnclaimedCaps asks the quota policy, if it implements
// extension.UnclaimedCapExemption, whether the agent skips the unclaimed caps.
func (s *Server) exemptFromUnclaimedCaps(r *http.Request, a Agent) bool {
	ex, ok := s.quota.(extension.UnclaimedCapExemption)
	return ok && ex.ExemptFromUnclaimedCaps(r.Context(), a.TenantID, a.ID)
}

// unclaimedCapHint is UNCLAIMED_CAP_HINT with {agent_id} filled in, for
// example how to buy a plan on Memro Cloud.
func (s *Server) unclaimedCapHint(a Agent) string {
	if s.cfg.UnclaimedCapHint == "" {
		return ""
	}
	return " " + strings.ReplaceAll(s.cfg.UnclaimedCapHint, "{agent_id}", a.ID)
}

// gate asks the quota policy, if it implements extension.UsageGate, whether
// the caller may use amount of metric. It writes the error response (403
// with the policy's message) and returns false when not.
func (s *Server) gate(w http.ResponseWriter, r *http.Request, a Agent, metric string, amount int64) bool {
	if max := s.cfg.UnclaimedMaxFileBytes; metric == extension.MetricIngestBytes && a.UserID == nil && max > 0 && amount > 0 {
		used, err := s.store.FileBytes(r.Context(), a.ID)
		if err != nil {
			httpError(w, http.StatusInternalServerError, "Failed to check usage limits")
			return false
		}
		if used+amount > max && !s.exemptFromUnclaimedCaps(r, a) {
			httpError(w, http.StatusForbidden, fmt.Sprintf("Unclaimed agents can store up to %d MB of files. Ask your person to claim this agent to lift the limit.", max/1_000_000)+s.unclaimedCapHint(a))
			return false
		}
	}
	g, ok := s.quota.(extension.UsageGate)
	if !ok || amount <= 0 {
		return true
	}
	if err := g.AllowUsage(r.Context(), a.TenantID, a.ID, metric, amount); err != nil {
		if errors.Is(err, extension.ErrQuotaExceeded) {
			httpError(w, http.StatusForbidden, err.Error())
		} else {
			slog.Error("usage gate", "metric", metric, "err", err)
			httpError(w, http.StatusInternalServerError, "Failed to check usage limits")
		}
		return false
	}
	return true
}

func (s *Server) recordUsage(r *http.Request, a Agent, metric string, value int64) {
	if err := s.meter.Record(r.Context(), extension.UsageEvent{TenantID: a.TenantID, AgentID: a.ID, Metric: metric, Value: value}); err != nil {
		slog.Warn("record usage", "metric", metric, "err", err)
	}
}

func (s *Server) afterMemoryCreated(r *http.Request, a Agent, m *domain.Memory, action string) {
	s.indexer.Enqueue(m.ID, m.AgentID, m.TenantID, m.Content)
	s.engine.Cache().InvalidateAgent(a.ID)
	metrics.MemoriesCreated.Inc()
	id := m.ID.String()
	s.audit(r, a, action, &id)
	s.recordUsage(r, a, extension.MetricMemoriesStored, 1)
	s.recordUsage(r, a, extension.MetricStoredTokens, extension.EstimateTokens(m.Content))
	if urls, err := s.store.GetHooks(r.Context(), a.ID, "memory_created"); err == nil {
		hooks.Notify(urls, "memory_created", map[string]any{
			"event": "memory_created", "memory_id": m.ID, "agent_id": m.AgentID,
			"content": m.Content, "memory_type": m.MemoryType,
		})
	}
}

func (s *Server) listMemories(w http.ResponseWriter, r *http.Request) {
	a, agentID := agentFrom(r), r.PathValue("agent_id")
	if a.ID != agentID {
		httpError(w, http.StatusForbidden, "Agent ID mismatch")
		return
	}
	q := r.URL.Query()
	limit := 100
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = min(v, 100)
	}
	mems, err := s.store.ListMemories(r.Context(), agentID, a.TenantID, q.Get("memory_type"), q.Get("visibility"), limit)
	if err != nil {
		slog.Error("list memories", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to retrieve memories")
		return
	}
	writeJSON(w, http.StatusOK, mems)
}

func (s *Server) deleteMemory(w http.ResponseWriter, r *http.Request) {
	a, raw := agentFrom(r), r.PathValue("id")
	id, err := uuid.Parse(raw)
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid memory ID format")
		return
	}
	ok, fileID, err := s.store.DeleteMemory(r.Context(), id, a.ID, a.TenantID)
	if ok && fileID != nil {
		// The file goes with the last memory made from it.
		if path, err := s.store.DeleteOrphanFile(r.Context(), *fileID, a.ID); err != nil {
			slog.Warn("delete orphan file", "err", err)
		} else if path != "" {
			if err := s.files.Remove(path); err != nil {
				slog.Warn("remove file", "err", err)
			}
		}
	}
	switch {
	case err != nil:
		slog.Error("delete memory", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to delete memory")
	case !ok:
		httpError(w, http.StatusNotFound, "Memory not found")
	default:
		s.engine.Cache().InvalidateAgent(a.ID)
		metrics.MemoriesDeleted.Inc()
		s.audit(r, a, "memory.delete", &raw)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	a, agentID := agentFrom(r), r.PathValue("agent_id")
	if a.ID != agentID {
		httpError(w, http.StatusForbidden, "Agent ID mismatch")
		return
	}
	mems, err := s.store.AllMemories(r.Context(), agentID, a.TenantID)
	if err != nil {
		slog.Error("export", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to export data")
		return
	}
	s.audit(r, a, "data.export", nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"agent_id": agentID, "exported_at": time.Now().UTC().Format(time.RFC3339Nano),
		"total": len(mems), "memories": mems,
	})
}

// --- Usage ----------------------------------------------------------------------------

func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	a, ctx := agentFrom(r), r.Context()
	counts, err := s.store.ActionCountsToday(ctx, a.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nodes, err := s.store.CountMemories(ctx, a.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	edges, err := s.store.CountRelationsCreatedBy(ctx, a.ID, "")
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var warning *string
	limit := s.quota.SoftLimit(ctx, a.TenantID, a.ID)
	if limit > 0 && nodes > limit*8/10 {
		warning = strPtr(fmt.Sprintf("Approaching memory soft cap (%d). Consider pruning.", limit))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reason_calls": counts["mee.reason"], "deep_reason_calls": 0, "evolve_calls": counts["mee.evolve"],
		"memory_nodes": nodes, "memory_limit": limit, "graph_edges": edges, "warning": warning,
	})
}

// --- Tenants ----------------------------------------------------------------------------

type tenantResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Domain    *string `json:"domain"`
	CreatedAt string  `json:"created_at"`
}

func toTenantResponse(t *domain.Tenant) tenantResponse {
	return tenantResponse{ID: t.ID.String(), Name: t.Name, Domain: t.Domain, CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339Nano)}
}

// maxTenantUsageHours bounds GET /tenant/{id}/usage to 90 days.
const maxTenantUsageHours = 90 * 24

// requirePlatformAdmin allows Memro staff acting through a dashboard session
// (ADR 0012). API keys and signatures never carry admin power.
func (s *Server) requirePlatformAdmin(w http.ResponseWriter, r *http.Request, a Agent) bool {
	if a.Via != "session" || a.UserID == nil {
		httpError(w, http.StatusForbidden, "Platform admin access requires a dashboard session")
		return false
	}
	ok, err := s.store.IsPlatformAdmin(r.Context(), *a.UserID)
	if err != nil {
		slog.Error("platform admin check", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return false
	}
	if !ok {
		httpError(w, http.StatusForbidden, "Platform admin access required")
		return false
	}
	return true
}

// canReadTenant allows platform admins, and members of a dedicated tenant
// reading their own. The shared default tenant holds every self-signup user,
// so belonging to it grants nothing.
func (s *Server) canReadTenant(w http.ResponseWriter, r *http.Request, a Agent, tid uuid.UUID) bool {
	if a.TenantID == tid && tid != domain.DefaultTenantID {
		return true
	}
	return s.requirePlatformAdmin(w, r, a)
}

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r, agentFrom(r)) {
		return
	}
	var req struct {
		Name   string  `json:"name"`
		Domain *string `json:"domain"`
	}
	if err := decodeJSON(body(r), &req); err != nil || req.Name == "" {
		httpError(w, http.StatusBadRequest, "name is required")
		return
	}
	now := time.Now().UTC()
	t := &domain.Tenant{ID: uuid.New(), Name: req.Name, Domain: req.Domain, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateTenant(r.Context(), t); err != nil {
		slog.Error("create tenant", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to create tenant")
		return
	}
	metrics.TenantsCreated.Inc()
	writeJSON(w, http.StatusOK, toTenantResponse(t))
}

func (s *Server) getTenant(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	tid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid tenant ID format")
		return
	}
	if !s.canReadTenant(w, r, a, tid) {
		return
	}
	t, err := s.store.GetTenant(r.Context(), tid)
	switch {
	case err != nil:
		httpError(w, http.StatusInternalServerError, "Failed to get tenant")
	case t == nil:
		httpError(w, http.StatusNotFound, "Tenant not found")
	default:
		writeJSON(w, http.StatusOK, toTenantResponse(t))
	}
}

func (s *Server) tenantUsage(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	tid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid tenant ID format")
		return
	}
	if !s.canReadTenant(w, r, a, tid) {
		return
	}
	hours := 24
	if v, err := strconv.Atoi(r.URL.Query().Get("hours")); err == nil && v > 0 {
		hours = min(v, maxTenantUsageHours)
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	type item struct {
		TenantID    string `json:"tenant_id"`
		Metric      string `json:"metric"`
		Value       int64  `json:"value"`
		PeriodHours int    `json:"period_hours"`
	}
	out := []item{}
	for _, m := range []string{"memories_stored", "memories_searched", "api_calls"} {
		v, err := s.store.TenantUsage(r.Context(), tid, m, since)
		if err != nil {
			slog.Warn("tenant usage", "metric", m, "err", err)
		}
		out = append(out, item{tid.String(), m, v, hours})
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Hooks ------------------------------------------------------------------------------

func (s *Server) registerHook(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	var req struct {
		EventType string `json:"event_type"`
		URL       string `json:"url"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.EventType == "" || len(req.EventType) > 50 {
		httpError(w, http.StatusBadRequest, "event_type is required (max 50 characters)")
		return
	}
	if err := hooks.ValidateURL(req.URL); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.RegisterHook(r.Context(), a.TenantID, a.ID, a.KeyID, req.EventType, req.URL); err != nil {
		slog.Error("register hook", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to register hook")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// DELETE /hooks {"event_type": "…", "url": "…"}
func (s *Server) deleteHook(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	var req struct {
		EventType string `json:"event_type"`
		URL       string `json:"url"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	ok, err := s.store.DeleteHook(r.Context(), a.ID, req.EventType, req.URL)
	switch {
	case err != nil:
		httpError(w, http.StatusInternalServerError, "Failed to delete hook")
	case !ok:
		httpError(w, http.StatusNotFound, "Hook not found")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) getHooks(w http.ResponseWriter, r *http.Request) {
	urls, err := s.store.GetHooks(r.Context(), agentFrom(r).ID, r.PathValue("event_type"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to retrieve hooks")
		return
	}
	writeJSON(w, http.StatusOK, urls)
}
