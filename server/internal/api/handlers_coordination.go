package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/memrohq/memro/server/internal/store"
)

// coordinationSession identifies which session of an agent is acting, so two
// tools sharing one agent (Claude Code and Cursor on one account) are
// different holders: the request's session_id, else X-Session-Id, else the
// credential (API key or signing key), else the dashboard.
func coordinationSession(r *http.Request, agent Agent, fromBody string) string {
	if fromBody != "" {
		return fromBody
	}
	if h := r.Header.Get("X-Session-Id"); h != "" {
		return h
	}
	switch {
	case agent.Via == "api_key" && agent.KeyID != nil:
		return "key:" + agent.KeyID.String()
	case agent.Via == "signature" && agent.SigningKey != "":
		return "sig:" + agent.SigningKey
	case agent.Via == "session":
		return "dashboard"
	}
	return store.DefaultCoordinationSession
}

// POST /coordination/claim
func (s *Server) coordinationClaim(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)
	var req struct {
		Resource   string `json:"resource"`
		Intent     string `json:"intent"`
		SessionID  string `json:"session_id"`
		TTLSeconds int    `json:"ttl_seconds"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}
	if req.Resource == "" {
		httpError(w, http.StatusBadRequest, "resource is required")
		return
	}
	sessionID := coordinationSession(r, agent, req.SessionID)
	ttl := time.Duration(req.TTLSeconds) * time.Second

	scopeKey := store.CoordinationScopeKey(agent.TenantID, agent.UserID, agent.ID)
	result, err := s.store.ClaimLease(r.Context(), scopeKey, req.Resource, agent.ID, sessionID, req.Intent, ttl)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to claim lease: "+err.Error())
		return
	}
	s.audit(r, agent, "coordination.claim", &req.Resource)
	writeJSON(w, http.StatusOK, result)
}

// POST /coordination/heartbeat
func (s *Server) coordinationHeartbeat(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)
	var req struct {
		Resource   string     `json:"resource"`
		LeaseID    *uuid.UUID `json:"lease_id"`
		SessionID  string     `json:"session_id"`
		TTLSeconds int        `json:"ttl_seconds"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}
	if req.Resource == "" && req.LeaseID == nil {
		httpError(w, http.StatusBadRequest, "resource or lease_id is required")
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second

	scopeKey := store.CoordinationScopeKey(agent.TenantID, agent.UserID, agent.ID)
	newExpires, err := s.store.HeartbeatLease(r.Context(), scopeKey, agent.ID, coordinationSession(r, agent, req.SessionID), req.LeaseID, req.Resource, ttl)
	if err != nil {
		if errors.Is(err, store.ErrLeaseNotFound) {
			httpError(w, http.StatusNotFound, "Active lease not found or not owned by caller")
			return
		}
		httpError(w, http.StatusInternalServerError, "Failed to extend lease: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"renewed":    true,
		"success":    true,
		"expires_at": newExpires,
	})
}

// POST /coordination/release
func (s *Server) coordinationRelease(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)
	var req struct {
		Resource  string     `json:"resource"`
		LeaseID   *uuid.UUID `json:"lease_id"`
		SessionID string     `json:"session_id"`
		Status    string     `json:"status"` // completed | abandoned
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}
	if req.Resource == "" && req.LeaseID == nil {
		httpError(w, http.StatusBadRequest, "resource or lease_id is required")
		return
	}

	scopeKey := store.CoordinationScopeKey(agent.TenantID, agent.UserID, agent.ID)
	if err := s.store.ReleaseLease(r.Context(), scopeKey, agent.ID, coordinationSession(r, agent, req.SessionID), req.LeaseID, req.Resource, req.Status); err != nil {
		if errors.Is(err, store.ErrLeaseNotFound) {
			httpError(w, http.StatusNotFound, "Active lease not found or not owned by caller")
			return
		}
		httpError(w, http.StatusInternalServerError, "Failed to release lease: "+err.Error())
		return
	}
	s.audit(r, agent, "coordination.release", &req.Resource)
	writeJSON(w, http.StatusOK, map[string]any{
		"released": true,
		"resource": req.Resource,
	})
}

// POST /coordination/handoff
func (s *Server) coordinationHandoff(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)
	var req struct {
		Resource    string     `json:"resource"`
		LeaseID     *uuid.UUID `json:"lease_id"`
		SessionID   string     `json:"session_id"`
		Summary     string     `json:"summary"`
		NextAgentID string     `json:"next_agent_id"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}
	if req.Resource == "" && req.LeaseID == nil {
		httpError(w, http.StatusBadRequest, "resource or lease_id is required")
		return
	}
	if req.Summary == "" {
		httpError(w, http.StatusBadRequest, "summary is required for handoff")
		return
	}

	scopeKey := store.CoordinationScopeKey(agent.TenantID, agent.UserID, agent.ID)
	if err := s.store.HandoffLease(r.Context(), scopeKey, agent.ID, coordinationSession(r, agent, req.SessionID), req.LeaseID, req.Resource, req.Summary, req.NextAgentID); err != nil {
		if errors.Is(err, store.ErrLeaseHeldByOther) {
			httpError(w, http.StatusConflict, "Resource is held by another agent or session")
			return
		}
		if errors.Is(err, store.ErrLeaseNotFound) {
			httpError(w, http.StatusNotFound, "Active lease not found or not owned by caller")
			return
		}
		httpError(w, http.StatusInternalServerError, "Failed to handoff lease: "+err.Error())
		return
	}
	s.audit(r, agent, "coordination.handoff", &req.Resource)
	writeJSON(w, http.StatusOK, map[string]any{
		"handed_off": true,
		"resource":   req.Resource,
	})
}

// POST /coordination/check
func (s *Server) coordinationCheck(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)
	var req struct {
		Resource  string `json:"resource"`
		Intent    string `json:"intent"`
		SessionID string `json:"session_id"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}
	if req.Resource == "" {
		httpError(w, http.StatusBadRequest, "resource is required")
		return
	}

	scopeKey := store.CoordinationScopeKey(agent.TenantID, agent.UserID, agent.ID)
	result, err := s.store.CheckCoordination(r.Context(), scopeKey, req.Resource, agent.ID, coordinationSession(r, agent, req.SessionID), req.Intent)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to check coordination: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GET /coordination/active
func (s *Server) coordinationActive(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)
	prefix := r.URL.Query().Get("resource")
	if prefix == "" {
		prefix = r.URL.Query().Get("prefix")
	}

	scopeKey := store.CoordinationScopeKey(agent.TenantID, agent.UserID, agent.ID)
	leases, intents, err := s.store.ActiveCoordination(r.Context(), scopeKey, prefix)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to get active coordination: "+err.Error())
		return
	}

	// Format claims for MCP / UI compatibility
	claims := make([]map[string]any, len(leases))
	for i, l := range leases {
		claims[i] = map[string]any{
			"lease_id":        l.ID,
			"agent":           l.HolderAgentID,
			"resource":        l.Resource,
			"reason":          l.Intent,
			"session_id":      l.HolderSession,
			"created_at":      l.CreatedAt,
			"heartbeat_at":    l.HeartbeatAt,
			"expires_at":      l.ExpiresAt,
			"handoff_summary": l.HandoffSummary,
			"handoff_to":      l.HandoffTo,
		}
	}

	intentList := make([]map[string]any, len(intents))
	for i, it := range intents {
		intentList[i] = map[string]any{
			"id":         it.ID,
			"agent":      it.AgentID,
			"session_id": it.Session,
			"resource":   it.Resource,
			"intent":     it.Intent,
			"created_at": it.CreatedAt,
			"expires_at": it.ExpiresAt,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"leases":  leases,
		"claims":  claims,
		"intents": intentList,
	})
}

// POST /coordination/intent
func (s *Server) coordinationIntent(w http.ResponseWriter, r *http.Request) {
	agent := agentFrom(r)
	var req struct {
		Resource   string `json:"resource"`
		Intent     string `json:"intent"`
		SessionID  string `json:"session_id"`
		TTLSeconds int    `json:"ttl_seconds"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}
	if req.Resource == "" || req.Intent == "" {
		httpError(w, http.StatusBadRequest, "resource and intent are required")
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second

	scopeKey := store.CoordinationScopeKey(agent.TenantID, agent.UserID, agent.ID)
	it, err := s.store.LogIntent(r.Context(), scopeKey, req.Resource, agent.ID, coordinationSession(r, agent, req.SessionID), req.Intent, ttl)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to log intent: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, it)
}
