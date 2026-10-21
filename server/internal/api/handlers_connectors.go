package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/connectors"
)

// Connectors are managed from the dashboard only (ADR 0011): connecting
// one hands an account's data to this agent, so an API key may not do it.

// GET /connectors — available providers and this agent's connections.
func (s *Server) listConnectors(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !requireSession(w, a) {
		return
	}
	conns, err := s.connectors.List(r.Context(), a.ID)
	if err != nil {
		slog.Error("list connectors", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": s.connectors.Providers(), "connections": conns})
}

// POST /connectors/{provider}/authorize — returns the provider URL to open
// and sets the cookie the callback checks.
func (s *Server) authorizeConnector(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !requireSession(w, a) {
		return
	}
	provider := r.PathValue("provider")
	authURL, state, err := s.connectors.StartAuthorization(r.Context(), provider, a.ID, a.TenantID)
	if err != nil {
		connectorError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: connectors.StateCookie, Value: state, Path: "/", MaxAge: int((10 * time.Minute).Seconds()),
		HttpOnly: true, Secure: strings.HasPrefix(s.cfg.Connectors.CallbackBaseURL, "https://"), SameSite: http.SameSiteLaxMode,
	})
	s.audit(r, a, "connector.authorize", &provider)
	writeJSON(w, http.StatusOK, map[string]string{"url": authURL})
}

// POST /connectors/{provider}/token — connects a token-based connector
func (s *Server) connectToken(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !requireSession(w, a) {
		return
	}
	provider := r.PathValue("provider")
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		httpError(w, http.StatusBadRequest, "Token cannot be empty")
		return
	}
	conn, err := s.connectors.ConnectToken(r.Context(), provider, a.ID, a.TenantID, token)
	if err != nil {
		connectorError(w, err)
		return
	}
	cid := conn.ID.String()
	s.audit(r, a, "connector.connect_token", &cid)
	writeJSON(w, http.StatusCreated, conn)
}

// GET /connectors/{provider}/callback — the provider redirects the browser
// here; it always ends on the dashboard with the outcome in the query.
func (s *Server) connectorCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	cookie := ""
	if c, err := r.Cookie(connectors.StateCookie); err == nil {
		cookie = c.Value
	}
	http.SetCookie(w, &http.Cookie{Name: connectors.StateCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: strings.HasPrefix(s.cfg.Connectors.CallbackBaseURL, "https://"), SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")

	conn, err := s.connectors.CompleteAuthorization(r.Context(), provider, r.URL.Query(), cookie)
	if err != nil {
		var ue *connectors.UserError
		switch {
		case errors.As(err, &ue), errors.Is(err, connectors.ErrBadState), errors.Is(err, connectors.ErrDisabled),
			errors.Is(err, connectors.ErrUnknownProvider):
		default:
			slog.Error("connector callback", "provider", provider, "err", err)
			err = errors.New("Something went wrong while connecting. Please try again.")
		}
		http.Redirect(w, r, s.connectors.DashboardURL(provider, err), http.StatusFound)
		return
	}
	id := conn.ID.String()
	s.audit(r, Agent{ID: conn.AgentID, TenantID: conn.TenantID}, "connector.connect", &id)
	http.Redirect(w, r, s.connectors.DashboardURL(provider, nil), http.StatusFound)
}

func connectionID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid connection ID")
		return uuid.Nil, false
	}
	return id, true
}

// GET /connectors/{id}/sources
func (s *Server) connectorSources(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !requireSession(w, a) {
		return
	}
	id, ok := connectionID(w, r)
	if !ok {
		return
	}
	srcs, err := s.connectors.Sources(r.Context(), a.ID, id)
	if err != nil {
		connectorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, srcs)
}

// PUT /connectors/{id}/sources {"enabled": {"C0123": true, "C0456": false}}
func (s *Server) updateConnectorSources(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !requireSession(w, a) {
		return
	}
	id, ok := connectionID(w, r)
	if !ok {
		return
	}
	var req struct {
		Enabled map[string]bool `json:"enabled"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Enabled) > 500 {
		httpError(w, http.StatusBadRequest, "Change at most 500 sources at a time")
		return
	}
	if err := s.connectors.UpdateSources(r.Context(), a.ID, id, req.Enabled); err != nil {
		connectorError(w, err)
		return
	}
	cid := id.String()
	s.audit(r, a, "connector.sources", &cid)
	srcs, err := s.connectors.Sources(r.Context(), a.ID, id)
	if err != nil {
		connectorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, srcs)
}

// POST /connectors/{id}/sync — sync now (also resumes a paused connection).
func (s *Server) syncConnector(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !requireSession(w, a) {
		return
	}
	id, ok := connectionID(w, r)
	if !ok {
		return
	}
	if err := s.connectors.SyncNow(r.Context(), a.ID, id); err != nil {
		connectorError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}

// DELETE /connectors/{id}?delete_memories=true
func (s *Server) deleteConnector(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if !requireSession(w, a) {
		return
	}
	id, ok := connectionID(w, r)
	if !ok {
		return
	}
	purge := r.URL.Query().Get("delete_memories") == "true"
	n, err := s.connectors.Disconnect(r.Context(), a.ID, id, purge)
	if err != nil {
		connectorError(w, err)
		return
	}
	cid := id.String()
	s.audit(r, a, "connector.disconnect", &cid)
	writeJSON(w, http.StatusOK, map[string]int{"deleted_memories": n})
}

// POST /connectors/slack/events — Slack Events API (signed by Slack).
func (s *Server) slackEvents(w http.ResponseWriter, r *http.Request) {
	raw, ok := readWebhook(w, r)
	if !ok {
		return
	}
	status, resp := s.connectors.SlackEvents(r.Context(), r.Header, raw)
	writeJSON(w, status, resp)
}

// POST /connectors/github/webhook — GitHub App webhooks (signed by GitHub).
func (s *Server) githubWebhook(w http.ResponseWriter, r *http.Request) {
	raw, ok := readWebhook(w, r)
	if !ok {
		return
	}
	w.WriteHeader(s.connectors.GitHubWebhook(r.Context(), r.Header, raw))
}

// POST /connectors/linear/webhook — Linear webhooks (signed by Linear).
func (s *Server) linearWebhook(w http.ResponseWriter, r *http.Request) {
	raw, ok := readWebhook(w, r)
	if !ok {
		return
	}
	w.WriteHeader(s.connectors.LinearWebhook(r.Context(), r.Header, raw))
}

// POST /connectors/sentry/webhook — Sentry webhooks (signed with HMAC).
func (s *Server) sentryWebhook(w http.ResponseWriter, r *http.Request) {
	raw, ok := readWebhook(w, r)
	if !ok {
		return
	}
	w.WriteHeader(s.connectors.SentryWebhook(r.Context(), r.Header, raw))
}

// POST /connectors/notion/webhook — Notion webhooks.
func (s *Server) notionWebhook(w http.ResponseWriter, r *http.Request) {
	raw, ok := readWebhook(w, r)
	if !ok {
		return
	}
	w.WriteHeader(s.connectors.NotionWebhook(r.Context(), r.Header, raw))
}

// POST /connectors/jira/webhook — Jira webhooks (signed with HMAC).
func (s *Server) jiraWebhook(w http.ResponseWriter, r *http.Request) {
	raw, ok := readWebhook(w, r)
	if !ok {
		return
	}
	w.WriteHeader(s.connectors.JiraWebhook(r.Context(), r.Header, r.URL.Query(), raw))
}

// POST /connectors/gitlab/webhook — GitLab webhooks (token verified).
func (s *Server) gitlabWebhook(w http.ResponseWriter, r *http.Request) {
	raw, ok := readWebhook(w, r)
	if !ok {
		return
	}
	w.WriteHeader(s.connectors.GitLabWebhook(r.Context(), r.Header, raw))
}

const maxWebhookBytes = 5 << 20

func readWebhook(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	raw, err := readAllLimit(r, maxWebhookBytes)
	if err != nil {
		httpError(w, http.StatusRequestEntityTooLarge, "Payload too large")
		return nil, false
	}
	return raw, true
}

func connectorError(w http.ResponseWriter, err error) {
	var ue *connectors.UserError
	switch {
	case errors.As(err, &ue):
		httpError(w, http.StatusBadRequest, ue.Msg)
	case errors.Is(err, connectors.ErrNotFound):
		httpError(w, http.StatusNotFound, "Connection not found")
	case errors.Is(err, connectors.ErrUnknownProvider):
		httpError(w, http.StatusNotFound, "Unknown connector")
	case errors.Is(err, connectors.ErrDisabled):
		httpError(w, http.StatusServiceUnavailable, err.Error())
	default:
		slog.Error("connector", "err", err)
		httpError(w, http.StatusInternalServerError, "Connector error")
	}
}

func readAllLimit(r *http.Request, n int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, n+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > n {
		return nil, errors.New("too large")
	}
	return raw, nil
}
