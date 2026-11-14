-- ADR 0027: verified company domains and requests to join an organisation.

-- Domains an organisation has claimed. A claim proves nothing until the DNS
-- TXT record is found (verified_at); token NULL means Memro staff verified it
-- by hand, so it is not re-checked. Any number of organisations may claim a
-- domain, but only one may hold it verified.
CREATE TABLE IF NOT EXISTS tenant_domains (
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain      TEXT NOT NULL,
    token       TEXT,
    verified_at TIMESTAMPTZ,
    checked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, domain)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_domains_verified ON tenant_domains (domain) WHERE verified_at IS NOT NULL;

-- Domains staff set when creating a tenant were checked by them.
INSERT INTO tenant_domains (tenant_id, domain, verified_at)
SELECT id, lower(domain), NOW() FROM tenants
WHERE domain IS NOT NULL AND id <> '00000000-0000-0000-0000-000000000000'
ON CONFLICT DO NOTHING;

-- invite: members join only by invitation. request: people at a verified
-- domain may ask to join, and an owner or admin decides.
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS join_policy TEXT NOT NULL DEFAULT 'invite'
    CHECK (join_policy IN ('invite', 'request'));

-- Who created a self-service organisation; NULL for ones staff created.
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS tenant_join_requests (
    id         UUID PRIMARY KEY,
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- One pending request per person and organisation; decided requests are deleted.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_join_requests_pending ON tenant_join_requests (tenant_id, user_id);
