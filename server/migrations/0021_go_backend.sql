-- Go backend: pgvector embeddings, fixes for columns the API writes, and
-- hardened auth storage.

CREATE EXTENSION IF NOT EXISTS vector;

-- The API has always written memories.metadata, but no migration created it.
ALTER TABLE memories ADD COLUMN IF NOT EXISTS metadata JSONB;

-- Embeddings live next to the memories they describe (replaces Qdrant).
-- The column is dimension-agnostic so the embedding model can change; rows
-- whose dims differ from the configured model are re-embedded on startup.
CREATE TABLE IF NOT EXISTS memory_embeddings (
    memory_id  UUID PRIMARY KEY REFERENCES memories(id) ON DELETE CASCADE,
    tenant_id  UUID NOT NULL,
    agent_id   TEXT NOT NULL,
    model      TEXT NOT NULL,
    dims       INTEGER NOT NULL,
    embedding  vector NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_memory_embeddings_agent ON memory_embeddings (tenant_id, agent_id, dims);

-- Session tokens are now stored as SHA-256 hashes. Existing rows held
-- plaintext tokens, so they are dropped and users sign in again.
DELETE FROM sessions;
CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions (token_hash);

-- OTP codes are stored hashed, with an attempt counter to stop brute force.
UPDATE users SET otp_code = NULL, otp_expires_at = NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS otp_attempts INTEGER NOT NULL DEFAULT 0;

-- Uploads from /upload/image never set tenant_id; give it a default so the
-- NOT NULL constraint from 0011 is satisfied.
ALTER TABLE files ALTER COLUMN tenant_id SET DEFAULT '00000000-0000-0000-0000-000000000000';

CREATE INDEX IF NOT EXISTS idx_audit_log_resource ON audit_log (resource_id);
