-- ADR 0026: organisation accounts and usage per end user.

-- Usage per agent (for a tenant key, per end user) and month, kept as running
-- totals so it stays small. Not tied to agent_identities: a deleted end user's
-- usage still counts for the month it happened.
CREATE TABLE IF NOT EXISTS usage_monthly (
    tenant_id UUID NOT NULL,
    agent_id  TEXT NOT NULL,
    month     DATE NOT NULL,
    metric    TEXT NOT NULL,
    value     BIGINT NOT NULL,
    PRIMARY KEY (tenant_id, month, agent_id, metric)
);

-- People who manage a dedicated tenant (an organisation). A person signs in
-- once (users row, usually in the default tenant) and may belong to several.
-- Roles: owner (everything), admin (members, keys, usage), billing (usage and
-- billing), viewer (read-only usage). None of them can read end users'
-- memories: content is reachable only through the tenant's key.
CREATE TABLE IF NOT EXISTS tenant_members (
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'billing', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_tenant_members_user ON tenant_members (user_id);

-- Invitations by email; the link's secret is stored only as a hash.
CREATE TABLE IF NOT EXISTS tenant_invites (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    role        TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'billing', 'viewer')),
    token_hash  TEXT NOT NULL UNIQUE,
    invited_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_tenant_invites_tenant ON tenant_invites (tenant_id) WHERE accepted_at IS NULL;
