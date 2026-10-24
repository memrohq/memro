-- Migration 0031: Add associated and causes relation types, and enforce edge uniqueness.
-- ADR 0023: Graph edge semantics, save-time edges, and optional graph expansion in search

-- 1. Deduplicate any existing duplicate edges before adding unique index
DELETE FROM memory_relations a
USING memory_relations b
WHERE a.source_memory_id = b.source_memory_id
  AND a.target_memory_id = b.target_memory_id
  AND a.relation_type = b.relation_type
  AND a.ctid < b.ctid;

-- 2. Extend relation_type CHECK constraint to support associated and causes
ALTER TABLE memory_relations DROP CONSTRAINT IF EXISTS valid_relation_type;
ALTER TABLE memory_relations ADD CONSTRAINT valid_relation_type
    CHECK (relation_type IN ('updates', 'extends', 'derives', 'associated', 'causes'));

-- 3. Enforce edge uniqueness per (source, target, relation_type)
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_relations_unique_edge
    ON memory_relations (source_memory_id, target_memory_id, relation_type);
