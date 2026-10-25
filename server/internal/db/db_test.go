package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/memrohq/memro/server/internal/db"
	"github.com/memrohq/memro/server/migrations"
)

func TestMigration0031RelationTypesAndUniqueness(t *testing.T) {
	url := os.Getenv("MEMRO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MEMRO_TEST_DATABASE_URL not set; skipping database migration test")
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

	// Create test tenant, agent and memories
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

	mem1 := uuid.New()
	mem2 := uuid.New()
	mem3 := uuid.New()
	now := time.Now().UTC()
	for _, id := range []uuid.UUID{mem1, mem2, mem3} {
		_, err := pool.Exec(ctx, `INSERT INTO memories (id, agent_id, tenant_id, content, memory_type, visibility, created_at, document_date, last_accessed_at, confidence)
			VALUES ($1, $2, $3, 'test memory content', 'episodic', 'private', $4, $4, $4, 1.0)`, id, agentID, tenantID, now)
		if err != nil {
			t.Fatalf("insert memory %s: %v", id, err)
		}
	}

	// 1. Verify associated and causes relation types can be inserted
	for _, relType := range []string{"associated", "causes", "updates", "extends", "derives"} {
		relID := uuid.New()
		_, err := pool.Exec(ctx, `INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id, relation_type, confidence, created_by)
			VALUES ($1, $2, $3, $4, $5, 0.8, $6)`, relID, tenantID, mem1, mem2, relType, agentID)
		if err != nil {
			t.Errorf("failed to insert relation_type '%s': %v", relType, err)
		}
	}

	// 2. Verify invalid relation type is rejected by CHECK constraint
	invalidRelID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id, relation_type, confidence, created_by)
		VALUES ($1, $2, $3, $4, 'invalid_rel_type', 0.8, $5)`, invalidRelID, tenantID, mem1, mem2, agentID)
	if err == nil {
		t.Error("expected error inserting invalid relation_type, but got nil")
	}

	// 3. Verify edge uniqueness constraint prevents duplicates of same (source, target, relation_type)
	dupRelID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id, relation_type, confidence, created_by)
		VALUES ($1, $2, $3, $4, 'associated', 0.9, $5)`, dupRelID, tenantID, mem1, mem2, agentID)
	if err == nil {
		t.Error("expected unique violation on duplicate (mem1, mem2, 'associated'), but got nil")
	}

	// 4. Verify ON CONFLICT (source_memory_id, target_memory_id, relation_type) DO NOTHING succeeds
	tag, err := pool.Exec(ctx, `INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id, relation_type, confidence, created_by)
		VALUES ($1, $2, $3, $4, 'associated', 0.9, $5)
		ON CONFLICT (source_memory_id, target_memory_id, relation_type) DO NOTHING`, dupRelID, tenantID, mem1, mem2, agentID)
	if err != nil {
		t.Errorf("ON CONFLICT DO NOTHING failed: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Errorf("expected 0 rows affected on conflict, got %d", tag.RowsAffected())
	}
}

func TestMigration0032RepointsBackwardsUpdatesEdges(t *testing.T) {
	url := os.Getenv("MEMRO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MEMRO_TEST_DATABASE_URL not set; skipping database migration test")
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
	repoint, err := migrations.FS.ReadFile("0032_repoint_backwards_updates_edges.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	tenantID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, name, created_at, updated_at) VALUES ($1, 'Repoint Test Tenant', NOW(), NOW())`, tenantID); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	agentID := "test-agent-" + uuid.NewString()[:8]
	if _, err := pool.Exec(ctx, `INSERT INTO agent_identities (id, tenant_id, created_at) VALUES ($1, $2, NOW())`, agentID, tenantID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	// Pairs of (older, newer). event_date wins over created_at, as in the linker.
	now := time.Now().UTC()
	mem := func(created time.Time, event *time.Time) uuid.UUID {
		id := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO memories (id, agent_id, tenant_id, content, memory_type, visibility, created_at, document_date, event_date, last_accessed_at, confidence)
			VALUES ($1, $2, $3, 'fact', 'semantic', 'private', $4, $4, $5, $4, 1.0)`, id, agentID, tenantID, created, event); err != nil {
			t.Fatalf("insert memory: %v", err)
		}
		return id
	}
	edge := func(from, to uuid.UUID) {
		if _, err := pool.Exec(ctx, `INSERT INTO memory_relations (id, tenant_id, source_memory_id, target_memory_id, relation_type, confidence, created_by)
			VALUES ($1, $2, $3, $4, 'updates', 0.75, $5)`, uuid.New(), tenantID, from, to, agentID); err != nil {
			t.Fatalf("insert edge: %v", err)
		}
	}
	countEdges := func(from, to uuid.UUID) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM memory_relations WHERE source_memory_id = $1 AND target_memory_id = $2 AND relation_type = 'updates'`, from, to).Scan(&n); err != nil {
			t.Fatalf("count edges: %v", err)
		}
		return n
	}

	oldA, newA := mem(now.Add(-48*time.Hour), nil), mem(now, nil)
	edge(oldA, newA) // backwards: flipped
	oldB, newB := mem(now.Add(-48*time.Hour), nil), mem(now, nil)
	edge(newB, oldB) // already correct: untouched
	oldC, newC := mem(now.Add(-48*time.Hour), nil), mem(now, nil)
	edge(oldC, newC) // backwards with a correct twin: removed
	edge(newC, oldC)
	early := now.Add(-30 * 24 * time.Hour)
	oldD, newD := mem(now, &early), mem(now.Add(-48*time.Hour), nil) // saved later, but happened earlier
	edge(oldD, newD)

	if _, err := pool.Exec(ctx, string(repoint)); err != nil {
		t.Fatalf("run migration 0032: %v", err)
	}

	for _, tc := range []struct {
		name         string
		newer, older uuid.UUID
	}{{"flipped", newA, oldA}, {"untouched", newB, oldB}, {"twin kept", newC, oldC}, {"event_date order", newD, oldD}} {
		if got := countEdges(tc.newer, tc.older); got != 1 {
			t.Errorf("%s: newer -> older edges = %d, want 1", tc.name, got)
		}
		if got := countEdges(tc.older, tc.newer); got != 0 {
			t.Errorf("%s: older -> newer edges = %d, want 0", tc.name, got)
		}
	}
	var marked string
	if err := pool.QueryRow(ctx, `SELECT metadata->>'repointed_by' FROM memory_relations WHERE source_memory_id = $1`, newA).Scan(&marked); err != nil || marked != "0032" {
		t.Errorf("flipped edge metadata repointed_by = %q (%v), want 0032", marked, err)
	}
}
