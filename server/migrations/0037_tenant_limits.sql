-- ADR 0028: limits per organisation, in the same units as Memro Cloud's
-- public tiers: stored tokens (bytes of memory text / 4) and queries.
-- Self-service organisations start on a trial; Memro staff set Enterprise or
-- custom contracts. A tenant without a row keeps the server's quota policy,
-- which on Memro Cloud is the tier the organisation bought (Free, Builder,
-- Pro, Scale). NULL limits mean none. A contract's included usage and
-- overage are billing's business (memro-cloud).
CREATE TABLE IF NOT EXISTS tenant_limits (
    tenant_id                  UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    plan                       TEXT NOT NULL CHECK (plan IN ('trial', 'enterprise', 'custom')),
    max_stored_tokens_per_user BIGINT CHECK (max_stored_tokens_per_user > 0),
    max_stored_tokens_total    BIGINT CHECK (max_stored_tokens_total > 0), -- shared by all users
    max_queries_monthly        BIGINT CHECK (max_queries_monthly > 0),     -- shared, per calendar month (UTC)
    max_members                INT CHECK (max_members > 0),                -- dashboard seats incl. pending invites
    allow_images               BOOLEAN NOT NULL DEFAULT TRUE,              -- image analysis costs model tokens
    trial_ends_at              TIMESTAMPTZ,  -- after it: read-only; data deleted 60 days later
    trial_notice               TEXT,         -- last trial email sent: ended, d14, d7, d1, deleted
    warned_at                  TIMESTAMPTZ,  -- owners emailed at 80% of a limit
    updated_by                 UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tenant_limits_trial ON tenant_limits (trial_ends_at) WHERE plan = 'trial';

-- One free trial per company: the email domain of whoever started it.
CREATE TABLE IF NOT EXISTS trial_domains (
    domain     TEXT PRIMARY KEY,
    tenant_id  UUID REFERENCES tenants(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
