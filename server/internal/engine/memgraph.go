package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/server/internal/metrics"
)

// --- Memory graph (dashboard) ------------------------------------------------
//
// MemoryGraph is the dashboard's 3D view of an agent's memories. Besides the
// stored memory_relations edges it links memories that mention the same
// entity (entity_tags) and consecutive memories from the same conversation,
// because ordinary saves write no edges and the graph would otherwise be a
// cloud of unconnected points.

const (
	GraphViewDefaultLimit = 300
	GraphViewMaxLimit     = 1000
	graphViewMaxEntities  = 80
	graphViewContentChars = 280
)

type GraphViewNode struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"` // memory | entity
	Label      string  `json:"label"`
	Content    string  `json:"content,omitempty"`
	MemoryType string  `json:"memory_type,omitempty"`
	Confidence float32 `json:"confidence,omitempty"`
	SessionID  string  `json:"session_id,omitempty"`
	At         string  `json:"at,omitempty"`
	Val        float32 `json:"val"` // node size
}

type GraphViewLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // a relation type, mentions or same_session
}

type GraphView struct {
	Nodes []GraphViewNode `json:"nodes"`
	Links []GraphViewLink `json:"links"`
	// Truncated is true when the agent has more memories than limit.
	Truncated bool `json:"truncated"`
}

type graphViewMemory struct {
	ID         uuid.UUID
	Content    string
	MemoryType string
	Confidence float32
	Tags       []string
	SessionID  *uuid.UUID
	At         time.Time
}

type graphViewRelation struct {
	From, To uuid.UUID
	Type     string
}

// MemoryGraph returns the agent's newest memories (at most limit) as a graph.
func (e *Engine) MemoryGraph(ctx context.Context, agentID string, tenantID uuid.UUID, limit int) (*GraphView, error) {
	if limit <= 0 {
		limit = GraphViewDefaultLimit
	}
	limit = min(limit, GraphViewMaxLimit)
	if g, ok := e.cache.GetGraph(agentID, tenantID, limit); ok {
		metrics.GraphViewCacheHits.Inc()
		return g, nil
	}
	metrics.GraphViewCacheMisses.Inc()
	rows, err := e.pool.Query(ctx, `SELECT id, content, memory_type, COALESCE(confidence, 1.0)::REAL,
			COALESCE(entity_tags, '{}'), session_id, COALESCE(event_date, document_date, created_at)
		FROM memories
		WHERE agent_id = $1 AND tenant_id = $2
		  AND (metadata->>'action' IS NULL OR metadata->>'action' NOT IN ('claim', 'release', 'intent', 'handoff'))
		ORDER BY created_at DESC LIMIT $3`, agentID, tenantID, limit+1)
	if err != nil {
		return nil, err
	}
	var mems []graphViewMemory
	for rows.Next() {
		var m graphViewMemory
		if err := rows.Scan(&m.ID, &m.Content, &m.MemoryType, &m.Confidence, &m.Tags, &m.SessionID, &m.At); err != nil {
			rows.Close()
			return nil, err
		}
		mems = append(mems, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	truncated := len(mems) > limit
	if truncated {
		mems = mems[:limit]
	}

	ids := make([]uuid.UUID, len(mems))
	for i, m := range mems {
		ids[i] = m.ID
	}
	var rels []graphViewRelation
	if len(ids) > 0 {
		rrows, err := e.pool.Query(ctx, `SELECT source_memory_id, target_memory_id, relation_type FROM memory_relations
			WHERE tenant_id = $1 AND source_memory_id = ANY($2) AND target_memory_id = ANY($2)`, tenantID, ids)
		if err != nil {
			return nil, err
		}
		for rrows.Next() {
			var r graphViewRelation
			if err := rrows.Scan(&r.From, &r.To, &r.Type); err != nil {
				rrows.Close()
				return nil, err
			}
			rels = append(rels, r)
		}
		rrows.Close()
		if err := rrows.Err(); err != nil {
			return nil, err
		}
	}
	g := buildGraphView(mems, rels)
	g.Truncated = truncated
	e.cache.SetGraph(agentID, tenantID, limit, g)
	return g, nil
}

// buildGraphView turns memories and their relations into nodes and links.
// Entities mentioned by only one memory are left out: they add a node without
// connecting anything.
func buildGraphView(mems []graphViewMemory, rels []graphViewRelation) *GraphView {
	g := &GraphView{Nodes: []GraphViewNode{}, Links: []GraphViewLink{}}
	for _, m := range mems {
		n := GraphViewNode{
			ID: m.ID.String(), Kind: "memory", Label: truncateRunes(m.Content, 48),
			Content: truncateRunes(m.Content, graphViewContentChars), MemoryType: m.MemoryType,
			Confidence: m.Confidence, Val: 1 + 2*m.Confidence,
		}
		if !m.At.IsZero() {
			n.At = m.At.UTC().Format(time.RFC3339)
		}
		if m.SessionID != nil {
			n.SessionID = m.SessionID.String()
		}
		g.Nodes = append(g.Nodes, n)
	}

	for _, r := range rels {
		g.Links = append(g.Links, GraphViewLink{Source: r.From.String(), Target: r.To.String(), Type: r.Type})
	}

	// Entities shared by at least two memories, most mentioned first.
	mentions := map[string][]string{}
	for _, m := range mems {
		seen := map[string]bool{}
		for _, t := range m.Tags {
			t = strings.ToLower(strings.TrimSpace(t))
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			mentions[t] = append(mentions[t], m.ID.String())
		}
	}
	var entities []string
	for t, ms := range mentions {
		if len(ms) >= 2 {
			entities = append(entities, t)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		if a, b := len(mentions[entities[i]]), len(mentions[entities[j]]); a != b {
			return a > b
		}
		return entities[i] < entities[j]
	})
	if len(entities) > graphViewMaxEntities {
		entities = entities[:graphViewMaxEntities]
	}
	for _, t := range entities {
		id := "entity:" + t
		g.Nodes = append(g.Nodes, GraphViewNode{ID: id, Kind: "entity", Label: t, Val: 1 + float32(len(mentions[t]))/2})
		for _, mid := range mentions[t] {
			g.Links = append(g.Links, GraphViewLink{Source: mid, Target: id, Type: "mentions"})
		}
	}

	// Consecutive memories of one conversation, oldest first: a chain, not a
	// clique, so long conversations don't turn into a hairball.
	sessions := map[uuid.UUID][]graphViewMemory{}
	for _, m := range mems {
		if m.SessionID != nil {
			sessions[*m.SessionID] = append(sessions[*m.SessionID], m)
		}
	}
	for _, ms := range sessions {
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].At.Before(ms[j].At) })
		for i := 1; i < len(ms); i++ {
			g.Links = append(g.Links, GraphViewLink{Source: ms[i-1].ID.String(), Target: ms[i].ID.String(), Type: "same_session"})
		}
	}
	sort.SliceStable(g.Links, func(i, j int) bool {
		a, b := g.Links[i], g.Links[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Target < b.Target
	})
	return g
}

func truncateRunes(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
