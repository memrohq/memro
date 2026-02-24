CREATE TABLE IF NOT EXISTS memories (
    id UUID PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agent_identities(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    memory_type TEXT NOT NULL, -- episodic, semantic, profile
    visibility TEXT NOT NULL, -- private, shared, public
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
