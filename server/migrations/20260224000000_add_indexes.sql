-- Performance indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_memories_agent_id ON memories(agent_id);
CREATE INDEX IF NOT EXISTS idx_memories_agent_created ON memories(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memories_type ON memories(agent_id, memory_type);
CREATE INDEX IF NOT EXISTS idx_memories_visibility ON memories(agent_id, visibility);
