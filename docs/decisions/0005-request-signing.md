# 0005. Request signing v1/v2 and key formats
Date: 2026-10-18 · Status: Accepted

## Context
Agents authenticate with ed25519. SDKs disagreed: Python signed
`timestamp || body`, JS and Go signed only the body, and deletes signed the
resource ID while sending an empty body. Key formats differed (32-byte seed vs
64-byte key). v1 signatures do not cover method/path/query, so a captured
signature could be replayed against another route within 5 minutes.

## Decision
- v1 (legacy): `ed25519(timestamp || body)`; disabled by default (`ALLOW_LEGACY_SIGNATURES=false`).
  When enabled for legacy clients, v1 requests are checked against the replay cache,
  increment `memro_legacy_signature_requests_total`, and log at INFO once per agent per hour
  (agent ID only; never signatures or bodies).
- v2 (`X-Signature-Version: 2`, default): `timestamp \n METHOD \n path?query \n body`,
  with replay protection and signature case normalization. All current SDKs send v2.
- All SDKs accept 32-byte seeds and 64-byte keys.

## Consequences
Cross-SDK keys work. Legacy v1 signatures are rejected by default. Older clients
can re-enable them via `ALLOW_LEGACY_SIGNATURES=true` while migrating.

### Note on Replay Cache Distribution
The replay cache is currently per-process. During a blue-green switch, or with more than one API instance, a request could be replayed on the other instance within the 5-minute window. If the API ever runs more than one instance, move it to Postgres (similar to `connector_webhook_deliveries`).

