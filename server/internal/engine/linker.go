package engine

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/metrics"
)

// ubiquitousEntityTags are ignored during save-time entity linking to prevent
// non-specific star-graphs from chat turns.
var ubiquitousEntityTags = map[string]bool{
	"user": true, "assistant": true, "ai": true, "human": true, "system": true,
	"chat": true, "session": true, "turn": true,
}

// LinkerConfig configures the save-time graph edge linker.
type LinkerConfig struct {
	Enabled       bool
	Timeout       time.Duration // default 200ms
	MaxTags       int           // max entities inspected (default 5)
	MaxCandidates int           // max candidate memories evaluated (default 8)
}

func DefaultLinkerConfig() LinkerConfig {
	return LinkerConfig{
		Enabled:       false,
		Timeout:       200 * time.Millisecond,
		MaxTags:       5,
		MaxCandidates: 8,
	}
}

// LinkOnSave evaluates a newly inserted memory against existing memories of the
// same agent and tenant, creating 'associated' or 'updates' edges.
// It is strictly best-effort, bounded by a 200ms deadline, and never returns an error
// that would fail a memory write.
func (e *Engine) LinkOnSave(ctx context.Context, m *domain.Memory, cfg LinkerConfig) {
	if !cfg.Enabled || m == nil || len(m.EntityTags) == 0 {
		return
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 200 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := e.linkEntities(ctx, m, cfg); err != nil {
		slog.Warn("save-time graph linking failed (best-effort)", "memory_id", m.ID, "err", err)
	}
}

type candidateMemory struct {
	ID           uuid.UUID
	Content      string
	CreatedAt    time.Time
	DocumentDate time.Time
	EventDate    *time.Time
	EntityTags   []string
}

func (e *Engine) linkEntities(ctx context.Context, m *domain.Memory, cfg LinkerConfig) error {
	var specificTags []string
	for _, tag := range m.EntityTags {
		t := strings.ToLower(strings.TrimSpace(tag))
		if t == "" || ubiquitousEntityTags[t] {
			continue
		}
		if !slices.Contains(specificTags, t) {
			specificTags = append(specificTags, t)
		}
		if len(specificTags) >= cfg.MaxTags {
			break
		}
	}
	if len(specificTags) == 0 {
		return nil
	}

	maxCand := cfg.MaxCandidates
	if maxCand <= 0 {
		maxCand = 8
	}

	// Find most recent memories of this agent sharing any of the specific entity tags.
	rows, err := e.pool.Query(ctx, `
		SELECT id, content, created_at, document_date, event_date, entity_tags
		FROM memories
		WHERE agent_id = $1 AND tenant_id = $2 AND id != $3
		  AND entity_tags && $4
		ORDER BY created_at DESC
		LIMIT $5`, m.AgentID, m.TenantID, m.ID, specificTags, maxCand)
	if err != nil {
		return err
	}
	defer rows.Close()

	var candidates []candidateMemory
	for rows.Next() {
		var c candidateMemory
		if err := rows.Scan(&c.ID, &c.Content, &c.CreatedAt, &c.DocumentDate, &c.EventDate, &c.EntityTags); err != nil {
			return err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, cand := range candidates {
		// Calculate shared specific entities
		var shared []string
		for _, t := range cand.EntityTags {
			tl := strings.ToLower(strings.TrimSpace(t))
			if slices.Contains(specificTags, tl) && !slices.Contains(shared, tl) {
				shared = append(shared, tl)
			}
		}
		if len(shared) == 0 {
			continue
		}

		// Check for contradiction
		conf := DetectContradiction(m.Content, cand.Content)
		if conf >= 0.7 {
			// Updates edge: directed newer -> older
			newerID, olderID := determineTemporalOrder(m, cand)
			meta, _ := json.Marshal(map[string]any{
				"basis":           "contradiction",
				"confidence":      conf,
				"shared_entities": shared,
			})
			if err := e.insertEdge(ctx, m.TenantID, newerID, olderID, domain.RelUpdates, conf, meta, m.AgentID); err != nil {
				return err
			}
			metrics.GraphEdgesCreated.WithLabelValues("updates").Inc()
		} else {
			// Associated edge: undirected, stored canonically (smaller UUID -> larger UUID)
			srcID, tgtID := m.ID, cand.ID
			if srcID.String() > tgtID.String() {
				srcID, tgtID = tgtID, srcID
			}
			edgeConf := 0.5 + 0.1*float32(len(shared)-1)
			if edgeConf > 0.8 {
				edgeConf = 0.8
			}
			meta, _ := json.Marshal(map[string]any{
				"basis":           "entity",
				"shared_entities": shared,
			})
			if err := e.insertEdge(ctx, m.TenantID, srcID, tgtID, domain.RelAssociated, edgeConf, meta, m.AgentID); err != nil {
				return err
			}
			metrics.GraphEdgesCreated.WithLabelValues("associated").Inc()
		}
	}
	return nil
}

func (e *Engine) insertEdge(ctx context.Context, tenantID uuid.UUID, src, tgt uuid.UUID, rel domain.RelationType, conf float32, meta json.RawMessage, createdBy string) error {
	_, err := e.pool.Exec(ctx, `
		INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id, relation_type, confidence, metadata, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (source_memory_id, target_memory_id, relation_type) DO NOTHING`,
		uuid.New(), tenantID, src, tgt, string(rel), conf, meta, createdBy)
	return err
}

func determineTemporalOrder(m *domain.Memory, cand candidateMemory) (newer, older uuid.UUID) {
	timeM := memoryTime(m.EventDate, m.DocumentDate, m.CreatedAt)
	timeCand := memoryTime(cand.EventDate, cand.DocumentDate, cand.CreatedAt)
	if timeM.After(timeCand) {
		return m.ID, cand.ID
	}
	return cand.ID, m.ID
}

func memoryTime(eventDate *time.Time, docDate, createdAt time.Time) time.Time {
	if eventDate != nil && !eventDate.IsZero() {
		return *eventDate
	}
	if !docDate.IsZero() {
		return docDate
	}
	return createdAt
}
