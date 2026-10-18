package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/internal/extract"
	"github.com/memrohq/memro/server/internal/metrics"
)

// queryAgent returns the agent_id query parameter, defaulting to the caller.
// Agents may only query their own memories.
func queryAgent(w http.ResponseWriter, r *http.Request, a Agent) (string, bool) {
	id := r.URL.Query().Get("agent_id")
	if id == "" {
		return a.ID, true
	}
	if id != a.ID {
		httpError(w, http.StatusForbidden, "agent_id does not match authenticated identity")
		return "", false
	}
	return id, true
}

func intParam(r *http.Request, name string, def, maxV int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || v <= 0 {
		return def
	}
	return min(v, maxV)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	a := agentFrom(r)
	agentID, ok := queryAgent(w, r, a)
	if !ok {
		return
	}
	q := r.URL.Query()
	query := q.Get("query")
	if query == "" {
		httpError(w, http.StatusBadRequest, "query cannot be empty")
		return
	}
	if len(query) > 500 {
		httpError(w, http.StatusBadRequest, "query exceeds 500 character limit")
		return
	}
	mode := q.Get("mode")
	switch mode {
	case "":
		mode = "hybrid"
	case "hybrid", "semantic", "keyword":
	default:
		httpError(w, http.StatusBadRequest, "mode must be one of hybrid, semantic, keyword")
		return
	}
	resp, err := s.engine.Search(r.Context(), domain.SearchQuery{
		AgentID: agentID, Query: query, Limit: intParam(r, "limit", 10, 50),
		MemoryType: q.Get("memory_type"), Mode: mode,
	}, a.TenantID)
	if err != nil {
		slog.Error("search", "err", err)
		httpError(w, http.StatusInternalServerError, "Search operation failed")
		return
	}
	metrics.SearchesTotal.Inc()
	s.audit(r, a, "memory.search", nil)
	s.recordUsage(r, a, extension.MetricMemoriesSearched, 1)
	out := *resp
	out.LatencyMS = time.Since(start).Milliseconds()
	writeJSON(w, http.StatusOK, out)
}

func parseTime(v string) *time.Time {
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil
	}
	return &t
}

func (s *Server) searchTemporal(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	agentID, ok := queryAgent(w, r, a)
	if !ok {
		return
	}
	q := r.URL.Query()
	if q.Get("query") == "" {
		httpError(w, http.StatusBadRequest, "query cannot be empty")
		return
	}
	tq := engine.TemporalQuery{
		AgentID: agentID, Query: q.Get("query"),
		AfterDate: parseTime(q.Get("after_date")), BeforeDate: parseTime(q.Get("before_date")),
		AfterEventDate: parseTime(q.Get("after_event_date")), BeforeEventDate: parseTime(q.Get("before_event_date")),
		TemporalWeight: 0.3, Limit: intParam(r, "limit", 10, 50),
	}
	if sid, err := uuid.Parse(q.Get("session_id")); err == nil {
		tq.SessionID = &sid
	}
	if v, err := strconv.ParseBool(q.Get("atomic_only")); err == nil {
		tq.AtomicOnly = &v
	}
	if v, err := strconv.ParseFloat(q.Get("temporal_weight"), 32); err == nil {
		tq.TemporalWeight = float32(min(1, max(0, v)))
	}
	items, err := s.engine.TemporalSearch(r.Context(), tq, a.TenantID)
	if err != nil {
		slog.Error("temporal search", "err", err)
		httpError(w, http.StatusInternalServerError, "Temporal search failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": items, "total": len(items)})
}

func (s *Server) temporalStats(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	st, err := s.engine.TemporalStats(r.Context(), a.ID, a.TenantID)
	if err != nil {
		slog.Error("temporal stats", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to fetch memories")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) analyzeConflicts(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	res, err := s.engine.AnalyzeConflicts(r.Context(), a.ID, a.TenantID)
	if err != nil {
		slog.Error("analyze conflicts", "err", err)
		httpError(w, http.StatusInternalServerError, "Conflict analysis failed")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) memoryChain(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid memory ID format")
		return
	}
	chain, err := s.engine.MemoryChain(r.Context(), a.ID, a.TenantID, id)
	if errors.Is(err, engine.ErrNotFound) {
		httpError(w, http.StatusNotFound, "Memory not found")
		return
	}
	if err != nil {
		slog.Error("memory chain", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to load memory chain")
		return
	}
	writeJSON(w, http.StatusOK, chain)
}

func (s *Server) generateAtoms(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Chunk   string  `json:"chunk"`
		Context *string `json:"context"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	preview := req.Chunk
	if runes := []rune(preview); len(runes) > 100 {
		preview = string(runes[:100]) + "..."
	}
	writeJSON(w, http.StatusOK, map[string]any{"atomic_memories": extract.Atoms(req.Chunk), "chunk_preview": preview})
}

// --- MEE (cognitive engine) -----------------------------------------------------------

func (s *Server) reason(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	var req struct {
		Query string `json:"query"`
	}
	if err := decodeJSON(body(r), &req); err != nil || req.Query == "" {
		httpError(w, http.StatusBadRequest, "query is required")
		return
	}
	s.audit(r, a, "mee.reason", nil)
	res, err := s.engine.Reason(r.Context(), a.ID, a.TenantID, req.Query)
	if err != nil {
		slog.Error("reason", "err", err)
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	var req struct {
		MemoryAID uuid.UUID `json:"memory_a_id"`
		MemoryBID uuid.UUID `json:"memory_b_id"`
	}
	if err := decodeJSON(body(r), &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, a, "mee.resolve", nil)
	res, err := s.engine.ResolveConflict(r.Context(), a.ID, a.TenantID, req.MemoryAID, req.MemoryBID)
	if errors.Is(err, engine.ErrNotFound) {
		httpError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		slog.Error("resolve", "err", err)
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) evolve(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	s.audit(r, a, "mee.evolve", nil)
	res, err := s.engine.Evolve(r.Context(), a.ID, a.TenantID)
	if err != nil {
		slog.Error("evolve", "err", err)
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "Invalid memory ID format")
		return
	}
	tl, err := s.engine.Timeline(r.Context(), a.ID, a.TenantID, id)
	if errors.Is(err, engine.ErrNotFound) {
		httpError(w, http.StatusNotFound, "Memory not found")
		return
	}
	if err != nil {
		slog.Error("timeline", "err", err)
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tl)
}
