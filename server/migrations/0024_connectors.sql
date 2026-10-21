-- Connectors (ADR 0011): Slack workspaces and GitHub App installations whose
-- content is synced into the connecting agent's memory.

CREATE TABLE IF NOT EXISTS connector_connections (
    id                  UUID PRIMARY KEY,
    tenant_id           UUID NOT NULL,
    agent_id            TEXT NOT NULL REFERENCES agent_identities(id) ON DELETE CASCADE,
    provider            TEXT NOT NULL CHECK (provider IN ('slack', 'github', 'linear', 'sentry', 'notion')),
    account_id          TEXT NOT NULL,            -- Slack team ID / GitHub installation ID
    account_name        TEXT NOT NULL DEFAULT '',
    external_user_id    TEXT NOT NULL DEFAULT '', -- Slack user ID / GitHub user ID that connected
    external_user_name  TEXT NOT NULL DEFAULT '',
    credentials         BYTEA NOT NULL,           -- AES-256-GCM sealed JSON; never returned by the API
    status              TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'error', 'revoked')),
    status_detail       TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_synced_at      TIMESTAMPTZ,
    UNIQUE (agent_id, provider, account_id)
);
CREATE INDEX IF NOT EXISTS idx_connector_connections_account ON connector_connections (provider, account_id);

-- Channels / repositories of a connection. Slack channels are opt-in;
-- GitHub repositories are the ones the user granted the installation.
CREATE TABLE IF NOT EXISTS connector_sources (
    connection_id   UUID NOT NULL REFERENCES connector_connections(id) ON DELETE CASCADE,
    external_id     TEXT NOT NULL,
    name            TEXT NOT NULL,
    is_private      BOOLEAN NOT NULL DEFAULT false,
    enabled         BOOLEAN NOT NULL DEFAULT false,
    cursor          TEXT NOT NULL DEFAULT '',     -- provider position for incremental sync
    last_synced_at  TIMESTAMPTZ,
    PRIMARY KEY (connection_id, external_id)
);

-- One external item (a Slack channel-day, a GitHub issue or pull request)
-- and the memories made from it.
CREATE TABLE IF NOT EXISTS connector_items (
    connection_id  UUID NOT NULL REFERENCES connector_connections(id) ON DELETE CASCADE,
    item_key       TEXT NOT NULL,
    source_id      TEXT NOT NULL,
    document       JSONB,                         -- Slack messages, needed to rebuild a day from events
    content_hash   TEXT NOT NULL DEFAULT '',
    -- Hashes of every chunk stored as a memory. A hash listed here with no
    -- matching memory was deleted by the user and is not stored again.
    chunk_hashes   TEXT[] NOT NULL DEFAULT '{}',
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (connection_id, item_key)
);

ALTER TABLE memories ADD COLUMN IF NOT EXISTS connection_id UUID REFERENCES connector_connections(id) ON DELETE SET NULL;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS source_item TEXT;
CREATE INDEX IF NOT EXISTS idx_memories_connector_item ON memories (connection_id, source_item) WHERE connection_id IS NOT NULL;

-- Background work. Workers claim rows with FOR UPDATE SKIP LOCKED, so any
-- number of API instances (e.g. both colors during a deploy) can share it.
CREATE TABLE IF NOT EXISTS connector_jobs (
    id             BIGSERIAL PRIMARY KEY,
    connection_id  UUID NOT NULL REFERENCES connector_connections(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL,
    key            TEXT NOT NULL DEFAULT '',
    payload        JSONB,
    run_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_until   TIMESTAMPTZ,
    attempts       INTEGER NOT NULL DEFAULT 0,
    last_error     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_connector_jobs_due ON connector_jobs (run_at);
-- Coalesce identical work that has not started yet (e.g. a burst of
-- webhooks for one issue). Event jobs have an empty key and never coalesce.
CREATE UNIQUE INDEX IF NOT EXISTS idx_connector_jobs_pending ON connector_jobs (connection_id, kind, key)
    WHERE locked_until IS NULL AND key <> '';

-- OAuth state for an authorization in progress: single use, short-lived,
-- bound to the agent that started it and to the browser (cookie).
CREATE TABLE IF NOT EXISTS connector_oauth_states (
    state_hash  TEXT PRIMARY KEY,
    provider    TEXT NOT NULL,
    agent_id    TEXT NOT NULL REFERENCES agent_identities(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL
);
