package engine_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/memrohq/memro/server/internal/db"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/migrations"
)

// Evolve must be safe to run repeatedly: episodes are consolidated once, and
// a conflict between two facts is linked once.
func TestEvolveIsIdempotent(t *testing.T) {
	url := os.Getenv("MEMRO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MEMRO_TEST_DATABASE_URL not set; skipping evolve test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, url, 5, 1)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	eng := engine.New(pool, nil, dummyIndexer{})

	tenantID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, name, created_at, updated_at) VALUES ($1, 'Evolve Test Tenant', NOW(), NOW())`, tenantID); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	agentID := "evolve-agent-" + uuid.NewString()[:8]
	if _, err := pool.Exec(ctx, `INSERT INTO agent_identities (id, tenant_id, created_at) VALUES ($1, $2, NOW())`, agentID, tenantID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	insert := func(content, memType string, at time.Time) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO memories (id, agent_id, tenant_id, content, memory_type, visibility, created_at, document_date, entity_tags, confidence, last_accessed_at)
			VALUES ($1, $2, $3, $4, $5, 'private', $6, $6, '{}', 1.0, NOW())`, uuid.New(), agentID, tenantID, content, memType, at); err != nil {
			t.Fatalf("insert memory: %v", err)
		}
	}
	count := func(query string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, query, agentID, tenantID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	summaries := `SELECT count(*) FROM memories WHERE agent_id = $1 AND tenant_id = $2 AND content LIKE 'Consolidated Summary:%'`
	updates := `SELECT count(*) FROM memory_relations r JOIN memories m ON m.id = r.source_memory_id
		WHERE m.agent_id = $1 AND m.tenant_id = $2 AND r.relation_type = 'updates'`

	now := time.Now()
	for i, c := range []string{"Deployed the billing service", "Rolled back the billing deploy", "Fixed the billing migration"} {
		insert(c, "episodic", now.Add(time.Duration(i-10)*time.Minute))
	}
	insert("The billing contact is Marco", "semantic", now.Add(-48*time.Hour))
	insert("The billing contact is Ana", "semantic", now.Add(-time.Hour))

	for run := 1; run <= 3; run++ {
		if _, err := eng.Evolve(ctx, agentID, tenantID); err != nil {
			t.Fatalf("evolve run %d: %v", run, err)
		}
		if n := count(summaries); n != 1 {
			t.Fatalf("after run %d: %d consolidated summaries, want 1 (re-runs must not re-summarise)", run, n)
		}
		if n := count(updates); n != 1 {
			t.Fatalf("after run %d: %d updates edges, want 1", run, n)
		}
	}

	// New episodes get their own summary, without the ones already consolidated.
	for i, c := range []string{"Paired on the search ranking", "Shipped the ranking fix", "Wrote the ranking benchmark"} {
		insert(c, "episodic", now.Add(time.Duration(i-3)*time.Minute))
	}
	if _, err := eng.Evolve(ctx, agentID, tenantID); err != nil {
		t.Fatalf("evolve with new episodes: %v", err)
	}
	if n := count(summaries); n != 2 {
		t.Fatalf("%d consolidated summaries, want 2", n)
	}
	var latest string
	if err := pool.QueryRow(ctx, `SELECT content FROM memories WHERE agent_id = $1 AND tenant_id = $2
		AND content LIKE 'Consolidated Summary:%' ORDER BY created_at DESC LIMIT 1`, agentID, tenantID).Scan(&latest); err != nil {
		t.Fatalf("latest summary: %v", err)
	}
	if want := "Consolidated Summary: Paired on the search ranking; Shipped the ranking fix; Wrote the ranking benchmark"; latest != want {
		t.Fatalf("second summary = %q, want only the new episodes: %q", latest, want)
	}
}
