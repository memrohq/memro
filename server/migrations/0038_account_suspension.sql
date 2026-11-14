-- ADR 0029: Memro staff can suspend a person or an organisation. While
-- suspended, every request it makes is refused (dashboard sessions, API keys,
-- server keys, signed requests); its data is kept.
ALTER TABLE users   ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ;
ALTER TABLE users   ADD COLUMN IF NOT EXISTS suspended_reason TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMPTZ;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS suspended_reason TEXT;
