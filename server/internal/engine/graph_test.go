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

	// 4b. Graph neighbours pass the search filters, and the total respects limit.
	claimID, oldID := uuid.New(), uuid.New()
	for _, m := range []struct {
		id       uuid.UUID
		at       time.Time
		metadata string
	}{{claimID, now, `{"action":"claim"}`}, {oldID, now.Add(-100 * 24 * time.Hour), `{}`}} {
		if _, err := pool.Exec(ctx, `INSERT INTO memories (id, agent_id, tenant_id, content, memory_type, visibility, created_at, document_date, last_accessed_at, confidence, metadata)
			VALUES ($1, $2, $3, 'filtered neighbour', 'episodic', 'private', $4, $4, $4, 1.0, $5)`,
			m.id, agentID, tenantID, m.at, m.metadata); err != nil {
			t.Fatalf("insert filtered memory: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id, relation_type, confidence, created_by)
			VALUES ($1, $2, $3, $4, 'associated', 1.0, $5)`, uuid.New(), tenantID, mems[1], m.id, agentID); err != nil {
			t.Fatalf("insert filtered relation: %v", err)
		}
	}
	resFiltered, _ := eng.ExpandGraph(ctx, qBounded, tenantID, hybridResults)
	for _, r := range resFiltered {
		if r.MemoryID == claimID.String() || r.MemoryID == oldID.String() {
			t.Errorf("graph expansion returned %s, which search filters exclude", r.MemoryID)
		}
	}
	qProcedural := qBounded
	qProcedural.MemoryType = "procedural"
	resProcedural, _ := eng.ExpandGraph(ctx, qProcedural, tenantID, hybridResults)
	for _, r := range resProcedural {
		if r.Graph != nil {
			t.Errorf("memory_type=procedural added episodic graph neighbour %s", r.MemoryID)
		}
	}
	for _, limit := range []int{3, 5} {
		qSmall := qBounded
		qSmall.Limit = limit
		resSmall, _ := eng.ExpandGraph(ctx, qSmall, tenantID, hybridResults[:limit])
		if len(resSmall) > limit {
			t.Errorf("limit=%d returned %d results", limit, len(resSmall))
		}
	}

	// 5. Test "rerank" mode: zero new nodes added, degree boost applied
	// Add an edge between mem4 and mem3
	_, err = pool.Exec(ctx, `INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id, relation_type, confidence, created_by)
		VALUES ($1, $2, $3, $4, 'associated', 1.0, $5)`,
		uuid.New(), tenantID, mems[4], mems[3], agentID)
	if err != nil {
		t.Fatalf("insert relation for rerank: %v", err)
	}

	qRerank := domain.SearchQuery{
		AgentID:               agentID,
		Query:                 "Explain relationship between components",
		Limit:                 7,
		GraphMode:             "rerank",
		GraphRetrievalEnabled: true,
	}
	resRerank, diagRerank := eng.ExpandGraph(ctx, qRerank, tenantID, hybridResults)
	if !diagRerank.Applied || diagRerank.Mode != "rerank" {
		t.Fatalf("expected rerank mode applied, got applied=%v mode=%s", diagRerank.Applied, diagRerank.Mode)
	}
	if diagRerank.NodesAdded != 0 {
		t.Errorf("expected 0 nodes added in rerank mode, got %d", diagRerank.NodesAdded)
	}
	if len(resRerank) != len(hybridResults) {
		t.Errorf("expected result length unchanged in rerank mode, got %d vs %d", len(resRerank), len(hybridResults))
	}

	// 6. Test "selective" mode on aggregation query: bypasses graph expansion, reranks only
	qSelectiveAgg := domain.SearchQuery{
		AgentID:               agentID,
		Query:                 "How many times did the server fail?",
		Limit:                 7,
		GraphMode:             "selective",
		GraphRetrievalEnabled: true,
	}
	resSelectiveAgg, diagSelectiveAgg := eng.ExpandGraph(ctx, qSelectiveAgg, tenantID, hybridResults)
	if !diagSelectiveAgg.Applied || diagSelectiveAgg.Mode != "selective" {
		t.Fatalf("expected selective mode applied, got applied=%v mode=%s", diagSelectiveAgg.Applied, diagSelectiveAgg.Mode)
	}
	if diagSelectiveAgg.QueryIntent != "aggregation" {
		t.Errorf("expected query_intent=aggregation, got %s", diagSelectiveAgg.QueryIntent)
	}
	if diagSelectiveAgg.NodesAdded != 0 {
		t.Errorf("expected 0 nodes added for aggregation query in selective mode, got %d", diagSelectiveAgg.NodesAdded)
	}
	if diagSelectiveAgg.Reason != "aggregation_query_rerank_only" {
		t.Errorf("expected reason=aggregation_query_rerank_only, got %s", diagSelectiveAgg.Reason)
	}
	if len(resSelectiveAgg) != len(hybridResults) {
		t.Errorf("expected result length unchanged in selective aggregation, got %d vs %d", len(resSelectiveAgg), len(hybridResults))
	}

	// 7. Test "selective" mode on multi-hop query: applies bounded expansion
	qSelectiveHop := domain.SearchQuery{
		AgentID:               agentID,
		Query:                 "Trace the cause of the database issue",
		Limit:                 7,
		GraphMode:             "selective",
		GraphRetrievalEnabled: true,
	}
	resSelectiveHop, diagSelectiveHop := eng.ExpandGraph(ctx, qSelectiveHop, tenantID, hybridResults)
	if !diagSelectiveHop.Applied || diagSelectiveHop.Mode != "selective" {
		t.Fatalf("expected selective mode applied, got applied=%v mode=%s", diagSelectiveHop.Applied, diagSelectiveHop.Mode)
	}
	if diagSelectiveHop.QueryIntent != "multi_hop" {
		t.Errorf("expected query_intent=multi_hop, got %s", diagSelectiveHop.QueryIntent)
	}
	foundHopMem7 := false
	for _, r := range resSelectiveHop {
		if r.MemoryID == mems[7].String() {
			foundHopMem7 = true
			break
		}
	}
	if !foundHopMem7 {
		t.Errorf("expected selective mode to include terminal path to mem7 on multi-hop query")
	}
}

func TestDetectQueryIntent(t *testing.T) {
	aggCases := []string{
		"How many sessions did Alice attend?",
		"What is the total amount raised?",
		"Count the number of items delivered",
		"How much money was spent on marketing?",
		"List all projects completed in 2025",
		"How often does the worker cycle run?",
	}
	for _, c := range aggCases {
		if intent := engine.DetectQueryIntent(c); intent != "aggregation" {
			t.Errorf("expected aggregation for %q, got %q", c, intent)
		}
	}

	hopCases := []string{
		"Why did the build fail on Tuesday?",
		"What is the capital of France?",
		"Trace the relationship between Auth and Engine",
		"Who is the manager of the DevOps team?",
	}
	for _, c := range hopCases {
		if intent := engine.DetectQueryIntent(c); intent != "multi_hop" {
			t.Errorf("expected multi_hop for %q, got %q", c, intent)
		}
	}
}
