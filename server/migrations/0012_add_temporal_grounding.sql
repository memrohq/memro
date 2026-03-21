-- Migration: Add Temporal Grounding for Supermemory-style memory architecture
-- Adds document_date and event_date to track when conversations happened vs when events occurred
-- This enables accurate temporal reasoning and multi-session synthesis

-- Add UNIQUE(id, tenant_id) to agent_identities for multi-tenant FK support
-- Necessary for batch embedding foreign keys in 0015
ALTER TABLE agent_identities ADD CONSTRAINT agent_identities_id_tenant_unique UNIQUE (id, tenant_id);

-- Add temporal columns to memories table
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='memories' AND column_name='document_date') THEN
        ALTER TABLE memories ADD COLUMN document_date TIMESTAMPTZ DEFAULT NOW();
        COMMENT ON COLUMN memories.document_date IS 'When the conversation/document occurred (e.g., when user spoke about an event)';
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='memories' AND column_name='event_date') THEN
        ALTER TABLE memories ADD COLUMN event_date TIMESTAMPTZ;
        COMMENT ON COLUMN memories.event_date IS 'When the event described actually occurred (extracted from content)';
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='memories' AND column_name='session_id') THEN
        ALTER TABLE memories ADD COLUMN session_id UUID;
        COMMENT ON COLUMN memories.session_id IS 'Session this memory belongs to (for multi-session reasoning)';
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='memories' AND column_name='is_atomic') THEN
        ALTER TABLE memories ADD COLUMN is_atomic BOOLEAN DEFAULT FALSE;
        COMMENT ON COLUMN memories.is_atomic IS 'Whether this is an atomic fact (derived from chunk) or raw chunk';
    END IF;
END $$;

-- Create indexes for temporal queries
CREATE INDEX IF NOT EXISTS idx_memories_document_date ON memories(tenant_id, agent_id, document_date DESC);
CREATE INDEX IF NOT EXISTS idx_memories_event_date ON memories(tenant_id, agent_id, event_date DESC);
CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_memories_atomic ON memories(tenant_id, agent_id, is_atomic);

-- Create conversation_sessions table for multi-session reasoning
CREATE TABLE IF NOT EXISTS conversation_sessions (
    id UUID PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agent_identities(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_start TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_end TIMESTAMPTZ,
    session_metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversation_sessions_agent ON conversation_sessions(agent_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_conversation_sessions_start ON conversation_sessions(session_start DESC);
