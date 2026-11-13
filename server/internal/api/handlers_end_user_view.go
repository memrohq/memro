package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/memrohq/memro/server/internal/store"
)

// Organisation owners and admins can read their end users' memories and
// graph (ADR 0030, which changes ADR 0026's "never"). Read-only, and every
// view is written to the organisation's audit log, so customers can show
// their users who looked. Billing and viewer members still see usage only.

// GET /tenant/{id}/end-users/{agent_id}/memories?limit= and …/graph?limit=
func (s *Server) viewEndUser(w http.ResponseWriter, r *http.Request) {
	tid, _, ok := s.tenantAccess(w, r, store.RoleOwner, store.RoleAdmin)
	if !ok {
		return
	}
	ctx := r.Context()
	agentID := r.PathValue("agent_id")
	user, err := s.store.EndUserOf(ctx, tid, agentID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if user == "" {
		httpError(w, http.StatusNotFound, "No such user in this organisation (deleted users' memories are gone)")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var res any
	what := r.PathValue("what")
	switch what {
	case "memories":
		res, err = s.store.ListMemories(ctx, agentID, tid, "", "", min(max(limit, 1), 100))
	case "graph":
		res, err = s.engine.MemoryGraph(ctx, agentID, tid, limit)
	default:
		httpError(w, http.StatusNotFound, "Not found")
		return
	}
	if err != nil {
		slog.Error("view end user", "what", what, "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to load memories")
		return
	}
	// Into the organisation's own audit log, as the viewer's agent.
	a, ip, resource := agentFrom(r), s.clientIP(r), user+" ("+agentID+")"
	if err := s.store.Audit(ctx, a.ID, tid, "end_user."+what+".view", &resource, &ip); err != nil {
		slog.Error("audit end-user view", "err", err)
	}
	writeJSON(w, http.StatusOK, res)
}
