# 0017. Webhook replay protection for GitHub, Linear and Sentry
Date: 2026-10-22 · Status: Accepted

## Context
Connectors receive webhooks from third-party SaaS providers to keep memories synchronized in near real time. Previously, only Slack checked timestamp skew (`X-Slack-Request-Timestamp`). GitHub, Linear, and Sentry webhooks were vulnerable to replay attacks if an attacker intercepted or re-sent valid signed payloads. Additionally, provider retries or network duplicates could result in redundant sync queueing and processing.

## Decision
Implement a two-tier replay defense across GitHub, Linear, and Sentry:

1. **Delivery ID Deduplication in Postgres**:
   - Create table `connector_webhook_deliveries (provider, delivery_id, received_at) PRIMARY KEY (provider, delivery_id)`.
   - Before processing a webhook delivery, attempt an atomic insert (`ON CONFLICT (provider, delivery_id) DO NOTHING`).
   - If not claimed, acknowledge the HTTP delivery with `200 OK` (to prevent provider retry loops) and increment a Prometheus metric with label `duplicate`, but skip queueing or work.
   - Provider delivery ID sources:
     - GitHub: `X-GitHub-Delivery` header.
     - Linear: `webhookId` in JSON payload.
     - Sentry: `Request-ID` or `Request-Id` header.
   - Run hourly TTL cleanup jobs purging records older than 24 hours.
   - Provide an in-memory fallback for environments without an active database connection.

2. **Timestamp Window Verification**:
   - Providers transmitting delivery timestamps are validated against a ±5 minute skew window:
     - Linear: `webhookTimestamp` (milliseconds epoch).
     - Sentry: `Sentry-Hook-Timestamp` (seconds epoch).
   - Deliveries outside the allowed skew window are rejected immediately with `401 Unauthorized` and increment label `stale_timestamp`.

## Alternatives considered
- **In-memory cache only (Redis / Go map)**: Fails across multi-replica deployments or restarts, allowing replays against separate pods. Postgres provides atomic uniqueness with minimal overhead.
- **Reject duplicate deliveries with 409 or 400**: Causes webhook providers like GitHub and Linear to mark deliveries as failed and trigger exponential retry storms. Returning 200 OK cleanly absorbs duplicate deliveries.

## Consequences
- Replay attacks using intercepted or duplicate payloads are blocked.
- Duplicate webhook deliveries are deduplicated before job queueing.
- Stale or expired deliveries are rejected before database queries.
