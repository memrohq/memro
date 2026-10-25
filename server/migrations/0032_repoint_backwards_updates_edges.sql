-- Migration 0032: point every 'updates' edge from the newer memory to the older one.
-- ADR 0021: search marks the target of an 'updates' edge as superseded. Evolve()
-- used to write these edges in query order, so some point from the older memory
-- to the newer one and would mark the current fact as replaced. "Newer" uses the
-- same order as the save-time linker: event_date, then document_date, then created_at.

-- 1. A backwards edge whose correctly directed twin already exists is a duplicate.
DELETE FROM memory_relations r
USING memories s, memories t
WHERE r.relation_type = 'updates'
  AND s.id = r.source_memory_id
  AND t.id = r.target_memory_id
  AND COALESCE(s.event_date, s.document_date, s.created_at) < COALESCE(t.event_date, t.document_date, t.created_at)
  AND EXISTS (SELECT 1 FROM memory_relations x
              WHERE x.relation_type = 'updates'
                AND x.source_memory_id = r.target_memory_id
                AND x.target_memory_id = r.source_memory_id);

-- 2. Flip the rest. Right-hand sides read the old row, so this swaps the ends.
UPDATE memory_relations r
SET source_memory_id = r.target_memory_id,
    target_memory_id = r.source_memory_id,
    metadata = COALESCE(r.metadata, '{}'::jsonb) || '{"repointed_by": "0032"}'::jsonb
FROM memories s, memories t
WHERE r.relation_type = 'updates'
  AND s.id = r.source_memory_id
  AND t.id = r.target_memory_id
  AND COALESCE(s.event_date, s.document_date, s.created_at) < COALESCE(t.event_date, t.document_date, t.created_at);
