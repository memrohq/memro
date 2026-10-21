-- Expand connector_connections to support all Top 50 connectors (DCE declarative catalog & native).
ALTER TABLE connector_connections DROP CONSTRAINT IF EXISTS connector_connections_provider_check;
