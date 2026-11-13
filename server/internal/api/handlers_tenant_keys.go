package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/auth"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/store"
)

// Tenant keys (ADR 0025): a customer gets one server key for its tenant and
// names the end user each request is for in X-Memro-User. Each end user is
// its own agent, created on first use, so memories never mix between them.
// The tenant's owners and admins (and platform admins) issue and revoke
// tenant keys (ADR 0026); the keys themselves act only as an end user, with
// the same limits as any API key.

const endUserHeader = "X-Memro-User"

// validExternalID accepts the customer's own user IDs: 1-128 characters of
// letters, digits and . _ - : @.
func validExternalID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-:@", c)) {
			return false
		}
	}
	return true
}

// endUser returns the tenant's end user, creating it on first use. Its agent
// ID is a fresh public key whose private key is discarded, so the agent can
// only be reached through a tenant key.
func (s *Server) endUser(ctx context.Context, tenantID uuid.UUID, externalID string) (*domain.AgentIdentity, error) {
	if a, err := s.store.EndUser(ctx, tenantID, externalID); a != nil || err != nil {
		return a, err
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return s.store.EnsureEndUser(ctx, tenantID, externalID, hex.EncodeToString(pub))
}

// GET /tenant/{id}/api-keys
func (s *Server) listTenantKeys(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	keys, err := s.store.ListTenantKeys(r.Context(), tid)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// POST /tenant/{id}/api-keys {"name": "…"} — the key is returned once and never again.
func (s *Server) createTenantKey(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	_ = decodeJSON(body(r), &req)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Tenant key"
	}
	if utf8.RuneCountInString(name) > 60 {
		httpError(w, http.StatusBadRequest, "name is limited to 60 characters")
		return
	}
	key, prefix, hash, err := auth.NewAPIKey()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to create key")
		return
	}
	id := uuid.New()
	if err := s.store.CreateTenantKey(r.Context(), id, tid, name, prefix, hash); err != nil {
		slog.Error("create tenant key", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to create key")
		return
	}
	resource := id.String() + " for tenant " + tid.String()
	s.audit(r, agentFrom(r), "tenant_key.create", &resource)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "tenant_id": tid, "name": name, "prefix": prefix, "key": key})
}

// DELETE /tenant/{id}/api-keys/{key_id}
func (s *Server) revokeTenantKey(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("key_id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid key ID")
		return
	}
	ok, err = s.store.RevokeTenantKey(r.Context(), id, tid)
	switch {
	case err != nil:
		httpError(w, http.StatusInternalServerError, "Database error")
	case !ok:
		httpError(w, http.StatusNotFound, "Key not found")
	default:
		resource := id.String() + " for tenant " + tid.String()
		s.audit(r, agentFrom(r), "tenant_key.revoke", &resource)
		w.WriteHeader(http.StatusNoContent)
	}
}

// DELETE /end-user — deletes the end user named in X-Memro-User with all its
// memories and files (offboarding, right to erasure). Only a tenant key may.
func (s *Server) deleteEndUser(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if a.EndUser == "" {
		httpError(w, http.StatusForbidden, "Only a tenant key can delete an end user")
		return
	}
	if !s.removeIdentity(w, r, a.ID) {
		return
	}
	resource := a.EndUser
	s.audit(r, a, "end_user.delete", &resource)
	w.WriteHeader(http.StatusNoContent)
}
