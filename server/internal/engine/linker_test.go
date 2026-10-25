package engine_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/memrohq/memro/server/internal/db"
	"github.com/memrohq/memro/server/internal/domain"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/migrations"
)

type dummyIndexer struct{}

func (d dummyIndexer) Enqueue(id uuid.UUID, agentID string, tenantID uuid.UUID, content string) {}

func TestSaveTimeLinker(t *testing.T) {
	url := os.Getenv("MEMRO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MEMRO_TEST_DATABASE_URL not set; skipping linker test")
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
	_, err = pool.Exec(ctx, `INSERT INTO tenants (id, name, created_at, updated_at) VALUES ($1, 'Linker Test Tenant', NOW(), NOW())`, tenantID)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)

	agentID := "test-agent-" + uuid.NewString()[:8]
	_, err = pool.Exec(ctx, `INSERT INTO agent_identities (id, tenant_id, created_at) VALUES ($1, $2, NOW())`, agentID, tenantID)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	// Helper to insert a memory into Postgres
	insertMem := func(m *domain.Memory) {
		t.Helper()
		_, err := pool.Exec(ctx, `INSERT INTO memories (id, agent_id, tenant_id, content, memory_type, visibility, created_at, document_date, event_date, entity_tags, confidence, last_accessed_at)
			VALUES ($1, $2, $3, $4, $5, 'private', $6, $6, $7, $8, 1.0, NOW())`,
			m.ID, m.AgentID, m.TenantID, m.Content, string(m.MemoryType), m.CreatedAt, m.EventDate, m.EntityTags)
		if err != nil {
			t.Fatalf("insert memory: %v", err)
		}
	}

	cfg := engine.LinkerConfig{
		Enabled:       true,
		Timeout:       500 * time.Millisecond,
		MaxTags:       5,
		MaxCandidates: 8,
	}

	// 1. Test association edge between memories sharing a specific entity tag
	t1 := time.Date(2023, 4, 1, 10, 0, 0, 0, time.UTC)
	mem1 := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    agentID,
		TenantID:   tenantID,
		Content:    "I love visiting the gardens in Harajuku on weekends.",
		MemoryType: domain.Episodic,
		CreatedAt:  t1,
		EventDate:  &t1,
		EntityTags: []string{"harajuku", "gardens"},
	}
	insertMem(mem1)

	t2 := time.Date(2023, 4, 15, 10, 0, 0, 0, time.UTC)
	mem2 := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    agentID,
		TenantID:   tenantID,
		Content:    "Had matcha tea near Harajuku station with friends.",
		MemoryType: domain.Episodic,
		CreatedAt:  t2,
		EventDate:  &t2,
		EntityTags: []string{"harajuku", "station"},
	}
	insertMem(mem2)

	// Link mem2 on save
	eng.LinkOnSave(ctx, mem2, cfg)

	// Verify associated edge exists between mem1 and mem2
	var relType string
	var conf float32
	var src, tgt uuid.UUID
	err = pool.QueryRow(ctx, `SELECT source_memory_id, target_memory_id, relation_type, confidence
		FROM memory_relations
		WHERE (source_memory_id = $1 AND target_memory_id = $2) OR (source_memory_id = $2 AND target_memory_id = $1)`,
		mem1.ID, mem2.ID).Scan(&src, &tgt, &relType, &conf)
	if err != nil {
		t.Fatalf("expected associated edge between mem1 and mem2, got err: %v", err)
	}
	if relType != "associated" {
		t.Errorf("expected relation_type 'associated', got %s", relType)
	}
	if src.String() > tgt.String() {
		t.Errorf("expected canonical ordering source < target, got src=%s tgt=%s", src, tgt)
	}

	// 2. Test contradiction produces 'updates' edge directed newer -> older
	t3 := time.Date(2023, 5, 1, 10, 0, 0, 0, time.UTC)
	mem3 := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    agentID,
		TenantID:   tenantID,
		Content:    "I have lived in my Harajuku apartment for 1 month.",
		MemoryType: domain.Episodic,
		CreatedAt:  t3,
		EventDate:  &t3,
		EntityTags: []string{"harajuku", "apartment"},
	}
	insertMem(mem3)

	t4 := time.Date(2023, 7, 1, 10, 0, 0, 0, time.UTC)
	mem4 := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    agentID,
		TenantID:   tenantID,
		Content:    "I have lived in my Harajuku apartment for 3 months.",
		MemoryType: domain.Episodic,
		CreatedAt:  t4,
		EventDate:  &t4,
		EntityTags: []string{"harajuku", "apartment"},
	}
	insertMem(mem4)

	// Link mem4 on save
	eng.LinkOnSave(ctx, mem4, cfg)

	// Verify 'updates' edge exists pointing mem4 (newer) -> mem3 (older)
	err = pool.QueryRow(ctx, `SELECT source_memory_id, target_memory_id, relation_type
		FROM memory_relations
		WHERE source_memory_id = $1 AND target_memory_id = $2`,
		mem4.ID, mem3.ID).Scan(&src, &tgt, &relType)
	if err != nil {
		t.Fatalf("expected updates edge mem4 -> mem3, got err: %v", err)
	}
	if relType != "updates" {
		t.Errorf("expected relation_type 'updates', got %s", relType)
	}

	// 3. Verify ubiquitous tags like 'user' or 'assistant' alone do NOT create edges
	t5 := time.Date(2023, 8, 1, 10, 0, 0, 0, time.UTC)
	mem5 := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    agentID,
		TenantID:   tenantID,
		Content:    "User: hello there.",
		MemoryType: domain.Episodic,
		CreatedAt:  t5,
		EventDate:  &t5,
		EntityTags: []string{"user", "assistant"},
	}
	insertMem(mem5)

	t6 := time.Date(2023, 8, 2, 10, 0, 0, 0, time.UTC)
	mem6 := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    agentID,
		TenantID:   tenantID,
		Content:    "User: how are you?",
		MemoryType: domain.Episodic,
		CreatedAt:  t6,
		EventDate:  &t6,
		EntityTags: []string{"user", "assistant"},
	}
	insertMem(mem6)

	eng.LinkOnSave(ctx, mem6, cfg)

	var count int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM memory_relations
		WHERE (source_memory_id = $1 AND target_memory_id = $2) OR (source_memory_id = $2 AND target_memory_id = $1)`,
		mem5.ID, mem6.ID).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 edges between memories sharing only ubiquitous tags, got %d", count)
	}

	// 4. Isolation test: memories belonging to another agent are never linked
	otherAgentID := "other-agent-" + uuid.NewString()[:8]
	_, err = pool.Exec(ctx, `INSERT INTO agent_identities (id, tenant_id, created_at) VALUES ($1, $2, NOW())`, otherAgentID, tenantID)
	if err != nil {
		t.Fatalf("insert other agent: %v", err)
	}

	memOther := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    otherAgentID,
		TenantID:   tenantID,
		Content:    "Shopping in Harajuku for shoes.",
		MemoryType: domain.Episodic,
		CreatedAt:  t4,
		EventDate:  &t4,
		EntityTags: []string{"harajuku"},
	}
	insertMem(memOther)

	memLocal := &domain.Memory{
		ID:         uuid.New(),
		AgentID:    agentID,
		TenantID:   tenantID,
		Content:    "Shopping in Harajuku for bags.",
		MemoryType: domain.Episodic,
		CreatedAt:  t4.Add(time.Hour),
		EventDate:  &t4,
		EntityTags: []string{"harajuku"},
	}
	insertMem(memLocal)

	eng.LinkOnSave(ctx, memLocal, cfg)

	_ = pool.QueryRow(ctx, `SELECT count(*) FROM memory_relations
		WHERE (source_memory_id = $1 AND target_memory_id = $2) OR (source_memory_id = $2 AND target_memory_id = $1)`,
		memLocal.ID, memOther.ID).Scan(&count)
	if count != 0 {
		t.Errorf("expected zero cross-agent edges, but found %d", count)
	}
}
