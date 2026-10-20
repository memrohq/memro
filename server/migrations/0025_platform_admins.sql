-- Platform admins (Memro staff), ADR 0012. Admin power comes only from this
-- table and a dashboard session, never from an API key, a signature or an
-- email domain. Grant with SQL (see the ops runbook) until the admin
-- dashboard manages it.
CREATE TABLE IF NOT EXISTS platform_admins (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note       TEXT
);
