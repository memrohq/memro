-- ADR 0025: a customer (tenant) gets one server key that acts for any of its
-- end users. An api_keys row with no agent_id is such a tenant-wide key; each
-- end user is an agent identity in that tenant, named by the customer's own
-- user ID (external_id) and created on first use.
ALTER TABLE api_keys ALTER COLUMN agent_id DROP NOT NULL;
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_wide ON api_keys (tenant_id) WHERE agent_id IS NULL AND revoked_at IS NULL;

ALTER TABLE agent_identities ADD COLUMN IF NOT EXISTS external_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_identities_external
    ON agent_identities (tenant_id, external_id) WHERE external_id IS NOT NULL;
