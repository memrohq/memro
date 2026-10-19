-- Webhooks belong to the agent that registered them (ADR 0008). Hooks
-- registered before this change have no known owner and are removed.
ALTER TABLE hooks ADD COLUMN IF NOT EXISTS agent_id TEXT REFERENCES agent_identities(id) ON DELETE CASCADE;
DELETE FROM hooks WHERE agent_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_hooks_agent_event_url ON hooks (agent_id, event_type, url);
-- The tenant-wide uniqueness would silently drop a second agent's hook for
-- the same URL. The previous version cannot register hooks after this runs
-- (its ON CONFLICT needs that constraint); that only affects the seconds of
-- a blue-green switch.
ALTER TABLE hooks DROP CONSTRAINT IF EXISTS hooks_tenant_id_event_type_url_key;
