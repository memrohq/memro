package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/extract"
	"github.com/memrohq/memro/server/internal/hooks"
	"github.com/memrohq/memro/server/internal/metrics"
)

// item is one external thing rendered as text, ready to become memories.
type item struct {
	Key      string // unique within the connection
	SourceID string
	// Header goes in front of every chunk so each memory says where it came
	// from, e.g. "[Slack #eng · 2026-10-20]".
	Header    string
	Body      string
	Type      domain.MemoryType
	EventDate *time.Time
	Meta      map[string]any
}

// quotaError stops a sync: the agent cannot store more memories.
type quotaError struct{ msg string }

func (e *quotaError) Error() string { return e.msg }

type chunk struct {
	content, hash string
}

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:16])
}

func (it *item) chunks() []chunk {
	parts := extract.ChunkText(it.Body, extract.DefaultChunkSize, extract.DefaultChunkOverlap)
	out := make([]chunk, 0, len(parts))
	for _, p := range parts {
		c := p
		if it.Header != "" {
			c = it.Header + "\n" + p
		}
		out = append(out, chunk{c, hashOf(c)})
	}
	return out
}

type storedMemory struct {
	id   uuid.UUID
	hash string
}

// storeItem makes the item's memories match its current content. Only
// chunks that changed are deleted or inserted, so unchanged text keeps its
// memory and embedding. A chunk the user deleted is not stored again unless
// its text changes.
func (m *Manager) storeItem(ctx context.Context, c *conn, it item) error {
	meta := map[string]any{"source": c.Provider, "connector": true, "connection_id": c.ID.String()}
	for k, v := range it.Meta {
		meta[k] = v
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	chunks := it.chunks()
	hashes := make([]string, len(chunks))
	for i, ch := range chunks {
		hashes[i] = ch.hash
	}
	contentHash := hashOf(strings.Join(hashes, ",") + "|" + string(it.Type) + "|" + string(metaJSON))

	tx, err := m.deps.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	prevHash, prevChunks, err := lockItem(ctx, tx, c.ID, it.Key, it.SourceID)
	if err != nil {
		return err
	}
	if prevHash == contentHash {
		return tx.Commit(ctx)
	}

	existing, err := itemMemories(ctx, tx, c.ID, it.Key)
	if err != nil {
		return err
	}
	byHash := map[string][]uuid.UUID{}
	present := map[string]bool{}
	for _, e := range existing {
		byHash[e.hash] = append(byHash[e.hash], e.id)
		present[e.hash] = true
	}
	deletedByUser := map[string]bool{}
	for _, h := range prevChunks {
		if !present[h] {
			deletedByUser[h] = true
		}
	}

	var insert []chunk
	for _, ch := range chunks {
		if ids := byHash[ch.hash]; len(ids) > 0 {
			byHash[ch.hash] = ids[1:]
			continue
		}
		if deletedByUser[ch.hash] {
			continue
		}
		insert = append(insert, ch)
	}
	var remove []uuid.UUID
	for _, ids := range byHash {
		remove = append(remove, ids...)
	}

	if net := len(insert) - len(remove); net > 0 {
		var current int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM memories WHERE agent_id = $1`, c.AgentID).Scan(&current); err != nil {
			return err
		}
		if err := m.deps.Quota.AllowMemoryWrite(ctx, c.TenantID, c.AgentID, current, net); err != nil {
			if errors.Is(err, extension.ErrQuotaExceeded) {
				return &quotaError{err.Error()}
			}
			return err
		}
	}

	if len(remove) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM memories WHERE id = ANY($1) AND connection_id = $2`, remove, c.ID); err != nil {
			return err
		}
	}
	now := m.now().UTC()
	created := make([]domain.Memory, 0, len(insert))
	for _, ch := range insert {
		tags := append(extract.Entities(ch.content), ReferenceTags(ch.content)...)
		mem := domain.Memory{ID: uuid.New(), AgentID: c.AgentID, TenantID: c.TenantID, Content: ch.content,
			MemoryType: it.Type, Visibility: domain.Private, CreatedAt: now, DocumentDate: now, EventDate: it.EventDate,
			LastAccessedAt: now, Confidence: 1, EntityTags: tags, RelationshipTypes: []string{},
			Metadata: metaJSON}
		if _, err := tx.Exec(ctx, `INSERT INTO memories (id, agent_id, tenant_id, content, memory_type, visibility,
				created_at, document_date, event_date, is_atomic, entity_tags, relationship_types, access_count,
				confidence, last_accessed_at, metadata, connection_id, source_item)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,false,$10,$11,0,$12,$13,$14,$15,$16)`,
			mem.ID, mem.AgentID, mem.TenantID, mem.Content, string(mem.MemoryType), string(mem.Visibility),
			mem.CreatedAt, mem.DocumentDate, mem.EventDate, mem.EntityTags, mem.RelationshipTypes, mem.Confidence,
			mem.LastAccessedAt, metaJSON, c.ID, it.Key); err != nil {
			return err
		}
		created = append(created, mem)
	}
	if len(created) > 0 {
		_ = m.linkCrossReferences(ctx, tx, c, created)
	}
	if _, err := tx.Exec(ctx, `UPDATE connector_items SET content_hash = $3, chunk_hashes = $4, updated_at = NOW()
		WHERE connection_id = $1 AND item_key = $2`, c.ID, it.Key, contentHash, hashes); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	m.afterWrite(ctx, c, created, len(remove))
	return nil
}

// removeItem deletes an item that no longer exists at the provider.
func (m *Manager) removeItem(ctx context.Context, c *conn, key string) error {
	tag, err := m.deps.Pool.Exec(ctx, `DELETE FROM memories WHERE connection_id = $1 AND source_item = $2`, c.ID, key)
	if err != nil {
		return err
	}
	if _, err := m.deps.Pool.Exec(ctx, `DELETE FROM connector_items WHERE connection_id = $1 AND item_key = $2`, c.ID, key); err != nil {
		return err
	}
	m.afterWrite(ctx, c, nil, int(tag.RowsAffected()))
	return nil
}

func (m *Manager) afterWrite(ctx context.Context, c *conn, created []domain.Memory, removed int) {
	if len(created) == 0 && removed == 0 {
		return
	}
	for _, mem := range created {
		m.deps.Indexer.Enqueue(mem.ID, mem.AgentID, mem.TenantID, mem.Content)
	}
	if m.deps.Invalidate != nil {
		m.deps.Invalidate(c.AgentID)
	}
	metrics.MemoriesCreated.Add(float64(len(created)))
	metrics.MemoriesDeleted.Add(float64(removed))
	metrics.ConnectorMemories.WithLabelValues(c.Provider, "created").Add(float64(len(created)))
	metrics.ConnectorMemories.WithLabelValues(c.Provider, "deleted").Add(float64(removed))
	if len(created) == 0 {
		return
	}
	if m.deps.Usage != nil {
		if err := m.deps.Usage.Record(ctx, extension.UsageEvent{TenantID: c.TenantID, AgentID: c.AgentID,
			Metric: extension.MetricMemoriesStored, Value: int64(len(created))}); err != nil {
			slog.Warn("record connector usage", "err", err)
		}
	}
	if urls, err := m.deps.Store.GetHooks(ctx, c.AgentID, "memory_created"); err == nil && len(urls) > 0 {
		for _, mem := range created {
			hooks.Notify(urls, "memory_created", map[string]any{
				"event": "memory_created", "memory_id": mem.ID, "agent_id": mem.AgentID,
				"content": mem.Content, "memory_type": mem.MemoryType, "source": c.Provider,
			})
		}
	}
}

// lockItem creates the item row if needed and locks it for this transaction,
// so concurrent jobs for one item apply one after the other.
func lockItem(ctx context.Context, tx pgx.Tx, connID uuid.UUID, key, sourceID string) (string, []string, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO connector_items (connection_id, item_key, source_id) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, connID, key, sourceID); err != nil {
		return "", nil, err
	}
	var hash string
	var chunks []string
	err := tx.QueryRow(ctx, `SELECT content_hash, chunk_hashes FROM connector_items
		WHERE connection_id = $1 AND item_key = $2 FOR UPDATE`, connID, key).Scan(&hash, &chunks)
	return hash, chunks, err
}

func itemMemories(ctx context.Context, tx pgx.Tx, connID uuid.UUID, key string) ([]storedMemory, error) {
	rows, err := tx.Query(ctx, `SELECT id, content FROM memories WHERE connection_id = $1 AND source_item = $2`, connID, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storedMemory
	for rows.Next() {
		var id uuid.UUID
		var content string
		if err := rows.Scan(&id, &content); err != nil {
			return nil, err
		}
		out = append(out, storedMemory{id, hashOf(content)})
	}
	return out, rows.Err()
}
