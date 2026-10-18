# 0005. Request signing v1/v2 and key formats
Date: 2026-10-18 · Status: Accepted

## Context
Agents authenticate with ed25519. SDKs disagreed: Python signed
`timestamp || body`, JS and Go signed only the body, and deletes signed the
resource ID while sending an empty body. Key formats differed (32-byte seed vs
64-byte key). v1 signatures do not cover method/path/query, so a captured
signature could be replayed against another route within 5 minutes.

## Decision
- v1 (default for now): `ed25519(timestamp || body)`; every SDK signs the
  exact bytes sent (empty for GET/DELETE). `ALLOW_LEGACY_SIGNATURES` can turn
  v1 off.
- v2 (`X-Signature-Version: 2`): `timestamp \n METHOD \n path?query \n body`,
  with a replay cache. SDKs should move to v2.
- All SDKs accept 32-byte seeds and 64-byte keys.

## Consequences
Cross-SDK keys work. Next: switch SDKs to v2, then disable v1.
