-- Webhook replay protection (ADR 0017): delivery-ID deduplication across instances.
CREATE TABLE IF NOT EXISTS connector_webhook_deliveries (
    provider    TEXT NOT NULL,
    delivery_id TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider, delivery_id)
);

CREATE INDEX IF NOT EXISTS idx_connector_webhook_deliveries_received_at
    ON connector_webhook_deliveries (received_at);
