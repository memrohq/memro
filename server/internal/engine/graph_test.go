package engine_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/memrohq/memro/server/internal/db"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/migrations"
)

func TestExpandGraph(t *testing.T) {
	url := os.Getenv("MEMRO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MEMRO_TEST_DATABASE_URL not set; skipping graph test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, url, 5, 1)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	eng := engine.New(pool, nil, dummyIndexer{})

	tenantID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO tenants (id, name, created_at, updated_at) VALUES ($1, 'Graph Test Tenant', NOW(), NOW())`, tenantID)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)

	agentID := "test-agent-" + uuid.NewString()[:8]
	_, err = pool.Exec(ctx, `INSERT INTO agent_identities (id, tenant_id, created_at) VALUES ($1, $2, NOW())`, agentID, tenantID)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	// Create 8 memories:
	// mem0..mem4: top 5 hybrid results
	// mem5, mem6: lower-rank hybrid results
	// mem7: connected to mem0 via short 1-hop terminal edge (not in hybrid results)
	now := time.Now().UTC()
	mems := make([]uuid.UUID, 8)
	for i := range mems {
		mems[i] = uuid.New()
		_, err := pool.Exec(ctx, `INSERT INTO memories (id, agent_id, tenant_id, content, memory_type, visibility, created_at, document_date, last_accessed_at, confidence)
			VALUES ($1, $2, $3, $4, 'episodic', 'private', $5, $5, $5, 1.0)`,
			mems[i], agentID, tenantID, fmt.Sprintf("Memory number %d content", i), now.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatalf("insert memory %d: %v", i, err)
		}
	}

	// Edge: mem0 -> mem7 ('updates', confidence 0.9). mem7 has no outgoing edges (short terminal path).
	_, err = pool.Exec(ctx, `INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id, relation_type, confidence, created_by)
		VALUES ($1, $2, $3, $4, 'updates', 0.9, $5)`,
		uuid.New(), tenantID, mems[0], mems[7], agentID)
	if err != nil {
		t.Fatalf("insert relation: %v", err)
	}

	hybridResults := make([]domain.SearchResult, 7)
	for i := 0; i < 7; i++ {
		hybridResults[i] = domain.SearchResult{
			MemoryID:   mems[i].String(),
			Content:    fmt.Sprintf("Memory number %d content", i),
			Score:      float32(10 - i),
			MemoryType: "episodic",
		}
	}

	// 1. When GraphRetrievalEnabled = false, returns unchanged results with reason="disabled"
	qDisabled := domain.SearchQuery{
		AgentID:               agentID,
		Limit:                 7,
		GraphMode:             "bounded",
		GraphRetrievalEnabled: false,
	}
	resDisabled, diagDisabled := eng.ExpandGraph(ctx, qDisabled, tenantID, hybridResults)
	if len(resDisabled) != len(hybridResults) || diagDisabled.Applied {
		t.Errorf("expected disabled graph to return unchanged results, got applied=%v", diagDisabled.Applied)
	}

	// 2. Test "legacy" mode: drops short 1-hop terminal path when maxDepth=2
	qLegacy := domain.SearchQuery{
		AgentID:               agentID,
		Limit:                 7,
		GraphMode:             "legacy",
		GraphRetrievalEnabled: true,
	}
	resLegacy, diagLegacy := eng.ExpandGraph(ctx, qLegacy, tenantID, hybridResults)
	if !diagLegacy.Applied {
		t.Fatalf("expected legacy graph applied, got applied=%v reason=%s", diagLegacy.Applied, diagLegacy.Reason)
	}
	// In legacy mode, 1-hop walk to mem7 is dropped because depth < 2
	for _, r := range resLegacy {
		if r.MemoryID == mems[7].String() {
			t.Errorf("legacy mode should have dropped 1-hop terminal path to mem7, but found it in results")
		}
	}

	// 3. Test "bounded" mode (PR #43): preserves short 1-hop terminal path to mem7!
	qBounded := domain.SearchQuery{
		AgentID:               agentID,
		Limit:                 7,
		GraphMode:             "bounded",
		GraphRetrievalEnabled: true,
	}
	resBounded, diagBounded := eng.ExpandGraph(ctx, qBounded, tenantID, hybridResults)
	if !diagBounded.Applied {
		t.Fatalf("expected bounded graph applied, got applied=%v reason=%s", diagBounded.Applied, diagBounded.Reason)
	}
	foundMem7 := false
	for _, r := range resBounded {
		if r.MemoryID == mems[7].String() {
			foundMem7 = true
			if r.Graph == nil {
				t.Error("expected r.Graph metadata on graph result, got nil")
			} else {
				if r.Graph.SeedID != mems[0].String() {
					t.Errorf("expected seed_id=%s, got %s", mems[0], r.Graph.SeedID)
				}
				if r.Graph.Hops != 1 {
					t.Errorf("expected hops=1, got %d", r.Graph.Hops)
				}
			}
		}
	}
	if !foundMem7 {
		t.Errorf("expected bounded mode (PR #43) to include short terminal path to mem7, but it was missing")
	}

	// 4. Verify Recall@5 protection: top 5 hybrid results (mem0..mem4) must NOT be displaced
	top5IDs := map[string]bool{}
	for i := 0; i < 5; i++ {
		top5IDs[mems[i].String()] = true
	}
	for i := 0; i < 5; i++ {
		if !top5IDs[resBounded[i].MemoryID] {
			t.Errorf("top 5 hybrid slot %d displaced: expected one of top 5, got %s", i, resBounded[i].MemoryID)
		}
	}
}
