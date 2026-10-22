package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/auth"
)

const deviceCodeTTL = 10 * time.Minute

// requireSession limits account-management actions to the dashboard, so an
// API key cannot mint more keys or approve logins.
func requireSession(w http.ResponseWriter, a Agent) bool {
	if a.Via != "session" {
		httpError(w, http.StatusForbidden, "Sign in to the dashboard to do this")
		return false
	}
	return true
}

// GET /auth/whoami — who this credential acts as.
func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	writeJSON(w, http.StatusOK, map[string]any{"agent_id": a.ID, "tenant_id": a.TenantID, "user_id": a.UserID, "auth": a.Via})
}

// --- Device-code login (used by `memro connect`) ------------------------------------

// POST /auth/device/code — start a login; the CLI shows user_code and opens
// verification_uri_complete in the browser.
func (s *Server) deviceCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientName string `json:"client_name"`
	}
	_ = readJSON(r, &req)
	name := strings.TrimSpace(req.ClientName)
	if name == "" {
		name = "Memro CLI"
	}
	if utf8.RuneCountInString(name) > 60 {
		name = string([]rune(name)[:60])
	}
	deviceCode, userCode, err := auth.NewDeviceCode()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to create code")
		return
	}
	if err := s.store.CreateDeviceCode(r.Context(), auth.HashToken(deviceCode), userCode, name, time.Now().Add(deviceCodeTTL)); err != nil {
		slog.Error("create device code", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to create code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code":               deviceCode,
		"user_code":                 userCode,
		"verification_uri":          s.cfg.AppURL + "/connect",
		"verification_uri_complete": s.cfg.AppURL + "/connect?code=" + userCode,
		"expires_in":                int(deviceCodeTTL.Seconds()),
		"interval":                  3,
	})
}

// POST /auth/device/token — the CLI polls this. Once the user has approved,
// it returns a new API key exactly once. Errors follow RFC 8628.
func (s *Server) deviceToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if err := readJSON(r, &req); err != nil || req.DeviceCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	ctx, hash := r.Context(), auth.HashToken(req.DeviceCode)
	d, err := s.store.ConsumeDeviceCode(ctx, hash)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if d == nil {
		st, err := s.store.DeviceCodeStatus(ctx, hash)
		switch {
		case err != nil:
			httpError(w, http.StatusInternalServerError, "Database error")
		case st == nil || st.ConsumedAt != nil:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		case time.Now().After(st.ExpiresAt):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expired_token"})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "authorization_pending"})
		}
		return
	}
	key, prefix, keyHash, err := auth.NewAPIKey()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to create key")
		return
	}
	name := d.ClientName + " · " + time.Now().UTC().Format("2 Jan 2006")
	if err := s.store.CreateAPIKey(ctx, uuid.New(), *d.TenantID, *d.AgentID, name, prefix, keyHash); err != nil {
		slog.Error("create api key", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to create key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"api_key":  key,
		"agent_id": *d.AgentID,
		"api_url":  s.cfg.BaseURL,
		"mcp_url":  s.cfg.MCPURL,
	})
}

// GET /auth/device?user_code=… — what the dashboard shows before approving.
func (s *Server) deviceInfo(w http.ResponseWriter, r *http.Request) {
	if !requireSession(w, agentFrom(r)) {
		return
	}
	d, err := s.store.DeviceCodeByUserCode(r.Context(), strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("user_code"))))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	status := "pending"
	switch {
	case d == nil:
		httpError(w, http.StatusNotFound, "Code not found")
		return
	case d.ApprovedAt != nil:
		status = "approved"
	case time.Now().After(d.ExpiresAt):
		status = "expired"
	}
	writeJSON(w, http.StatusOK, map[string]any{"client_name": d.ClientName, "created_at": d.CreatedAt, "expires_at": d.ExpiresAt, "status": status})
}

// POST /auth/device/approve {"user_code": "…"} — connect the waiting CLI to
// the signed-in user's agent.
func (s *Server) deviceApprove(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !requireSession(w, a) {
		return
	}
	var req struct {
		UserCode string `json:"user_code"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	ok, err := s.store.ApproveDeviceCode(r.Context(), strings.ToUpper(strings.TrimSpace(req.UserCode)), a.ID, a.TenantID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !ok {
		httpError(w, http.StatusNotFound, "This code is invalid, expired or already used")
		return
	}
	s.audit(r, a, "device.approve", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"approved": true})
}

// --- API keys -------------------------------------------------------------------------

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if a.Via == "api_key" {
		httpError(w, http.StatusForbidden, "API keys cannot manage API keys")
		return
	}
	keys, err := s.store.ListAPIKeys(r.Context(), a.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// POST /api-keys {"name": "…"} — the key is returned once and never again.
func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if a.Via == "api_key" {
		httpError(w, http.StatusForbidden, "API keys cannot manage API keys")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	_ = decodeJSON(body(r), &req)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "API key"
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
	if err := s.store.CreateAPIKey(r.Context(), id, a.TenantID, a.ID, name, prefix, hash); err != nil {
		slog.Error("create api key", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to create key")
		return
	}
	kid := id.String()
	s.audit(r, a, "api_key.create", &kid)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "name": name, "prefix": prefix, "key": key})
}

func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if a.Via == "api_key" {
		httpError(w, http.StatusForbidden, "API keys cannot manage API keys")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid key ID")
		return
	}
	ok, err := s.store.RevokeAPIKey(r.Context(), id, a.ID)
	switch {
	case err != nil:
		httpError(w, http.StatusInternalServerError, "Database error")
	case !ok:
		httpError(w, http.StatusNotFound, "Key not found")
	default:
		kid := id.String()
		s.audit(r, a, "api_key.revoke", &kid)
		w.WriteHeader(http.StatusNoContent)
	}
}
