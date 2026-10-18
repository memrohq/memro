# Decision log

Short records of the significant technical and product decisions behind
Memro: what we chose, the alternatives, and why. Add a new numbered file for
each decision; don't rewrite old ones — supersede them with a new record.

| # | Decision | Date | Status |
|---|---|---|---|
| [0001](0001-go-backend.md) | Rewrite the backend in Go | 2026-10-18 | Accepted |
| [0002](0002-postgres-pgvector-only.md) | Postgres + pgvector only (no Qdrant, no graph DB) | 2026-10-18 | Accepted |
| [0003](0003-monorepo-and-private-cloud.md) | Public monorepo + private memro-cloud | 2026-10-18 | Accepted |
| [0004](0004-polyform-noncommercial.md) | License: PolyForm Noncommercial 1.0.0 | 2026-10-19 | Accepted |
| [0005](0005-request-signing.md) | Request signing v1/v2 and key formats | 2026-10-18 | Accepted |
| [0006](0006-api-keys-and-device-login.md) | API keys and device-code login for `memro connect` | 2026-10-19 | Accepted |

Template:

```markdown
# NNNN. Title
Date: YYYY-MM-DD · Status: Proposed | Accepted | Superseded by NNNN

## Context
## Decision
## Alternatives considered
## Consequences
```
