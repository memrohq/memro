-- Migration: Multi-Tenant SaaS Auth Schema
-- Adds humans (users), ties agents to users, and manages web sessions

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    email VARCHAR UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Associate existing and future agent_identities with a human user (optional for backwards compatibility)
ALTER TABLE agent_identities 
ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;

-- Index for finding agents owned by a user
CREATE INDEX IF NOT EXISTS idx_agent_identities_user_id ON agent_identities(user_id);
