// Package indexer embeds memories in the background and keeps the knowledge
// graph tidy.
package indexer

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/memrohq/memro/backend/internal/embed"
	"github.com/memrohq/memro/backend/internal/metrics"
	"github.com/memrohq/memro/backend/internal/store"
)

const (
	batchSize     = 100
	flushInterval = 200 * time.Millisecond
	queueCapacity = 10_000
	workers       = 4
	backfillEvery = 2 * time.Minute
)

type task struct {
	memoryID uuid.UUID
	agentID  string
	tenantID uuid.UUID
	content  string
}

// Indexer batches embedding requests. Memories that fail to embed (or are
// dropped when the queue is full) are picked up by the periodic backfill,
// so nothing is lost permanently.
type Indexer struct {
	store    *store.Store
	provider *embed.Provider
	queue    chan task
	wg       sync.WaitGroup
}

func New(s *store.Store, p *embed.Provider) *Indexer {
	return &Indexer{store: s, provider: p, queue: make(chan task, queueCapacity)}
}

func (ix *Indexer) Enqueue(memoryID uuid.UUID, agentID string, tenantID uuid.UUID, content string) {
	if !ix.provider.Enabled() {
		return
	}
	select {
	case ix.queue <- task{memoryID, agentID, tenantID, content}:
		metrics.EmbeddingQueue.Inc()
	default:
		slog.Warn("embedding queue full; memory will be embedded by backfill", "memory_id", memoryID)
	}
}

// Run processes the queue until ctx is cancelled.
func (ix *Indexer) Run(ctx context.Context) {
	if !ix.provider.Enabled() {
		slog.Warn("no embedding API key configured; semantic search disabled, keyword search only")
		return
	}
	batches := make(chan []task)
	for range workers {
		ix.wg.Add(1)
		go func() {
			defer ix.wg.Done()
			for b := range batches {
				ix.process(ctx, b)
			}
		}()
	}

	backfill := time.NewTicker(backfillEvery)
	defer backfill.Stop()
	flush := time.NewTicker(flushInterval)
	defer flush.Stop()
	ix.backfill(ctx)

	var pending []task
	send := func() {
		if len(pending) > 0 {
			batches <- pending
			pending = nil
		}
	}
	for {
		select {
		case <-ctx.Done():
			close(batches)
			ix.wg.Wait()
			return
		case t := <-ix.queue:
			pending = append(pending, t)
			if len(pending) >= batchSize {
				send()
			}
		case <-flush.C:
			send()
		case <-backfill.C:
			ix.backfill(ctx)
		}
	}
}

func (ix *Indexer) process(ctx context.Context, tasks []task) {
	defer metrics.EmbeddingQueue.Sub(float64(len(tasks)))
	texts := make([]string, len(tasks))
	for i, t := range tasks {
		texts[i] = t.content
	}
	cctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	vecs, err := ix.provider.EmbedBatch(cctx, texts)
	if err != nil {
		metrics.EmbeddingFailures.Inc()
		slog.Error("embedding batch failed; will retry on backfill", "size", len(tasks), "err", err)
		return
	}
	for i, t := range tasks {
		if err := ix.store.UpsertEmbedding(cctx, t.memoryID, t.tenantID, t.agentID, ix.provider.Model(), vecs[i]); err != nil {
			slog.Error("store embedding failed", "memory_id", t.memoryID, "err", err)
		}
	}
}

// backfill queues memories that have no embedding for the current model.
func (ix *Indexer) backfill(ctx context.Context) {
	pending, err := ix.store.MemoriesNeedingEmbedding(ctx, ix.provider.Model(), batchSize*10)
	if err != nil {
		slog.Error("embedding backfill query failed", "err", err)
		return
	}
	for _, p := range pending {
		ix.Enqueue(p.MemoryID, p.AgentID, p.TenantID, p.Content)
	}
	if len(pending) > 0 {
		slog.Info("queued memories for embedding backfill", "count", len(pending))
	}
}

// RunGraphMaintenance refreshes memories.relationship_types and prunes weak
// or decayed relations every 5 minutes.
func RunGraphMaintenance(ctx context.Context, s *store.Store) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if _, err := s.Pool.Exec(ctx, `UPDATE memories m SET relationship_types = COALESCE(r.types, '{}')
			FROM (SELECT target_memory_id AS id, ARRAY_AGG(DISTINCT relation_type) AS types
			      FROM memory_relations GROUP BY target_memory_id) r
			WHERE m.id = r.id AND m.relationship_types IS DISTINCT FROM r.types`); err != nil {
			slog.Warn("relationship type refresh failed", "err", err)
		}
		// Edges lose 0.05 confidence per day of age and are dropped below 0.1.
		if _, err := s.Pool.Exec(ctx, `DELETE FROM memory_relations WHERE COALESCE(confidence, 1.0) < 0.2
			OR (COALESCE(confidence, 1.0) - EXTRACT(EPOCH FROM (NOW() - created_at)) / 86400.0 * 0.05) < 0.1`); err != nil {
			slog.Warn("relation pruning failed", "err", err)
		}
	}
}
