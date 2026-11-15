-- ADR 0032, step 1: where each memory came from, a richer audit log, and an
-- archive so deleting a memory can be undone.

-- Where a memory came from: "mcp", "sdk-python/0.2.1", "dashboard",
-- "import:pdf", "connector:slack", "evolve", "api" … and who wrote it
-- ("agent", "user", "system"). NULL for memories written before this.
ALTER TABLE memories ADD COLUMN IF NOT EXISTS source TEXT;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS author_type TEXT;

-- Who acted (agent, user, system, llm), through what, what changed and why.
-- before/after hold the memory's content and type; a purge empties them.
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS actor_type TEXT;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS source TEXT;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS before JSONB;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS after JSONB;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS reason TEXT;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS request_id TEXT;
CREATE INDEX IF NOT EXISTS idx_audit_log_resource ON audit_log (resource_id, created_at);

-- Deleted memories, restorable until purged. Kept out of the memories table
-- so no search or listing can return them by mistake.
CREATE TABLE IF NOT EXISTS memory_archive (
    id          UUID        PRIMARY KEY,
    tenant_id   UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id    TEXT        NOT NULL REFERENCES agent_identities(id) ON DELETE CASCADE,
    memory      JSONB       NOT NULL,   -- the memories row as it was
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_by TEXT,
    reason      TEXT
);
CREATE INDEX IF NOT EXISTS idx_memory_archive_agent ON memory_archive (tenant_id, agent_id, archived_at DESC);
