# 0009. SDK 0.2: one API across languages, v2 signing, Memro Cloud by default
Date: 2026-10-19 · Status: Accepted

## Context
The JavaScript and Go SDKs no longer matched the server: identity creation
sent no public key, listing used a removed path, search and export were sent
unauthenticated, and neither supported API keys. The Python SDK worked but
covered only memories and search, and all three signed with the legacy v1
payload. The JS SDK had no build configuration and no CI. The MCP server
called the API with hand-built requests and ran blocking SDK calls on its
event loop.

Separately, deleting an agent left its uploaded files on disk and served at
`/assets/{id}`, and there was no way to delete a file or a webhook.

## Decision
- **SDKs 0.2 (Python, JavaScript, Go) share one surface**: API-key or
  self-owned (signed) agents; remember, recall (hybrid/semantic/keyword),
  temporal recall, recent, delete, export; reason, evolve, resolve, chain,
  timeline, conflicts, stats, usage; file, image and PDF upload; webhooks.
- **Default base URL is `https://api.memro.co`** (overridable with
  `MEMRO_BASE_URL`). API keys are the primary credential; keys must start
  with `memro_sk_`.
- **v2 signatures only** in new SDK code (legacy helpers kept, deprecated).
  The SDK builds the exact path and query it signs. Because ed25519 is
  deterministic, an identical signed request in the same second is refused
  as a replay; SDKs wait for the next second and re-sign once.
- **Retries**: 429 always (honouring `Retry-After`); 502–504 only for GET and
  DELETE, so writes are never duplicated.
- **JS SDK** uses `fetch` (Node 18+, Deno, Bun, browsers), ships ESM and
  CommonJS built with `tsc`, and depends only on `tweetnacl`.
- **MCP server** uses the SDK for every tool and runs tool calls in a worker
  thread. Stub tools now call real endpoints; a duplicate tool is removed.
- **Deletion**: deleting an agent deletes its files from the database and
  disk; deleting the last memory made from a file deletes the file;
  `DELETE /hooks` removes a webhook.
- **CI** builds and tests the JS SDK, and runs every SDK and every MCP tool
  against a real API with legacy signatures disabled.

## Consequences
- SDK users who relied on the old `http://localhost:8081` default must set
  `MEMRO_BASE_URL` (self-hosting docs say so).
- The VS Code extension now uses Streamable HTTP with an API key stored in
  VS Code's secret storage; private-key settings were removed.
- Packages are not yet published to npm/PyPI; docs give source installs
  until they are.
