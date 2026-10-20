-- A webhook registered with an API key goes when that key is revoked or
-- deleted (ADR 0012), so a leaked key cannot leave a hook behind that keeps
-- receiving the agent's new memories. NULL: registered with a dashboard
-- session or a signature, or before this migration.
ALTER TABLE hooks ADD COLUMN IF NOT EXISTS api_key_id UUID REFERENCES api_keys(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_hooks_api_key ON hooks (api_key_id) WHERE api_key_id IS NOT NULL;
