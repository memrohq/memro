-- API keys (memro_sk_...) that authenticate as an agent, and the device-code
-- flow `memro connect` uses to obtain one through the dashboard.

CREATE TABLE IF NOT EXISTS api_keys (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL,
    agent_id     TEXT NOT NULL REFERENCES agent_identities(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL,           -- first characters, shown in the dashboard
    key_hash     TEXT NOT NULL UNIQUE,    -- SHA-256 of the full key; the key itself is never stored
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_api_keys_agent ON api_keys (agent_id) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS device_codes (
    device_code_hash TEXT PRIMARY KEY,    -- SHA-256 of the secret the CLI polls with
    user_code        TEXT NOT NULL UNIQUE,-- short code the user confirms in the dashboard
    client_name      TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at       TIMESTAMPTZ NOT NULL,
    approved_at      TIMESTAMPTZ,
    agent_id         TEXT REFERENCES agent_identities(id) ON DELETE CASCADE,
    tenant_id        UUID,
    consumed_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_device_codes_expires ON device_codes (expires_at);
