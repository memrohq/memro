-- ADR 0013: an agent's identity is separate from the keys allowed to sign for
-- it. The agent ID stays the public key it was registered with; agent_keys
-- lists the keys that may act for it. Revoked keys stay (revoked_at set) so a
-- key can never be reused, and a public key belongs to at most one agent.
CREATE TABLE IF NOT EXISTS agent_keys (
    public_key TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL REFERENCES agent_identities(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('signing', 'recovery')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_agent_keys_active ON agent_keys (agent_id) WHERE revoked_at IS NULL;

-- Every existing agent signs with its own ID, as before.
INSERT INTO agent_keys (public_key, agent_id, role)
SELECT id, id, 'signing' FROM agent_identities
ON CONFLICT (public_key) DO NOTHING;

-- And so does every new one, however it is created.
CREATE OR REPLACE FUNCTION agent_identities_original_key() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO agent_keys (public_key, agent_id, role) VALUES (NEW.id, NEW.id, 'signing')
    ON CONFLICT (public_key) DO NOTHING;
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS agent_identities_original_key ON agent_identities;
CREATE TRIGGER agent_identities_original_key AFTER INSERT ON agent_identities
    FOR EACH ROW EXECUTE FUNCTION agent_identities_original_key();
