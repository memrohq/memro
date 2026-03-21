-- Migration: Add Relational Versioning for Knowledge Chains
-- Implements Supermemory's knowledge relationships: updates, extends, derives
-- Enables tracking of how facts evolve and conflict resolution

CREATE TABLE IF NOT EXISTS memory_relations (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_memory_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    target_memory_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    relation_type TEXT NOT NULL, -- 'updates' (state mutation), 'extends' (refinement), 'derives' (inference)
    confidence FLOAT DEFAULT 1.0, -- How confident is this relationship (0.0-1.0)
    metadata JSONB DEFAULT '{}', -- Additional context about the relationship
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT NOT NULL, -- agent_id that created this relation
    CONSTRAINT valid_relation_type CHECK (relation_type IN ('updates', 'extends', 'derives'))
);

CREATE INDEX IF NOT EXISTS idx_memory_relations_source ON memory_relations(source_memory_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_memory_relations_target ON memory_relations(target_memory_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_memory_relations_type ON memory_relations(relation_type, tenant_id);
CREATE INDEX IF NOT EXISTS idx_memory_relations_created ON memory_relations(created_at DESC, tenant_id);

-- Create a view for getting the "current version" of facts (following update chains)
CREATE OR REPLACE VIEW memory_current_version AS
WITH RECURSIVE version_chain AS (
    -- Base: memories with no incoming "updates" relations (these are current)
    SELECT 
        m.id,
        m.agent_id,
        m.tenant_id,
        m.id as root_id,
        0 as depth,
        m.created_at
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
        vc.created_at
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
    created_at
FROM version_chain
ORDER BY root_id, depth DESC;

-- Table for tracking memory chains (for efficient multi-session queries)
CREATE TABLE IF NOT EXISTS memory_chains (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    root_memory_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    chain_metadata JSONB DEFAULT '{}', -- e.g., { "topic": "user_hobby", "entities": [...] }
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_memory_chains_root ON memory_chains(root_memory_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_memory_chains_tenant ON memory_chains(tenant_id);
