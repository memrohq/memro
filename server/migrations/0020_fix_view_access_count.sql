-- Migration: Fix memory_current_version view to include access_count
-- Resolves ColumnNotFound("access_count") in Rust backend

CREATE OR REPLACE VIEW memory_current_version AS
WITH RECURSIVE version_chain AS (
    -- Base: memories with no incoming "updates" relations (these are current)
    SELECT 
        m.id,
        m.agent_id,
        m.tenant_id,
        m.id as root_id,
        0 as depth,
        m.created_at,
        m.access_count,
        m.confidence,       -- Added column
        m.last_accessed_at  -- Added column
    FROM memories m
    WHERE NOT EXISTS (
        SELECT 1 FROM memory_relations mr
        WHERE mr.target_memory_id = m.id 
        AND mr.relation_type = 'updates'
    )
    
    UNION ALL
    
    -- Recursive: follow the chain of updates backwards
    SELECT 
        vm.id,
        vm.agent_id,
        vm.tenant_id,
        vc.root_id,
        vc.depth + 1,
        vc.created_at,
        vm.access_count,
        vm.confidence,       -- Added column
        vm.last_accessed_at  -- Added column
    FROM version_chain vc
    JOIN memory_relations mr ON 
        mr.source_memory_id = vc.id 
        AND mr.relation_type = 'updates'
    JOIN memories vm ON vm.id = mr.target_memory_id
    WHERE vc.depth < 100 -- Prevent infinite loops
)
SELECT DISTINCT ON (root_id)
    root_id as current_memory_id,
    id as previous_memory_id,
    agent_id,
    tenant_id,
    depth as update_depth,
    created_at,
    access_count,
    confidence,        -- Added column
    last_accessed_at   -- Added column
FROM version_chain
ORDER BY root_id, depth DESC;
