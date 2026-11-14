package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/auth"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/store"
)

// Organisation accounts (ADR 0026): people sign in once and manage the
// dedicated tenants they are members of, with a role each. Members see usage,
// never end users' memories.

const (
	inviteTTL = 7 * 24 * time.Hour
	// platformAdmin is the role tenantAccess reports for Memro staff, who may
	// do anything in any tenant.
	platformAdmin = "platform_admin"
)

// tenantAccess checks a dashboard session may act on tenant {id}: a member
// whose role is in roles, or a platform admin. It returns the tenant and the
// caller's role, or writes the error and returns false.
func (s *Server) tenantAccess(w http.ResponseWriter, r *http.Request, roles ...string) (uuid.UUID, string, bool) {
	a := agentFrom(r)
	tid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid tenant ID format")
		return uuid.Nil, "", false
	}
	// The shared default tenant holds every self-signup user (ADR 0012).
	if tid == domain.DefaultTenantID {
		httpError(w, http.StatusBadRequest, "The default tenant is not an organisation")
		return uuid.Nil, "", false
	}
	if a.Via != "session" || a.UserID == nil {
		httpError(w, http.StatusForbidden, "Sign in to the dashboard to do this")
		return uuid.Nil, "", false
	}
	role, err := s.store.TenantRole(r.Context(), tid, *a.UserID)
	if err == nil && role == "" {
		var admin bool
		if admin, err = s.store.IsPlatformAdmin(r.Context(), *a.UserID); admin {
			role = platformAdmin
		}
	}
	if err != nil {
		slog.Error("tenant access", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return uuid.Nil, "", false
	}
	if role != platformAdmin && !slices.Contains(roles, role) {
		httpError(w, http.StatusForbidden, "Your role in this organisation can't do this")
		return uuid.Nil, "", false
	}
	if role == platformAdmin { // members prove the tenant exists; admins may name any ID
		if t, err := s.store.GetTenant(r.Context(), tid); err != nil || t == nil {
			httpError(w, http.StatusNotFound, "Tenant not found")
			return uuid.Nil, "", false
		}
	}
	return tid, role, true
}

// mayGrant reports whether a caller with role may give or take away target:
// owners (and staff) manage owners; admins manage everyone else.
func mayGrant(role, target string) bool {
	return role == platformAdmin || role == store.RoleOwner || target != store.RoleOwner
}

// GET /orgs — the organisations the signed-in person belongs to.
func (s *Server) listOrgs(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !requireSession(w, a) || a.UserID == nil {
		return
	}
	orgs, err := s.store.Memberships(r.Context(), *a.UserID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

// GET /tenant/{id}/members
func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.Roles...)
	if !ok {
		return
	}
	members, err := s.store.Members(r.Context(), tid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// PATCH /tenant/{id}/members/{user_id} {"role"}; DELETE removes the member.
// Anyone may leave; owners and admins manage others.
func (s *Server) changeMember(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	userID, err := uuid.Parse(r.PathValue("user_id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	leaving := r.Method == http.MethodDelete && a.UserID != nil && *a.UserID == userID
	roles := []string{store.RoleOwner, store.RoleAdmin}
	if leaving {
		roles = store.Roles
	}
	tid, role, ok := s.tenantAccess(w, r, roles...)
	if !ok {
		return
	}
	var newRole string // "" removes
	if r.Method == http.MethodPatch {
		var req struct {
			Role string `json:"role"`
		}
		if err := decodeJSON(body(r), &req); err != nil || !slices.Contains(store.Roles, req.Role) {
			httpError(w, http.StatusBadRequest, "role must be one of owner, admin, billing, viewer")
			return
		}
		newRole = req.Role
	}
	current, err := s.store.TenantRole(r.Context(), tid, userID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !leaving && (!mayGrant(role, current) || !mayGrant(role, newRole)) {
		httpError(w, http.StatusForbidden, "Only an owner can make or change owners")
		return
	}
	found, err := s.store.SetMemberRole(r.Context(), tid, userID, newRole)
	switch {
	case errors.Is(err, store.ErrLastOwner):
		httpError(w, http.StatusConflict, "The organisation needs at least one owner. Make someone else an owner first.")
	case err != nil:
		slog.Error("change member", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
	case !found:
		httpError(w, http.StatusNotFound, "Not a member of this organisation")
	default:
		resource := userID.String() + " in " + tid.String() + " -> " + newRole
		s.audit(r, a, "tenant_member.change", &resource)
		w.WriteHeader(http.StatusNoContent)
	}
}

// createInvite stores an invitation and emails it; the response carries the
// accept link too, so it can be shared by hand when email isn't set up. Only
// the invited address can accept it.
func (s *Server) createInvite(r *http.Request, tid uuid.UUID, email, role string) (map[string]any, error) {
	token, hash, err := auth.NewSessionToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	inv := store.Invite{ID: uuid.New(), TenantID: tid, Email: email, Role: role, CreatedAt: now, ExpiresAt: now.Add(inviteTTL)}
	if err := s.store.CreateInvite(r.Context(), inv, hash, agentFrom(r).UserID); err != nil {
		return nil, err
	}
	link := s.cfg.AppURL + "/invite?token=" + url.QueryEscape(token)
	emailed := false
	if s.email.Configured() {
		t, _ := s.store.GetTenant(r.Context(), tid)
		org := "an organisation"
		if t != nil {
			org = t.Name
		}
		if err := s.email.SendInvite(email, org, role, link); err != nil {
			slog.Warn("send invite", "err", err)
		} else {
			emailed = true
		}
	}
	resource := inv.ID.String() + " " + role + " in " + tid.String()
	s.audit(r, agentFrom(r), "tenant_invite.create", &resource)
	return map[string]any{"invite": inv, "link": link, "emailed": emailed}, nil
}

// inviteEmail normalises an address the way sign-in does, or returns "".
func inviteEmail(raw string) string {
	email := strings.ToLower(strings.TrimSpace(raw))
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 254 {
		return ""
	}
	return email
}

// POST /tenant/{id}/invites {"email", "role"}
func (s *Server) inviteMember(w http.ResponseWriter, r *http.Request) {
	tid, role, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	_ = decodeJSON(body(r), &req)
	email := inviteEmail(req.Email)
	if email == "" {
		httpError(w, http.StatusBadRequest, "A valid email address is required")
		return
	}
	if !slices.Contains(store.Roles, req.Role) {
		httpError(w, http.StatusBadRequest, "role must be one of owner, admin, billing, viewer")
		return
	}
	if !mayGrant(role, req.Role) {
		httpError(w, http.StatusForbidden, "Only an owner can invite owners")
		return
	}
	res, err := s.createInvite(r, tid, email, req.Role)
	if err != nil {
		slog.Error("create invite", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to create invite")
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// GET /tenant/{id}/invites — pending invitations.
func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	invites, err := s.store.Invites(r.Context(), tid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, invites)
}

// DELETE /tenant/{id}/invites/{invite_id}
func (s *Server) cancelInvite(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("invite_id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid invite ID")
		return
	}
	found, err := s.store.DeleteInvite(r.Context(), tid, id)
	switch {
	case err != nil:
		httpError(w, http.StatusInternalServerError, "Database error")
	case !found:
		httpError(w, http.StatusNotFound, "Invite not found")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// POST /invites/accept {"token"} — joins the signed-in person to the
// organisation, if the invitation was sent to their email.
func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !requireSession(w, a) || a.UserID == nil {
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	_ = decodeJSON(body(r), &req)
	email, err := s.store.UserEmail(r.Context(), *a.UserID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	m, err := s.store.AcceptInvite(r.Context(), auth.HashToken(strings.TrimSpace(req.Token)), *a.UserID, email)
	switch {
	case errors.Is(err, store.ErrInviteInvalid):
		httpError(w, http.StatusNotFound, "This invitation is invalid, expired or already used")
	case errors.Is(err, store.ErrInviteEmail):
		httpError(w, http.StatusForbidden, "This invitation was sent to a different email address. Sign in with that address to accept it.")
	case err != nil:
		slog.Error("accept invite", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
	default:
		resource := m.TenantID.String() + " as " + m.Role
		s.audit(r, a, "tenant_invite.accept", &resource)
		writeJSON(w, http.StatusOK, m)
	}
}
