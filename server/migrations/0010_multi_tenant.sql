-- Multi-tenant architecture migration
-- Adds tenant isolation and organization support

-- Create tenants table
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255) UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create organizations table (for enterprise)
CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add tenant_id to existing tables
ALTER TABLE agent_identities ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;

-- Create indexes for tenant isolation
CREATE INDEX IF NOT EXISTS idx_agent_identities_tenant ON agent_identities(tenant_id);
CREATE INDEX IF NOT EXISTS idx_memories_tenant ON memories(tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_tenant ON audit_log(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sessions_tenant ON sessions(tenant_id);

-- Create default tenant for existing data
INSERT INTO tenants (id, name, domain) 
VALUES ('00000000-0000-0000-0000-000000000000', 'Default Tenant', 'default.memro.co')
ON CONFLICT (id) DO NOTHING;

-- Migrate existing data to default tenant
UPDATE agent_identities SET tenant_id = '00000000-0000-0000-0000-000000000000' WHERE tenant_id IS NULL;
UPDATE memories SET tenant_id = '00000000-0000-0000-0000-000000000000' WHERE tenant_id IS NULL;
UPDATE audit_log SET tenant_id = '00000000-0000-0000-0000-000000000000' WHERE tenant_id IS NULL;
UPDATE sessions SET tenant_id = '00000000-0000-0000-0000-000000000000' WHERE tenant_id IS NULL;

-- Make tenant_id NOT NULL after migration
DO $$
BEGIN
    ALTER TABLE agent_identities ALTER COLUMN tenant_id SET NOT NULL;
EXCEPTION WHEN others THEN
    RAISE NOTICE 'Could not set tenant_id to NOT NULL on agent_identities';
END $$;

DO $$
BEGIN
    ALTER TABLE memories ALTER COLUMN tenant_id SET NOT NULL;
EXCEPTION WHEN others THEN
    RAISE NOTICE 'Could not set tenant_id to NOT NULL on memories';
END $$;

DO $$
BEGIN
    ALTER TABLE audit_log ALTER COLUMN tenant_id SET NOT NULL;
EXCEPTION WHEN others THEN
    RAISE NOTICE 'Could not set tenant_id to NOT NULL on audit_log';
END $$;

DO $$
BEGIN
    ALTER TABLE sessions ALTER COLUMN tenant_id SET NOT NULL;
EXCEPTION WHEN others THEN
    RAISE NOTICE 'Could not set tenant_id to NOT NULL on sessions';
END $$;

-- Create tenant-specific rate limiting table
CREATE TABLE IF NOT EXISTS tenant_rate_limits (
    tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    requests_per_minute INTEGER NOT NULL DEFAULT 1000,
    burst_limit INTEGER NOT NULL DEFAULT 100,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert default rate limits for default tenant
INSERT INTO tenant_rate_limits (tenant_id, requests_per_minute, burst_limit)
VALUES ('00000000-0000-0000-0000-000000000000', 1000, 100)
ON CONFLICT (tenant_id) DO NOTHING;

-- Create tenant usage tracking
CREATE TABLE IF NOT EXISTS tenant_usage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    metric_name VARCHAR(255) NOT NULL,
    metric_value BIGINT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tenant_usage_tenant_metric ON tenant_usage(tenant_id, metric_name, recorded_at);
CREATE INDEX IF NOT EXISTS idx_tenant_usage_recorded_at ON tenant_usage(recorded_at);