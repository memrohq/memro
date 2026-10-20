// Package engine implements memory search, reasoning, conflict resolution and
// consolidation on top of Postgres + pgvector.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/embed"
	"github.com/memrohq/memro/server/internal/metrics"
)

var ErrNotFound = errors.New("not found")

// Indexer queues memories for embedding.
type Indexer interface {
	Enqueue(memoryID uuid.UUID, agentID string, tenantID uuid.UUID, content string)
}

type Engine struct {
	pool    *pgxpool.Pool
	embed   *embed.Provider
	indexer Indexer
	cache   *ResultCache
	now     func() time.Time
}

func New(pool *pgxpool.Pool, provider *embed.Provider, indexer Indexer) *Engine {
	return &Engine{pool: pool, embed: provider, indexer: indexer, cache: NewResultCache(), now: time.Now}
}

func (e *Engine) Cache() *ResultCache { return e.cache }

// queryEmbedding returns nil (not an error) when semantic search is
// unavailable, so callers fall back to keyword search.
func (e *Engine) queryEmbedding(ctx context.Context, q string) []float32 {
	if !e.embed.Enabled() {
		return nil
	}
	v, err := e.embed.EmbedQuery(ctx, q)
	if err != nil {
		slog.Warn("query embedding failed; using keyword search only", "err", err)
		return nil
	}
	return v
}

// --- Search -----------------------------------------------------------------

// Search runs hybrid semantic + keyword search over an agent's memories from
// the last 90 days. Uses Reciprocal Rank Fusion (RRF, k=60) to combine dense
// embedding ranks and sparse keyword ranks without score calibration skew.
func (e *Engine) Search(ctx context.Context, q domain.SearchQuery, tenantID uuid.UUID) (*domain.SearchResponse, error) {
	start := e.now()
	mode := q.Mode
	if mode == "" {
		mode = "hybrid"
	}
	if cached, ok := e.cache.Get(q, tenantID); ok {
		metrics.SearchCacheHits.Inc()
		return cached, nil
	}
	metrics.SearchCacheMisses.Inc()

	scores := map[uuid.UUID]float32{}
	seen := map[uuid.UUID]bool{}
	var order []uuid.UUID

	const (
		rrfK      = 60.0
		weightSem = 1.0
		weightKw  = 0.8
	)

	var vec []float32
	if mode != "keyword" {
		vec = e.queryEmbedding(ctx, q.Query)
	}
	if vec != nil {
		ids, err := e.semanticIDs(ctx, q, tenantID, vec)
		if err != nil {
			return nil, err
		}
		for i, id := range ids {
			rrf := float32(weightSem / (rrfK + float64(i+1)))
			scores[id] += rrf
			if !seen[id] {
				seen[id] = true
				order = append(order, id)
			}
		}
	}
	if mode != "semantic" || vec == nil {
		kw, err := e.keywordHits(ctx, q, tenantID)
		if err != nil {
			return nil, err
		}
		for j, h := range kw {
			rrf := float32(weightKw / (rrfK + float64(j+1)))
			scores[h.id] += rrf
			if !seen[h.id] {
				seen[h.id] = true
				order = append(order, h.id)
			}
		}
	}

	maxPossibleRRF := float32(weightSem/(rrfK+1.0) + weightKw/(rrfK+1.0))
	if maxPossibleRRF > 0 {
		for id := range scores {
			scores[id] = min(1.0, scores[id]/maxPossibleRRF)
		}
	}

	sort.SliceStable(order, func(i, j int) bool { return scores[order[i]] > scores[order[j]] })
	if len(order) > q.Limit {
		order = order[:q.Limit]
	}

	results, err := e.loadResults(ctx, order, scores)
	if err != nil {
		return nil, err
	}
	resp := &domain.SearchResponse{
		Results:   results,
		Mode:      mode,
		LatencyMS: e.now().Sub(start).Milliseconds(),
		Total:     len(results),
	}
	e.cache.Set(q, tenantID, resp)
	return resp, nil
}

func typeFilter(q domain.SearchQuery, args *[]any) string {
	if q.MemoryType == "" {
		return ""
	}
	*args = append(*args, q.MemoryType)
	return fmt.Sprintf(" AND m.memory_type = $%d", len(*args))
}

func (e *Engine) semanticIDs(ctx context.Context, q domain.SearchQuery, tenantID uuid.UUID, vec []float32) ([]uuid.UUID, error) {
	args := []any{q.AgentID, tenantID, pgvector.NewVector(vec), len(vec), q.Limit}
	sql := `SELECT m.id FROM memory_embeddings e JOIN memories m ON m.id = e.memory_id
		WHERE m.agent_id = $1 AND m.tenant_id = $2 AND e.dims = $4
		  AND m.created_at > NOW() - INTERVAL '90 days'` + typeFilter(q, &args) + `
		ORDER BY e.embedding <=> $3 LIMIT $5`
	rows, err := e.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type keywordHit struct {
	id   uuid.UUID
	rank float32
}

func (e *Engine) keywordHits(ctx context.Context, q domain.SearchQuery, tenantID uuid.UUID) ([]keywordHit, error) {
	args := []any{q.AgentID, tenantID, q.Query, q.Limit}
	sql := `SELECT m.id, ts_rank_cd(to_tsvector('english', m.content), plainto_tsquery('english', $3)) AS rank
		FROM memories m
		WHERE m.agent_id = $1 AND m.tenant_id = $2
		  AND m.created_at > NOW() - INTERVAL '90 days'
		  AND to_tsvector('english', m.content) @@ plainto_tsquery('english', $3)` + typeFilter(q, &args) + `
		ORDER BY rank DESC LIMIT $4`
	rows, err := e.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	defer rows.Close()
	var hits []keywordHit
	for rows.Next() {
		var h keywordHit
		if err := rows.Scan(&h.id, &h.rank); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func (e *Engine) loadResults(ctx context.Context, ids []uuid.UUID, scores map[uuid.UUID]float32) ([]domain.SearchResult, error) {
	results := []domain.SearchResult{}
	if len(ids) == 0 {
		return results, nil
	}
	if _, err := e.pool.Exec(ctx, `UPDATE memories SET access_count = COALESCE(access_count, 0) + 1,
		last_accessed_at = NOW() WHERE id = ANY($1)`, ids); err != nil {
		slog.Warn("access tracking failed", "err", err)
	}
	rows, err := e.pool.Query(ctx, `SELECT id, content, memory_type, created_at, asset_url FROM memories WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[uuid.UUID]domain.SearchResult{}
	for rows.Next() {
		var id uuid.UUID
		var r domain.SearchResult
		var created time.Time
		if err := rows.Scan(&id, &r.Content, &r.MemoryType, &created, &r.AssetURL); err != nil {
			return nil, err
		}
		r.MemoryID = id.String()
		r.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		byID[id] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			r.Score = scores[id]
			results = append(results, r)
		}
	}
	return results, nil
}

// --- Temporal search ---------------------------------------------------------

type TemporalQuery struct {
	AgentID         string
	Query           string
	AfterDate       *time.Time
	BeforeDate      *time.Time
	AfterEventDate  *time.Time
	BeforeEventDate *time.Time
	SessionID       *uuid.UUID
	AtomicOnly      *bool
	TemporalWeight  float32
	Limit           int
}

type TemporalItem struct {
	MemoryID        string  `json:"memory_id"`
	Content         string  `json:"content"`
	DocumentDate    string  `json:"document_date"`
	EventDate       *string `json:"event_date"`
	IsAtomic        bool    `json:"is_atomic"`
	Score           float32 `json:"score"`
	TemporalContext string  `json:"temporal_context"`
}

// TemporalSearch filters by document/event dates and boosts recent memories:
// score = similarity*(1-w) + recency*w, where recency = 1/(1 + age_days/30).
func (e *Engine) TemporalSearch(ctx context.Context, q TemporalQuery, tenantID uuid.UUID) ([]TemporalItem, error) {
	args := []any{q.AgentID, tenantID}
	where := `m.agent_id = $1 AND m.tenant_id = $2`
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.AfterDate != nil {
		add("COALESCE(m.document_date, m.created_at) >= $%d", *q.AfterDate)
	}
	if q.BeforeDate != nil {
		add("COALESCE(m.document_date, m.created_at) <= $%d", *q.BeforeDate)
	}
	if q.AfterEventDate != nil {
		add("m.event_date >= $%d", *q.AfterEventDate)
	}
	if q.BeforeEventDate != nil {
		add("m.event_date <= $%d", *q.BeforeEventDate)
	}
	if q.SessionID != nil {
		add("m.session_id = $%d", *q.SessionID)
	}
	if q.AtomicOnly != nil {
		add("COALESCE(m.is_atomic, false) = $%d", *q.AtomicOnly)
	}

	var simExpr, join, order string
	if vec := e.queryEmbedding(ctx, q.Query); vec != nil {
		// Memories without an embedding yet (just written, or the embedding
		// call failed) are still found by keyword.
		args = append(args, pgvector.NewVector(vec), len(vec), q.Query)
		vi, di, qi := len(args)-2, len(args)-1, len(args)
		tsq := fmt.Sprintf("plainto_tsquery('english', $%d)", qi)
		join = fmt.Sprintf(" LEFT JOIN memory_embeddings e ON e.memory_id = m.id AND e.dims = $%d", di)
		where += fmt.Sprintf(" AND (e.memory_id IS NOT NULL OR to_tsvector('english', m.content) @@ %s)", tsq)
		simExpr = fmt.Sprintf("COALESCE((1 - (e.embedding <=> $%d))::REAL, ts_rank_cd(to_tsvector('english', m.content), %s)::REAL)", vi, tsq)
		order = simExpr + " DESC"
	} else {
		args = append(args, q.Query)
		qi := len(args)
		where += fmt.Sprintf(" AND to_tsvector('english', m.content) @@ plainto_tsquery('english', $%d)", qi)
		simExpr = fmt.Sprintf("ts_rank_cd(to_tsvector('english', m.content), plainto_tsquery('english', $%d))", qi)
		order = simExpr + " DESC"
	}
	args = append(args, q.Limit*3)
	sql := fmt.Sprintf(`SELECT m.id, m.content, COALESCE(m.document_date, m.created_at), m.event_date,
			COALESCE(m.is_atomic, false), %s
		FROM memories m%s WHERE %s ORDER BY %s LIMIT $%d`, simExpr, join, where, order, len(args))

	rows, err := e.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("temporal search: %w", err)
	}
	defer rows.Close()
	now := e.now()
	items := []TemporalItem{}
	for rows.Next() {
		var id uuid.UUID
		var it TemporalItem
		var doc time.Time
		var ev *time.Time
		var sim float32
		if err := rows.Scan(&id, &it.Content, &doc, &ev, &it.IsAtomic, &sim); err != nil {
			return nil, err
		}
		ageDays := float32(int(now.Sub(doc).Hours() / 24))
		recency := 1 / (1 + ageDays/30)
		it.MemoryID = id.String()
		it.Score = sim*(1-q.TemporalWeight) + recency*q.TemporalWeight
		it.DocumentDate = doc.UTC().Format(time.RFC3339Nano)
		occurred := "N/A"
		if ev != nil {
			s := ev.UTC().Format(time.RFC3339Nano)
			it.EventDate = &s
			occurred = ev.UTC().Format("2006-01-02")
		}
		it.TemporalContext = fmt.Sprintf("Observed: %s, Occurred: %s", doc.UTC().Format("2006-01-02"), occurred)
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	if len(items) > q.Limit {
		items = items[:q.Limit]
	}
	return items, nil
}

// --- Reasoning ----------------------------------------------------------------

// Reason answers from memory. Simple questions return the best match; "why /
// how / because" questions expand the knowledge graph around the top hits.
func (e *Engine) Reason(ctx context.Context, agentID string, tenantID uuid.UUID, query string) (*domain.ReasoningResult, error) {
	lower := strings.ToLower(query)
	complex := strings.Contains(lower, "why") || strings.Contains(lower, "how") || strings.Contains(lower, "because")
	sq := domain.SearchQuery{AgentID: agentID, Query: query, Limit: 3, Mode: "semantic"}
	if complex {
		sq.Limit, sq.Mode = 10, "hybrid"
	}
	res, err := e.Search(ctx, sq, tenantID)
	if err != nil {
		return nil, err
	}
	if len(res.Results) == 0 {
		return &domain.ReasoningResult{
			Answer: "I couldn't find any relevant memories to reason from.", Why: "No results found in initial search.",
			BasedOn: []uuid.UUID{}, Paths: [][]uuid.UUID{},
		}, nil
	}
	top := res.Results[0]
	topID := uuid.MustParse(top.MemoryID)
	if !complex {
		return &domain.ReasoningResult{
			Answer: top.Content, Confidence: top.Score,
			BasedOn: []uuid.UUID{topID}, Paths: [][]uuid.UUID{{topID}},
			Why: fmt.Sprintf("Fast-path retrieval: found direct match with score %.2f", top.Score),
		}, nil
	}

	ids := make([]uuid.UUID, 0, len(res.Results))
	nodes := make([]chainNode, 0, len(res.Results))
	for _, r := range res.Results {
		id := uuid.MustParse(r.MemoryID)
		ids = append(ids, id)
		nodes = append(nodes, chainNode{ID: id, Content: r.Content})
	}
	edges, err := e.relationsTouching(ctx, ids)
	if err != nil {
		return nil, err
	}
	walks := bfsWalks(topID, nodes, edges, 2)
	if len(walks) == 0 {
		return &domain.ReasoningResult{
			Answer: top.Content, Confidence: top.Score, BasedOn: ids, Paths: [][]uuid.UUID{{topID}},
			Why: "Direct match found in memory; no deep causal paths identified.",
		}, nil
	}
	best := walks[0]
	for _, w := range walks[1:] {
		if w.Confidence > best.Confidence {
			best = w
		}
	}
	var why strings.Builder
	fmt.Fprintf(&why, "Expanded graph from memory %s (confidence %.2f).\n", topID, best.Confidence)
	contents := make([]string, 0, len(best.Nodes))
	for _, n := range best.Nodes {
		fmt.Fprintf(&why, " -> Found connection to: %s\n", n.Content)
		contents = append(contents, n.Content)
	}
	return &domain.ReasoningResult{
		Answer:     fmt.Sprintf("I've identified a causal link between your memories: %s.", strings.Join(contents, ", which directly led to ")),
		Why:        why.String(),
		BasedOn:    ids,
		Confidence: best.Confidence,
		Paths:      [][]uuid.UUID{best.Path},
	}, nil
}

func (e *Engine) relationsTouching(ctx context.Context, ids []uuid.UUID) ([]domain.Relation, error) {
	rows, err := e.pool.Query(ctx, `SELECT source_memory_id, target_memory_id, relation_type, COALESCE(confidence, 1.0)::REAL
		FROM memory_relations WHERE source_memory_id = ANY($1) OR target_memory_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Relation
	for rows.Next() {
		var r domain.Relation
		var t string
		if err := rows.Scan(&r.From, &r.To, &t, &r.Confidence); err != nil {
			return nil, err
		}
		r.Type = domain.RelationType(t)
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Conflict resolution -------------------------------------------------------

// ResolveConflict picks the more trustworthy of two memories owned by the
// agent, weighing recency, access frequency and stored confidence.
func (e *Engine) ResolveConflict(ctx context.Context, agentID string, tenantID uuid.UUID, a, b uuid.UUID) (*domain.ResolutionResult, error) {
	rows, err := e.pool.Query(ctx, `SELECT id, created_at, COALESCE(access_count, 0), COALESCE(confidence, 1.0)
		FROM memories WHERE id IN ($1, $2) AND agent_id = $3 AND tenant_id = $4`, a, b, agentID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sides := map[uuid.UUID]conflictSide{}
	for rows.Next() {
		var s conflictSide
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.Access, &s.Confidence); err != nil {
			return nil, err
		}
		sides[s.ID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sa, okA := sides[a]
	sb, okB := sides[b]
	if !okA || !okB {
		return nil, fmt.Errorf("one or both memories not found: %w", ErrNotFound)
	}
	r := resolve(sa, sb, e.now(), DefaultWeights)
	return &r, nil
}

// --- Evolution ----------------------------------------------------------------

// Evolve consolidates recent episodic memories into one semantic memory and
// links contradicting profile/semantic memories with "updates" relations.
func (e *Engine) Evolve(ctx context.Context, agentID string, tenantID uuid.UUID) (*domain.EvolutionResult, error) {
	var patterns []string
	evolved := 0
	involved := []uuid.UUID{}

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `SELECT id, content FROM memories
		WHERE agent_id = $1 AND tenant_id = $2 AND memory_type = 'episodic'
		  AND created_at > NOW() - INTERVAL '30 days' AND COALESCE(confidence, 1.0) > 0.1
		ORDER BY created_at`, agentID, tenantID)
	if err != nil {
		return nil, err
	}
	var episodeIDs []uuid.UUID
	var episodes []string
	for rows.Next() {
		var id uuid.UUID
		var c string
		if err := rows.Scan(&id, &c); err != nil {
			rows.Close()
			return nil, err
		}
		episodeIDs = append(episodeIDs, id)
		episodes = append(episodes, c)
	}
	rows.Close()

	var newMemory *uuid.UUID
	var consolidated string
	if len(episodes) >= 3 {
		id := uuid.New()
		consolidated = "Consolidated Summary: " + strings.Join(episodes, "; ")
		if _, err := tx.Exec(ctx, `INSERT INTO memories (id, agent_id, tenant_id, content, memory_type, visibility,
				created_at, document_date, last_accessed_at, confidence, entity_tags, relationship_types)
			VALUES ($1, $2, $3, $4, 'semantic', 'private', NOW(), NOW(), NOW(), 0.9, '{}', '{}')`,
			id, agentID, tenantID, consolidated); err != nil {
			return nil, err
		}
		for _, src := range episodeIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id,
					relation_type, confidence, created_by) VALUES ($1, $2, $3, $4, 'derives', 0.8, $5)`,
				uuid.New(), tenantID, src, id, agentID); err != nil {
				return nil, err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE memories SET confidence = COALESCE(confidence, 1.0) * 0.5 WHERE id = ANY($1)`, episodeIDs); err != nil {
			return nil, err
		}
		newMemory = &id
		evolved += len(episodeIDs)
		involved = append(involved, episodeIDs...)
		patterns = append(patterns, "Consolidated episodic clusters into semantic core")
	}

	rows, err = tx.Query(ctx, `SELECT id, content FROM memories
		WHERE agent_id = $1 AND tenant_id = $2 AND memory_type IN ('profile', 'semantic')
		  AND COALESCE(confidence, 1.0) > 0.3`, agentID, tenantID)
	if err != nil {
		return nil, err
	}
	type core struct {
		id      uuid.UUID
		content string
	}
	var cores []core
	for rows.Next() {
		var c core
		if err := rows.Scan(&c.id, &c.content); err != nil {
			rows.Close()
			return nil, err
		}
		cores = append(cores, c)
	}
	rows.Close()

	for i := range cores {
		for j := i + 1; j < len(cores); j++ {
			conf := DetectContradiction(cores[i].content, cores[j].content)
			if conf <= 0.5 {
				continue
			}
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM memory_relations
				WHERE (source_memory_id = $1 AND target_memory_id = $2) OR (source_memory_id = $2 AND target_memory_id = $1))`,
				cores[i].id, cores[j].id).Scan(&exists); err != nil {
				return nil, err
			}
			if exists {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id,
					relation_type, confidence, created_by) VALUES ($1, $2, $3, $4, 'updates', $5, $6)`,
				uuid.New(), tenantID, cores[i].id, cores[j].id, conf, agentID); err != nil {
				return nil, err
			}
			patterns = append(patterns, fmt.Sprintf("Identified conflict between memories %s and %s", cores[i].id, cores[j].id))
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	e.cache.InvalidateAgent(agentID)
	if newMemory != nil && e.indexer != nil {
		e.indexer.Enqueue(*newMemory, agentID, tenantID, consolidated)
	}
	if len(patterns) == 0 {
		patterns = []string{"No evolution targets identified"}
	}
	diff, _ := json.Marshal(map[string]any{"involved": involved})
	return &domain.EvolutionResult{MemoriesEvolved: evolved, PatternsIdentified: patterns, Diff: diff}, nil
}

// --- Timeline -------------------------------------------------------------------

func (e *Engine) Timeline(ctx context.Context, agentID string, tenantID uuid.UUID, memoryID uuid.UUID) (*domain.MemoryTimeline, error) {
	var owned bool
	if err := e.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM memories WHERE id = $1 AND agent_id = $2 AND tenant_id = $3)
		OR EXISTS (SELECT 1 FROM audit_log WHERE resource_id = $4 AND agent_id = $2 AND tenant_id = $3)`,
		memoryID, agentID, tenantID, memoryID.String()).Scan(&owned); err != nil {
		return nil, err
	}
	if !owned {
		return nil, ErrNotFound
	}
	rows, err := e.pool.Query(ctx, `SELECT created_at, action, COALESCE(resource_id, '') FROM audit_log
		WHERE resource_id = $1 AND tenant_id = $2 ORDER BY created_at ASC`, memoryID.String(), tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tl := &domain.MemoryTimeline{MemoryID: memoryID, Events: []domain.TimelineEvent{}}
	for rows.Next() {
		var ts time.Time
		var action, res string
		if err := rows.Scan(&ts, &action, &res); err != nil {
			return nil, err
		}
		tl.Events = append(tl.Events, domain.TimelineEvent{
			Timestamp: ts.UTC().Format(time.RFC3339Nano), Action: action,
			Details: "Action performed on memory " + res,
		})
	}
	return tl, rows.Err()
}

// --- Memory chain -------------------------------------------------------------

type ChainItem struct {
	MemoryID     string   `json:"memory_id"`
	Content      string   `json:"content"`
	RelationType *string  `json:"relation_type"`
	Confidence   *float32 `json:"confidence"`
}

type Chain struct {
	RootMemory      ChainItem   `json:"root_memory"`
	RelatedMemories []ChainItem `json:"related_memories"`
}

// MemoryChain returns a memory plus every memory linked to it by a relation.
func (e *Engine) MemoryChain(ctx context.Context, agentID string, tenantID uuid.UUID, id uuid.UUID) (*Chain, error) {
	var root ChainItem
	err := e.pool.QueryRow(ctx, `SELECT id::TEXT, content FROM memories WHERE id = $1 AND agent_id = $2 AND tenant_id = $3`,
		id, agentID, tenantID).Scan(&root.MemoryID, &root.Content)
	if err != nil {
		return nil, ErrNotFound
	}
	rows, err := e.pool.Query(ctx, `SELECT m.id::TEXT, m.content, r.relation_type, COALESCE(r.confidence, 1.0)::REAL
		FROM memory_relations r
		JOIN memories m ON m.id = CASE WHEN r.source_memory_id = $1 THEN r.target_memory_id ELSE r.source_memory_id END
		WHERE (r.source_memory_id = $1 OR r.target_memory_id = $1) AND r.tenant_id = $2
		ORDER BY r.created_at`, id, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chain := &Chain{RootMemory: root, RelatedMemories: []ChainItem{}}
	for rows.Next() {
		var it ChainItem
		var rel string
		var conf float32
		if err := rows.Scan(&it.MemoryID, &it.Content, &rel, &conf); err != nil {
			return nil, err
		}
		it.RelationType, it.Confidence = &rel, &conf
		chain.RelatedMemories = append(chain.RelatedMemories, it)
	}
	return chain, rows.Err()
}

// --- Stats ----------------------------------------------------------------------

type TemporalStats struct {
	TotalMemories              int64            `json:"total_memories"`
	MemoriesToday              int64            `json:"memories_today"`
	MemoriesThisWeek           int64            `json:"memories_this_week"`
	MemoriesThisMonth          int64            `json:"memories_this_month"`
	AverageDaysBetweenMemories float64          `json:"average_days_between_memories"`
	MemoryTypesDistribution    map[string]int64 `json:"memory_types_distribution"`
}

func (e *Engine) TemporalStats(ctx context.Context, agentID string, tenantID uuid.UUID) (*TemporalStats, error) {
	st := &TemporalStats{MemoryTypesDistribution: map[string]int64{}}
	var first, last *time.Time
	err := e.pool.QueryRow(ctx, `SELECT COUNT(*),
			COUNT(*) FILTER (WHERE COALESCE(document_date, created_at) >= NOW() - INTERVAL '1 day'),
			COUNT(*) FILTER (WHERE COALESCE(document_date, created_at) >= NOW() - INTERVAL '7 days'),
			COUNT(*) FILTER (WHERE COALESCE(document_date, created_at) >= NOW() - INTERVAL '30 days'),
			MIN(COALESCE(document_date, created_at)), MAX(COALESCE(document_date, created_at))
		FROM memories WHERE agent_id = $1 AND tenant_id = $2`, agentID, tenantID).
		Scan(&st.TotalMemories, &st.MemoriesToday, &st.MemoriesThisWeek, &st.MemoriesThisMonth, &first, &last)
	if err != nil {
		return nil, err
	}
	if st.TotalMemories > 1 && first != nil && last != nil {
		st.AverageDaysBetweenMemories = float64(int(last.Sub(*first).Hours()/24)) / float64(st.TotalMemories)
	}
	rows, err := e.pool.Query(ctx, `SELECT memory_type, COUNT(*) FROM memories WHERE agent_id = $1 AND tenant_id = $2 GROUP BY memory_type`, agentID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var n int64
		if err := rows.Scan(&t, &n); err != nil {
			return nil, err
		}
		st.MemoryTypesDistribution[t] = n
	}
	return st, rows.Err()
}

// ConflictAnalysis summarizes the contradictions Evolve has recorded.
type ConflictAnalysis struct {
	TotalConflicts        int64   `json:"total_conflicts"`
	UnresolvedConflicts   int64   `json:"unresolved_conflicts"`
	AvgResolutionTimeSecs float64 `json:"avg_resolution_time_sec"`
}

func (e *Engine) AnalyzeConflicts(ctx context.Context, agentID string, tenantID uuid.UUID) (*ConflictAnalysis, error) {
	var a ConflictAnalysis
	// A conflict counts as unresolved while both sides are still trusted.
	err := e.pool.QueryRow(ctx, `SELECT COUNT(*),
			COUNT(*) FILTER (WHERE COALESCE(s.confidence, 1.0) > 0.3 AND COALESCE(t.confidence, 1.0) > 0.3)
		FROM memory_relations r
		JOIN memories s ON s.id = r.source_memory_id
		JOIN memories t ON t.id = r.target_memory_id
		WHERE r.relation_type = 'updates' AND r.created_by = $1 AND r.tenant_id = $2`, agentID, tenantID).
		Scan(&a.TotalConflicts, &a.UnresolvedConflicts)
	return &a, err
}

// --- Result cache ---------------------------------------------------------------

// ResultCache holds search responses for 5 minutes. Entries for an agent are
// dropped whenever that agent's memories change.
type ResultCache struct {
	mu      sync.Mutex
	entries map[string]cachedResponse
}

type cachedResponse struct {
	resp    *domain.SearchResponse
	agent   string
	expires time.Time
}

const (
	resultCacheTTL = 5 * time.Minute
	resultCacheMax = 1000
)

func NewResultCache() *ResultCache { return &ResultCache{entries: map[string]cachedResponse{}} }

func cacheKey(q domain.SearchQuery, tenantID uuid.UUID) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%s\x00%s", tenantID, q.AgentID, q.Query, q.Limit, q.MemoryType, q.Mode)
}

func (c *ResultCache) Get(q domain.SearchQuery, tenantID uuid.UUID) (*domain.SearchResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[cacheKey(q, tenantID)]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.resp, true
}

func (c *ResultCache) Set(q domain.SearchQuery, tenantID uuid.UUID, r *domain.SearchResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if len(c.entries) >= resultCacheMax {
		for k, e := range c.entries {
			if now.After(e.expires) || len(c.entries) >= resultCacheMax {
				delete(c.entries, k)
			}
		}
	}
	c.entries[cacheKey(q, tenantID)] = cachedResponse{resp: r, agent: q.AgentID, expires: now.Add(resultCacheTTL)}
}

func (c *ResultCache) InvalidateAgent(agentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if e.agent == agentID {
			delete(c.entries, k)
		}
	}
}
