# 0008. Per-agent webhooks, credentialed MCP connections, real client IPs
Date: 2026-10-19 · Status: Accepted

## Context
A code review on 2026-10-19 found authorization and rate-limiting
weaknesses in the hosted MCP server, webhooks, email sign-in and client IP
handling, plus some metering and caching gaps. This record describes the
resulting design.

## Decision
- **MCP over HTTP requires a credential on every connection.** `/mcp` and
  `/sse` accept a `memro_sk_` API key (Authorization header, or `token=`
  for SSE clients that cannot set headers). An SSE connection is bound to
  the agent it authenticated as; there is no shared session cache. Private
  keys are refused unless `MCP_ALLOW_PRIVATE_KEY_AUTH=1` (self-hosting
  only). Environment credentials and auto-created identities apply to the
  local stdio server only. `/webhook/{agent_id}` is removed.
- **Webhooks belong to an agent** (`hooks.agent_id`, migration 0023) and only
  fire for that agent's memories. Existing hooks had no known owner and are
  deleted; the tenant-wide unique constraint is dropped.
- **Sign-in codes**: each attempt is reserved atomically before the code is
  compared, a code is consumed atomically (single use), and an email can
  request at most 5 codes an hour (≤ 25 guesses an hour).
- **Client IP**: behind a trusted proxy the API uses `X-Real-IP`, which the
  edge sets to the real client (`header_up X-Real-IP {client_ip}`), and
  otherwise the last `X-Forwarded-For` entry. The MCP server forwards the end
  user's address as `X-Real-IP` on its API calls.
- PDF chunks are metered as `memories_stored`; the indexer drops an agent's
  cached search results once its new embeddings are stored; the MCP server
  logs to stderr at `MCP_LOG_LEVEL` (default INFO) and never logs arguments.

## Alternatives considered
- Removing `/sse` outright: the VS Code extension still uses it, so it stays
  but behind the same API-key check as `/mcp`.
- Keeping webhooks per tenant and blocking them for the default tenant:
  would make webhooks unusable on Memro Cloud.
- Keeping the old hooks unique constraint for one release: it silently
  dropped a second agent's hook for the same URL.

## Consequences
- Legacy SSE clients that authenticated with an agent private key must
  switch to an API key (`npx memro connect` or Dashboard → API keys).
- During the blue-green switch that applies migration 0023, the previous
  version's `POST /hooks` fails for a few seconds.
- Self-hosters behind a proxy should have it set `X-Real-IP`; with only
  `X-Forwarded-For`, the proxy must append the client address last.
