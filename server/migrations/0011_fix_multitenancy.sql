-- Migration: Fix Multi-Tenancy (Sessions, Users, and Files)
-- Adds missing tenant_id to sessions, users, and files tables

-- Add tenant_id to sessions (if missing)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='sessions' AND column_name='tenant_id') THEN
        ALTER TABLE sessions ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_sessions_tenant ON sessions(tenant_id);
    END IF;
END $$;

-- Add tenant_id to users
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='tenant_id') THEN
        ALTER TABLE users ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id);
    END IF;
END $$;

-- Add tenant_id to files
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='files' AND column_name='tenant_id') THEN
        ALTER TABLE files ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_files_tenant ON files(tenant_id);
    END IF;
END $$;

-- Populate existing rows with default tenant
UPDATE sessions SET tenant_id = '00000000-0000-0000-0000-000000000000' WHERE tenant_id IS NULL;
UPDATE users SET tenant_id = '00000000-0000-0000-0000-000000000000' WHERE tenant_id IS NULL;
UPDATE files SET tenant_id = '00000000-0000-0000-0000-000000000000' WHERE tenant_id IS NULL;

-- Make tenant_id NOT NULL
ALTER TABLE sessions ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE users ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE files ALTER COLUMN tenant_id SET NOT NULL;

-- Fix unique constraint on users (allow same email in different tenants)
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_email_tenant_key') THEN
        ALTER TABLE users ADD CONSTRAINT users_email_tenant_key UNIQUE (email, tenant_id);
    END IF;
END $$;
