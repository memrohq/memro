# 0014. Declarative Connector Engine (DCE)
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

## Revision: production review (2026-10-21)

A review before production found the first catalog could not work for most
services, and the engine could not express what their APIs need. Changes:

- **Per-connection fields.** A manifest declares the form the user fills in
  (`spec.fields`: the token plus account details such as a subdomain, site,
  workspace or second key). Values are templated into the base URL, paths,
  query, headers and bodies (`{{config.x}}`); a field used in a URL host must
  have a pattern, so it cannot point requests at another host.
- **Connect-time check.** `spec.verify` is a cheap request that proves the
  token works and names the account. A bad token is refused before anything
  is saved.
- **Checkpoint is not a page cursor.** The first engine sent the saved
  checkpoint (a timestamp) as the next page's cursor, so 11 connectors failed
  from their second sync. A sync now keeps the pass's page cursor, the last
  finished pass's checkpoint (sent only as a filter, `{{checkpoint}}`) and
  the highest value seen; the checkpoint advances only when a pass finishes.
- **Bounded jobs.** One job reads at most 5 pages, saves its position and
  queues the next; a timeout applies per request (not to the whole run), so
  large accounts finish across jobs instead of failing at a deadline.
- **More API shapes.** Pagination by next URL and `Link` header, zero-based
  pages, offsets in JSON bodies, GraphQL bodies with JSON-safe values
  (`{{cursor|json}}`), shared headers, `basic` auth from templates, `query`
  auth, source lists with filters (`where`), per-operation mappings,
  `{{#each}}` blocks and filters (`text` for HTML, `date`, `unix`, `or`,
  `kv`, `list`). Mapped text is deterministic, so unchanged items are not
  re-embedded.
- **Newest-first sources** stop paging at the last checkpoint less an
  `overlap` (deploys 6 h, incidents up to 14 days for later status changes).
- **Hardened HTTP.** Private, loopback, metadata, CGNAT, NAT64/6to4 and
  multicast addresses are refused when the connection is made (not only
  when the URL is checked); redirects to another host are refused; retries
  resend the body; secrets are redacted from errors, including query auth.
- **Job safety.** A keyed job (connection, kind, key) is not claimed while
  the same work is leased, so one source never syncs twice at once. A 401
  revokes a catalog connection; 429 waits.
- **Catalog.** 43 manifests written against each service's documentation,
  each tested against a fake API built from the manifest (connect, sources,
  every operation) plus contract tests of exact headers and bodies. Dropped:
  Pocket, Height, Obsidian Publish, Basecamp, Help Scout, Railway, GitBook,
  Slite (see docs/connectors.md for why).

The sign-in connectors were fixed in the same review: Linear, Sentry, Notion,
Jira and GitLab are listed as sign-in (not token) connectors; expiring tokens
are refreshed under a per-connection lock; Jira uses `/search/jql` with
pagination; 403 no longer revokes a whole connection; webhooks require their
secret, verify signatures (Notion's `sha256=` prefix, no Jira URL secret) and
reach only connections that chose that source; Linear and Notion items
fetched by webhook are checked against the chosen teams and databases.
