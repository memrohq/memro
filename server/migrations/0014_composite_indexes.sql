-- P0 Optimization: Composite indexes for faster filtered queries
-- Target query patterns:
-- 1. Get memories by agent_id + memory_type (for type-specific recall)
-- 2. Get memories by agent_id + created_at (for temporal queries)
-- 3. Get memories by tenant_id + agent_id (for multi-tenant filtering)

-- Index for agent + memory_type lookups (used in search, get_by_type queries)
CREATE INDEX IF NOT EXISTS idx_memories_agent_type 
ON memories(agent_id, memory_type);

-- Index for agent + created_at lookups (used in temporal queries)
CREATE INDEX IF NOT EXISTS idx_memories_agent_created 
ON memories(agent_id, created_at DESC);

-- Index for tenant + agent lookups (multi-tenant filtering)
CREATE INDEX IF NOT EXISTS idx_memories_tenant_agent 
ON memories(tenant_id, agent_id);

-- Index for event_date lookups (temporal grounding feature)
CREATE INDEX IF NOT EXISTS idx_memories_event_date 
ON memories(agent_id, event_date) 
WHERE event_date IS NOT NULL;

-- Index for session_id lookups (multi-session synthesis)
CREATE INDEX IF NOT EXISTS idx_memories_session_id 
ON memories(agent_id, session_id) 
WHERE session_id IS NOT NULL;

-- Index for full-text search + agent filtering
CREATE INDEX IF NOT EXISTS idx_memories_fts_agent 
ON memories USING GIN(to_tsvector('english', content));

-- Index for memory relations lookups
CREATE INDEX IF NOT EXISTS idx_memory_relations_agent 
ON memory_relations(created_by, relation_type);

-- Composite index for knowledge chain traversal
CREATE INDEX IF NOT EXISTS idx_memory_chains 
ON memory_relations(created_by, source_memory_id, target_memory_id, relation_type);

-- Comment for tracking optimization
COMMENT ON INDEX idx_memories_agent_type IS 'P0 Optimization: Composite index for agent_id + memory_type queries';
COMMENT ON INDEX idx_memories_agent_created IS 'P0 Optimization: Composite index for temporal queries (agent_id + created_at)';
COMMENT ON INDEX idx_memories_tenant_agent IS 'P0 Optimization: Multi-tenant filtering index';
COMMENT ON INDEX idx_memories_event_date IS 'P0 Optimization: Temporal grounding support';
COMMENT ON INDEX idx_memories_session_id IS 'P0 Optimization: Multi-session synthesis support';
COMMENT ON INDEX idx_memory_relations_agent IS 'P0 Optimization: Knowledge graph traversal';
COMMENT ON INDEX idx_memory_chains IS 'P0 Optimization: Complex knowledge chain queries';
