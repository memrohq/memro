-- Audit log: immutable record of every mutating API action.
-- Written by the API layer; never updated or deleted.

CREATE TABLE IF NOT EXISTS audit_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at  TIMESTAMPTZ NOT NULL    DEFAULT NOW(),
    agent_id    TEXT        NOT NULL,
    action      TEXT        NOT NULL,   -- e.g. 'memory.create', 'memory.delete', 'identity.delete'
    resource_id TEXT,                   -- UUID of the affected resource (NULL for identity ops)
    ip_addr     TEXT                    -- caller IP, best-effort
);

CREATE INDEX IF NOT EXISTS idx_audit_log_agent_id  ON audit_log (agent_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log (created_at DESC);
