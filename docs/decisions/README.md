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
| [0007](0007-no-private-keys-in-the-browser.md) | No private keys in the browser; one credential per surface | 2026-10-19 | Accepted |
| [0008](0008-security-review-fixes.md) | Per-agent webhooks, credentialed MCP connections, real client IPs | 2026-10-19 | Accepted |
| [0009](0009-sdk-v0-2-and-deletion.md) | SDK 0.2: one API across languages, v2 signing, Memro Cloud by default | 2026-10-19 | Accepted |
| [0010](0010-connector-hardening.md) | Connector hardening: untrusted memory content, working claims, safer config writes | 2026-10-19 | Accepted |
| [0011](0011-slack-and-github-connectors.md) | Slack and GitHub connectors | 2026-10-20 | Accepted |
| [0012](0012-identity-accounts-and-spaces.md) | Users, agents, accounts and Spaces; API keys cannot delete agents; platform admins | 2026-10-20 | Accepted |
| [0013](0013-agent-keys-claims-and-expiry.md) | Agent identity separate from its keys; claiming and expiry | 2026-10-21 | Accepted (steps 1 and 4 built) |
| [0014](0014-declarative-connector-engine.md) | Declarative Connector Engine (DCE) | 2026-10-21 | Accepted |
| [0015](0015-verified-code-memory.md) | Verified memory for coding agents: anchors, validity and provenance | 2026-10-21 | Proposed |
| [0016](0016-metrics-token-protection.md) | Metrics endpoint token protection (`METRICS_TOKEN`) | 2026-10-22 | Accepted |
| [0017](0017-webhook-replay-protection.md) | Webhook replay protection for GitHub, Linear and Sentry | 2026-10-22 | Accepted |
| [0018](0018-tenant-agent-scoping-audit.md) | Tenant and agent scoping audit & cross-agent isolation | 2026-10-22 | Accepted |
| [0019](0019-server-side-coordination.md) | Server-side multi-agent coordination leases and intents | 2026-10-22 | Accepted |

Template:

```markdown
# NNNN. Title
Date: YYYY-MM-DD · Status: Proposed | Accepted | Superseded by NNNN

## Context
## Decision
## Alternatives considered
## Consequences
```
