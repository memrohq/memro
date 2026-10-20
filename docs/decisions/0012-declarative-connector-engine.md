# 0012. Declarative Connector Engine (DCE)
Date: 2026-10-21 · Status: Accepted

## Context
Memro needs to connect to dozens of engineering, documentation, project management, and CRM tools (Asana, ClickUp, Linear, Jira, Datadog, Airtable, etc.) so agents can recall cross-tool context. Writing bespoke Go boilerplate for each conventional REST API creates maintenance overhead, increases code bloat, and slows down integration delivery.

However, completely replacing native Go code with generic configuration would break specialized connectors (Slack Events API, GitHub Apps, ed25519 Discord webhooks, Atlassian 3LO OAuth) that require complex handshakes, cryptographic signatures, or event streaming.

## Decision
- **Adopt a Hybrid Architecture:**
  - **80–90% Conventional Integrations:** Declared via YAML/JSON manifests (`apiVersion: memro.dev/v1`, `kind: Connector`) and executed by a shared Go runtime (`server/internal/connectors/dce`).
  - **10–20% Specialized Integrations:** Retain custom Go adapters (Slack, GitHub, Discord) for non-standard protocols, while sharing common storage and diffing sinks.
- **Manifest Contract (`memro.dev/v1`):**
  - Defines `spec.baseURL`, `spec.auth` (`bearer`, `apiKey`, `basic`), operations, pagination (`cursor`, `page`, `offset`), JSONPath field mappings (`$.id`, `$.title`, `$.description`, `$.updatedAt`), incremental sync checkpoints, and policies (`timeout`, `maxRetries`, `rateLimitPerMin`).
- **Compile-Time Validation:**
  - Manifests are validated and compiled into immutable execution plans at registration time, preventing runtime schema parse failures.
- **SSRF Defense & Resource Limits:**
  - The runtime rejects loopback (`127.0.0.0/8`, `::1`), link-local metadata (`169.254.169.254`), and private RFC 1918 addresses unless explicitly overridden in test fixtures.
  - Response bodies are capped at 10MB via `io.LimitReader` to prevent memory exhaustion attacks.
  - Pagination loops are bounded (default 50 pages) with backoff on HTTP 429 `Retry-After`.
- **Credential Isolation & Redaction:**
  - Secret references (`secretRef`) are resolved via `SecretResolver` from sealed AES-256-GCM storage; raw tokens are never persisted in manifest files.
  - Error messages pass through sanitization to guarantee auth tokens are never leaked in logs (`[REDACTED]`).
- **Two-Tier Rollout Strategy:**
  - **Group A (Token-Based):** 50 standard SaaS tools using personal API tokens (pasted in settings) for instant activation without multi-tenant app registration overhead.
  - **Group B (OAuth / App Registration):** Specialized multi-tenant integrations (Google Drive, Microsoft 365, Atlassian) requiring custom OAuth flows.

## Consequences
- Fast addition of new data sources by writing single declarative YAML manifests in `catalog/`.
- Zero duplicated code for HTTP retries, pagination loops, and JSONPath normalization.
- Safe execution in multi-tenant environments with strict SSRF and memory ceilings.
