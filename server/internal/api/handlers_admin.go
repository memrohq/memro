package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/domain"
)

// The staff console (ADR 0029): Memro-wide figures, every organisation and
// every person, for platform admins with a dashboard session only.

// GET /admin/overview
func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r, agentFrom(r)) {
		return
	}
	o, err := s.store.AdminOverview(r.Context())
	if err != nil {
		slog.Error("admin overview", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// GET /admin/tenants — every organisation with its plan, limits and size.
func (s *Server) adminTenants(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r, agentFrom(r)) {
		return
	}
	tenants, err := s.store.AdminTenants(r.Context())
	if err != nil {
		slog.Error("admin tenants", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, tenants)
}

// GET /admin/users?q= — people, newest first, or matching an email.
func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r, agentFrom(r)) {
		return
	}
	users, err := s.store.AdminUsers(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		slog.Error("admin users", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// PUT /admin/staff/{user_id} grants staff rights; DELETE revokes them (not
// your own, and never the last staff member's).
func (s *Server) setStaff(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !s.requirePlatformAdmin(w, r, a) {
		return
	}
	uid, err := uuid.Parse(r.PathValue("user_id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	grant := r.Method == http.MethodPut
	if !grant && a.UserID != nil && *a.UserID == uid {
		httpError(w, http.StatusConflict, "You can't remove your own staff rights; ask another staff member")
		return
	}
	changed, err := s.store.SetPlatformAdmin(r.Context(), uid, grant, "granted in the staff console")
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !changed && !grant {
		httpError(w, http.StatusConflict, "Not staff, or the last staff member")
		return
	}
	action, resource := "staff.revoke", uid.String()
	if grant {
		action = "staff.grant"
	}
	s.audit(r, a, action, &resource)
	w.WriteHeader(http.StatusNoContent)
}

// adminID parses {id} after checking the caller is staff.
func (s *Server) adminID(w http.ResponseWriter, r *http.Request) (Agent, uuid.UUID, bool) {
	a := agentFrom(r)
	if !s.requirePlatformAdmin(w, r, a) {
		return a, uuid.Nil, false
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid ID")
		return a, uuid.Nil, false
	}
	return a, id, true
}

// GET /admin/users/{id} — everything about one person.
func (s *Server) adminUser(w http.ResponseWriter, r *http.Request) {
	_, id, ok := s.adminID(w, r)
	if !ok {
		return
	}
	d, err := s.store.AdminUserDetail(r.Context(), id)
	switch {
	case err != nil:
		slog.Error("admin user", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
	case d == nil:
		httpError(w, http.StatusNotFound, "No such person")
	default:
		writeJSON(w, http.StatusOK, d)
	}
}

// GET /admin/tenants/{id} — everything about one organisation.
func (s *Server) adminTenant(w http.ResponseWriter, r *http.Request) {
	_, id, ok := s.adminID(w, r)
	if !ok {
		return
	}
	d, err := s.store.AdminTenantDetail(r.Context(), id)
	switch {
	case err != nil:
		slog.Error("admin tenant", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
	case d == nil:
		httpError(w, http.StatusNotFound, "No such organisation")
	default:
		writeJSON(w, http.StatusOK, d)
	}
}

// POST /admin/users/{id}/{action}: suspend {"reason"}, unsuspend, sign-out,
// revoke-keys. DELETE /admin/users/{id} {"confirm": email} deletes the person
// with their own agents' memories and files.
func (s *Server) adminUserAction(w http.ResponseWriter, r *http.Request) {
	a, id, ok := s.adminID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if a.UserID != nil && *a.UserID == id {
		httpError(w, http.StatusConflict, "You can't do this to your own account")
		return
	}
	d, err := s.store.AdminUserDetail(ctx, id)
	if err != nil || d == nil {
		httpError(w, http.StatusNotFound, "No such person")
		return
	}
	action := r.PathValue("action")
	if r.Method == http.MethodDelete {
		action = "delete"
	}
	if d.Staff && (action == "suspend" || action == "delete") {
		httpError(w, http.StatusConflict, "Remove their staff rights first")
		return
	}
	var req struct{ Reason, Confirm string }
	_ = decodeJSON(body(r), &req)
	result := map[string]any{}
	switch action {
	case "suspend":
		_, err = s.store.SetSuspended(ctx, "user", id, true, strings.TrimSpace(req.Reason))
		s.forgetSuspension()
	case "unsuspend":
		_, err = s.store.SetSuspended(ctx, "user", id, false, "")
		s.forgetSuspension()
	case "sign-out":
		result["sessions"], err = s.store.DeleteSessions(ctx, id)
	case "revoke-keys":
		result["keys"], err = s.store.RevokeUserKeys(ctx, id)
	case "delete":
		if !strings.EqualFold(strings.TrimSpace(req.Confirm), d.Email) {
			httpError(w, http.StatusBadRequest, "Type the person's email address to confirm")
			return
		}
		if orgs, err := s.store.SoleOwnerOrgs(ctx, id); err != nil || len(orgs) > 0 {
			httpError(w, http.StatusConflict, "They are the only owner of "+strings.Join(orgs, ", ")+". Make someone else an owner, or delete the organisation, first.")
			return
		}
		var agents []string
		if agents, err = s.store.UserAgents(ctx, id); err == nil {
			for _, ag := range agents {
				if err = s.deleteAgentData(ctx, ag); err != nil {
					break
				}
			}
		}
		if err == nil {
			_, err = s.store.DeleteUser(ctx, id)
		}
		result["agents"] = len(agents)
	default:
		httpError(w, http.StatusNotFound, "Unknown action")
		return
	}
	if err != nil {
		slog.Error("admin user action", "action", action, "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	resource := id.String() + " " + d.Email
	if req.Reason != "" {
		resource += ": " + req.Reason
	}
	s.audit(r, a, "admin.user."+action, &resource)
	writeJSON(w, http.StatusOK, result)
}

// POST /admin/tenants/{id}/{action}: suspend {"reason"}, unsuspend,
// revoke-keys. DELETE /admin/tenants/{id} {"confirm": name} deletes the
// organisation with all its users' memories and files.
func (s *Server) adminTenantAction(w http.ResponseWriter, r *http.Request) {
	a, id, ok := s.adminID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	t, err := s.store.GetTenant(ctx, id)
	if err != nil || t == nil || id == domain.DefaultTenantID {
		httpError(w, http.StatusNotFound, "No such organisation")
		return
	}
	action := r.PathValue("action")
	if r.Method == http.MethodDelete {
		action = "delete"
	}
	var req struct{ Reason, Confirm string }
	_ = decodeJSON(body(r), &req)
	result := map[string]any{}
	switch action {
	case "suspend":
		_, err = s.store.SetSuspended(ctx, "tenant", id, true, strings.TrimSpace(req.Reason))
		s.forgetSuspension()
	case "unsuspend":
		_, err = s.store.SetSuspended(ctx, "tenant", id, false, "")
		s.forgetSuspension()
	case "revoke-keys":
		result["keys"], err = s.store.RevokeTenantKeys(ctx, id)
	case "delete":
		if strings.TrimSpace(req.Confirm) != t.Name {
			httpError(w, http.StatusBadRequest, "Type the organisation's name to confirm")
			return
		}
		if !s.deleteTenantData(ctx, id) {
			httpError(w, http.StatusInternalServerError, "Some of its data couldn't be deleted; try again")
			return
		}
		_, err = s.store.DeleteTenant(ctx, id)
	default:
		httpError(w, http.StatusNotFound, "Unknown action")
		return
	}
	if err != nil {
		slog.Error("admin tenant action", "action", action, "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	resource := id.String() + " " + t.Name
	if req.Reason != "" {
		resource += ": " + req.Reason
	}
	s.audit(r, a, "admin.tenant."+action, &resource)
	writeJSON(w, http.StatusOK, result)
}

// GET /admin/activity?action= — sign-ups, new organisations, queries and the
// audit feed (filtered by action prefix).
func (s *Server) adminActivity(w http.ResponseWriter, r *http.Request) {
	if !s.requirePlatformAdmin(w, r, agentFrom(r)) {
		return
	}
	act, err := s.store.AdminActivity(r.Context(), strings.TrimSpace(r.URL.Query().Get("action")))
	if err != nil {
		slog.Error("admin activity", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, act)
}

// --- Suspension check ---------------------------------------------------------------

// suspensionTTL is how long a request may act on a stale answer; suspending
// clears this instance's cache at once.
const suspensionTTL = 30 * time.Second

type suspension struct {
	reason string // "" when not suspended
	is     bool
	at     time.Time
}

// suspended reports whether the caller's person or organisation is suspended,
// and why.
func (s *Server) suspended(ctx context.Context, a Agent) (bool, string) {
	key := a.TenantID.String()
	if a.UserID != nil {
		key += "/" + a.UserID.String()
	}
	s.suspMu.Lock()
	c, ok := s.susp[key]
	s.suspMu.Unlock()
	if ok && time.Since(c.at) < suspensionTTL {
		return c.is, c.reason
	}
	is, reason, err := s.store.Suspended(ctx, a.UserID, a.TenantID)
	if err != nil {
		slog.Warn("suspension check", "err", err)
		return false, "" // fail open: a database hiccup must not lock everyone out
	}
	s.suspMu.Lock()
	if s.susp == nil || len(s.susp) > 100_000 {
		s.susp = map[string]suspension{}
	}
	s.susp[key] = suspension{reason, is, time.Now()}
	s.suspMu.Unlock()
	return is, reason
}

func (s *Server) forgetSuspension() {
	s.suspMu.Lock()
	s.susp = nil
	s.suspMu.Unlock()
}
