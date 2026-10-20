package api

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/auth"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/store"
)

// Agent keys (ADR 0013): an agent's identity is separate from the keys that
// may sign for it. Keys can be listed, added (rotation, recovery keys) and
// revoked by the agent itself (a signing or recovery key) or by its owner
// through a dashboard session; API keys never manage keys.

// keyManagementRoutes are the only routes a recovery key may use.
var keyManagementRoutes = map[string]bool{
	"GET /identity/{id}/keys":          true,
	"POST /identity/{id}/keys":         true,
	"DELETE /identity/{id}/keys/{key}": true,
}

// keyProofPayload is what a new key signs to prove its holder is adding it:
// the agent, the key and a timestamp, so a proof cannot be reused for another
// agent or replayed after a few minutes.
func keyProofPayload(agentID, publicKey, ts string) []byte {
	return []byte("memro-agent-key\n" + agentID + "\n" + publicKey + "\n" + ts)
}

// keyActor checks the caller may manage agent id's keys and returns the
// agent, or writes the error and returns nil.
func (s *Server) keyActor(w http.ResponseWriter, r *http.Request) (*domain.AgentIdentity, Agent) {
	a, id := agentFrom(r), r.PathValue("id")
	switch a.Via {
	case "api_key":
		httpError(w, http.StatusForbidden, "API keys cannot manage agent keys")
		return nil, a
	case "signature":
		if a.ID != id {
			httpError(w, http.StatusForbidden, "Agent ID mismatch")
			return nil, a
		}
	}
	identity, err := s.store.GetIdentity(r.Context(), id)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return nil, a
	}
	if identity == nil {
		httpError(w, http.StatusNotFound, "Agent not found")
		return nil, a
	}
	if a.Via == "session" && (identity.UserID == nil || a.UserID == nil || *identity.UserID != *a.UserID) {
		httpError(w, http.StatusForbidden, "Only the agent's owner can manage its keys")
		return nil, a
	}
	return identity, a
}

// keyAudit records a key change against the agent, naming the key and who
// made the change.
func (s *Server) keyAudit(r *http.Request, identity *domain.AgentIdentity, a Agent, action, publicKey string) {
	by := "session"
	if a.Via == "signature" {
		by = a.KeyRole + ":" + a.SigningKey[:16]
	} else if a.UserID != nil {
		by = "session:" + a.UserID.String()
	}
	resource := publicKey + " by " + by
	s.audit(r, Agent{ID: identity.ID, TenantID: identity.TenantID}, action, &resource)
}

// GET /identity/{id}/keys
func (s *Server) listAgentKeys(w http.ResponseWriter, r *http.Request) {
	identity, _ := s.keyActor(w, r)
	if identity == nil {
		return
	}
	keys, err := s.store.ListAgentKeys(r.Context(), identity.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// POST /identity/{id}/keys {"public_key", "role", "proof", "proof_timestamp"}
//
// proof is the new key's hex ed25519 signature over keyProofPayload, made
// within the last five minutes (proof of possession).
func (s *Server) addAgentKey(w http.ResponseWriter, r *http.Request) {
	identity, a := s.keyActor(w, r)
	if identity == nil {
		return
	}
	var req struct {
		PublicKey      string `json:"public_key"`
		Role           string `json:"role"`
		Proof          string `json:"proof"`
		ProofTimestamp string `json:"proof_timestamp"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.PublicKey = strings.ToLower(strings.TrimSpace(req.PublicKey))
	if req.Role == "" {
		req.Role = store.KeySigning
	}
	if req.Role != store.KeySigning && req.Role != store.KeyRecovery {
		httpError(w, http.StatusBadRequest, `role must be "signing" or "recovery"`)
		return
	}
	pub, err := auth.ParsePublicKey(req.PublicKey)
	if err != nil {
		httpError(w, http.StatusBadRequest, "public_key must be a hex-encoded 32-byte ed25519 key (64 hex chars)")
		return
	}
	if err := auth.CheckTimestamp(req.ProofTimestamp, time.Now()); err != nil {
		httpError(w, http.StatusBadRequest, "proof_timestamp is missing, invalid or older than five minutes")
		return
	}
	sig, err := hex.DecodeString(req.Proof)
	if err != nil || !ed25519.Verify(pub, keyProofPayload(identity.ID, req.PublicKey, req.ProofTimestamp), sig) {
		httpError(w, http.StatusBadRequest, "proof must be the new key's signature over the agent ID, key and timestamp")
		return
	}
	switch err := s.store.AddAgentKey(r.Context(), identity.ID, req.PublicKey, req.Role); {
	case errors.Is(err, store.ErrKeyInUse):
		httpError(w, http.StatusConflict, "This key is already in use and cannot be added again")
		return
	case err != nil:
		slog.Error("add agent key", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	action := "agent.key.added"
	if a.Via == "signature" && a.KeyRole == store.KeyRecovery && req.Role == store.KeySigning {
		action = "agent.key.recovered"
	}
	s.keyAudit(r, identity, a, action, req.PublicKey)
	writeJSON(w, http.StatusCreated, map[string]string{"public_key": req.PublicKey, "role": req.Role})
}

// DELETE /identity/{id}/keys/{key}
func (s *Server) revokeAgentKey(w http.ResponseWriter, r *http.Request) {
	identity, a := s.keyActor(w, r)
	if identity == nil {
		return
	}
	key := strings.ToLower(r.PathValue("key"))
	switch err := s.store.RevokeAgentKey(r.Context(), identity.ID, key); {
	case errors.Is(err, store.ErrKeyNotFound):
		httpError(w, http.StatusNotFound, "No active key with that ID")
		return
	case errors.Is(err, store.ErrLastKey):
		httpError(w, http.StatusConflict, "This is the agent's last signing key; add another signing or recovery key first")
		return
	case err != nil:
		slog.Error("revoke agent key", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	s.keyAudit(r, identity, a, "agent.key.revoked", key)
	w.WriteHeader(http.StatusNoContent)
}
