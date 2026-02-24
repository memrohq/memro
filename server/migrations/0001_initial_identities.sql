CREATE TABLE IF NOT EXISTS agent_identities (
    id TEXT PRIMARY KEY, -- Public Key
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

