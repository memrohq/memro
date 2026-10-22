-- Server-side multi-agent coordination leases and intents
-- Replaces memory-based claims with atomic, expiring, scoped leases.

CREATE TABLE IF NOT EXISTS coordination_leases (
    id UUID PRIMARY KEY,
    scope_key TEXT NOT NULL,
    resource TEXT NOT NULL,
    holder_agent_id TEXT NOT NULL,
    holder_session TEXT NOT NULL,
    intent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    released_at TIMESTAMPTZ,
    release_status TEXT CHECK (release_status IS NULL OR release_status IN ('completed', 'abandoned', 'handoff', 'expired')),
    handoff_summary TEXT,
    handoff_to TEXT
);

-- Exactly one active lease per resource within a scope
CREATE UNIQUE INDEX IF NOT EXISTS idx_coordination_leases_active 
ON coordination_leases (scope_key, resource) 
WHERE released_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_coordination_leases_released
ON coordination_leases (released_at) WHERE released_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_coordination_leases_scope 
ON coordination_leases (scope_key, expires_at);

CREATE TABLE IF NOT EXISTS coordination_intents (
    id UUID PRIMARY KEY,
    scope_key TEXT NOT NULL,
    resource TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    session TEXT NOT NULL DEFAULT 'default',
    intent TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_coordination_intents_scope 
ON coordination_intents (scope_key, resource, expires_at);
