package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/memrohq/memro/backend/internal/auth"
	"github.com/memrohq/memro/backend/internal/domain"
	"github.com/memrohq/memro/backend/internal/engine"
	"github.com/memrohq/memro/backend/internal/extract"
	"github.com/memrohq/memro/backend/internal/hooks"
	"github.com/memrohq/memro/backend/internal/metrics"
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

func (s *Server) createIdentity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicKey string `json:"public_key"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := auth.ParsePublicKey(req.PublicKey); err != nil {
		httpError(w, http.StatusBadRequest, "public_key must be a hex-encoded 32-byte ed25519 key (64 hex chars)")
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

func (s *Server) getIdentity(w http.ResponseWriter, r *http.Request) {
	id, err := s.store.GetIdentity(r.Context(), r.PathValue("id"))
	switch {
	case err != nil:
		slog.Error("get identity", "err", err)
		httpError(w, http.StatusInternalServerError, "Internal error")
	case id == nil:
		httpError(w, http.StatusNotFound, "Identity not found")
	default:
		writeJSON(w, http.StatusOK, id)
	}
}

func (s *Server) deleteIdentity(w http.ResponseWriter, r *http.Request) {
	a, id := agentFrom(r), r.PathValue("id")
	if a.ID != id {
		httpError(w, http.StatusForbidden, "Agent ID mismatch")
		return
	}
	if err := s.store.DeleteIdentity(r.Context(), id); err != nil {
		slog.Error("delete identity", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to delete identity")
		return
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
	if req.Content == "" {
		httpError(w, http.StatusBadRequest, "content cannot be empty")
		return
	}
	if utf8.RuneCountInString(req.Content) > maxContentChars {
		httpError(w, http.StatusBadRequest, "content exceeds 10,000 character limit")
		return
	}

	memType := domain.MemoryType("")
	if req.MemoryType == nil || *req.MemoryType == "" || *req.MemoryType == "auto" || *req.MemoryType == "unknown" {
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
	n, err := s.store.CountMemories(ctx, a.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to store memory")
		return false
	}
	if n >= s.cfg.MemorySoftCap {
		httpError(w, http.StatusForbidden, fmt.Sprintf("Memory soft cap reached (%d/agent). Please prune old memories to continue in Developer Preview.", s.cfg.MemorySoftCap))
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

func (s *Server) afterMemoryCreated(r *http.Request, a Agent, m *domain.Memory, action string) {
	s.indexer.Enqueue(m.ID, m.AgentID, m.TenantID, m.Content)
	s.engine.Cache().InvalidateAgent(a.ID)
	metrics.MemoriesCreated.Inc()
	id := m.ID.String()
	s.audit(r, a, action, &id)
	if err := s.store.RecordTenantUsage(r.Context(), a.TenantID, "memories_stored", 1); err != nil {
		slog.Warn("record usage", "err", err)
	}
	if urls, err := s.store.GetHooks(r.Context(), a.TenantID, "memory_created"); err == nil {
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
	ok, err := s.store.DeleteMemory(r.Context(), id, a.ID, a.TenantID)
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
	if nodes > s.cfg.MemorySoftCap*8/10 {
		warning = strPtr(fmt.Sprintf("Approaching memory soft cap (%d). Consider pruning.", s.cfg.MemorySoftCap))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reason_calls": counts["mee.reason"], "deep_reason_calls": 0, "evolve_calls": counts["mee.evolve"],
		"memory_nodes": nodes, "graph_edges": edges, "warning": warning,
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

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	a := agentFrom(r)
	if a.TenantID != domain.DefaultTenantID {
		httpError(w, http.StatusForbidden, "Only default tenant can create new tenants")
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
	if a.TenantID != tid && a.TenantID != domain.DefaultTenantID {
		httpError(w, http.StatusForbidden, "Cannot access other tenant")
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
	if a.TenantID != tid {
		httpError(w, http.StatusForbidden, "Cannot access other tenant usage")
		return
	}
	hours := 24
	if v, err := strconv.Atoi(r.URL.Query().Get("hours")); err == nil && v > 0 {
		hours = v
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
	if err := s.store.RegisterHook(r.Context(), a.TenantID, req.EventType, req.URL); err != nil {
		slog.Error("register hook", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to register hook")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) getHooks(w http.ResponseWriter, r *http.Request) {
	urls, err := s.store.GetHooks(r.Context(), agentFrom(r).TenantID, r.PathValue("event_type"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to retrieve hooks")
		return
	}
	writeJSON(w, http.StatusOK, urls)
}
