# API reference

Base URL: `https://api.memro.co` (self-hosted: `http://localhost:8081`).
Requests and responses are JSON unless noted. Send a credential as described
in [Authentication](authentication.md); 🔒 marks endpoints that need one.
Errors return a non-2xx status with a plain-text message (see
[Limits and errors](limits-and-errors.md)).

Wherever an endpoint takes an `agent_id` (in the path, query or body) it
must be the authenticated agent's own ID, or you get `403`. When it is
optional, it defaults to the authenticated agent.

- [Account](#account) · [Memories](#memories) · [Search](#search) ·
  [Memory engine](#memory-engine) · [Files](#files) · [Webhooks](#webhooks) ·
  [Identities](#identities) · [API keys](#api-keys) · [Health](#health)

---

## Account

### 🔒 `GET /auth/whoami`

```json
{ "agent_id": "3f9a…", "tenant_id": "00000000-…", "user_id": "b1c2…", "auth": "api_key" }
```

`auth` is `api_key`, `signature` or `session`.

### 🔒 `GET /usage`

Today's usage for the agent.

```json
{ "memory_nodes": 128, "memory_limit": 1000, "graph_edges": 14, "reason_calls": 3,
  "deep_reason_calls": 0, "evolve_calls": 1, "warning": null }
```

`memory_limit` is 0 when there is no limit. `warning` is set when the agent
passes 80% of it.

### 🔒 `GET /stats/temporal`

```json
{ "total_memories": 128, "memories_today": 4, "memories_this_week": 19, "memories_this_month": 61,
  "average_days_between_memories": 0.7,
  "memory_types_distribution": { "episodic": 70, "semantic": 40, "procedural": 8, "profile": 10 } }
```

---

## Memories

### 🔒 `POST /memory`

| Field | Type | |
|---|---|---|
| `content` | string, required | Up to 10,000 characters |
| `memory_type` | string | `episodic` · `semantic` · `procedural` · `profile` · `auto`. Omitted or `auto`: the server classifies it |
| `visibility` | string | `private` (default) · `shared` · `public` |
| `event_date` | RFC 3339 | When it happened. Omitted: detected from the text if possible |
| `session_id` | UUID | Groups memories from one conversation |
| `is_atomic` | bool | A single fact rather than a raw chunk |
| `metadata` | object | Any JSON |
| `agent_id` | string | Optional; must be your own |

Returns the stored [memory](concepts.md#memories):

```json
{ "id": "8d0e…", "agent_id": "3f9a…", "tenant_id": "00000000-…", "content": "Ana leads the platform team",
  "memory_type": "profile", "visibility": "private", "created_at": "2026-10-19T08:00:00Z",
  "document_date": "2026-10-19T08:00:00Z", "event_date": null, "session_id": null, "is_atomic": false,
  "entity_tags": ["Ana"], "relationship_types": [], "access_count": 0,
  "last_accessed_at": "2026-10-19T08:00:00Z", "confidence": 1, "metadata": null,
  "file_id": null, "asset_url": null }
```

`403` when the agent has reached its memory limit.

### 🔒 `GET /memory/agent/{agent_id}`

Most recent memories first. Query: `limit` (default and max 100),
`memory_type`, `visibility`. Returns an array of memories.

### 🔒 `DELETE /memory/{id}`

`204`, or `404` if the agent has no such memory. If the memory was made
from an uploaded file and no other memory uses that file, the file is
deleted too.

### 🔒 `GET /export/{agent_id}`

Every memory, for backups and portability.

```json
{ "agent_id": "3f9a…", "exported_at": "2026-10-19T08:00:00Z", "total": 128, "memories": [ … ] }
```

### 🔒 `POST /memory/generate-atoms`

Splits text into atomic facts without storing them.
Body `{"chunk": "…"}` → `{"atomic_memories": ["…"], "chunk_preview": "…"}`.

---

## Search

### 🔒 `GET /search`

| Query | | |
|---|---|---|
| `query` | required | Up to 500 characters |
| `mode` | `hybrid` | `hybrid` · `semantic` · `keyword` ([how they differ](concepts.md#search)) |
| `limit` | 10 | Max 50 |
| `memory_type` | | Filter |
| `max_per_session` | off | Optional: at most this many results per conversation (`session_id`, or source file) before the rest fill in by score. Broadens results across conversations, but on LongMemEval it returned fewer of the exact exchanges holding answers, so it is off by default. |
| `agent_id` | yours | |

`keyword` mode matches memories containing any of the query's words (ranked by
how many, and how close together), so a natural-language question finds them.
`hybrid` combines semantic and keyword candidates with equal weight.

```json
{ "results": [ { "memory_id": "8d0e…", "content": "Ana leads the platform team", "score": 0.91,
                 "memory_type": "profile", "created_at": "2026-10-19T08:00:00Z", "asset_url": null } ],
  "mode": "hybrid", "latency_ms": 42, "total": 1 }
```

Covers the last 90 days. Identical searches are cached for up to 5 minutes;
the cache is cleared when the agent's memories change.

### 🔒 `GET /search/temporal`

| Query | | |
|---|---|---|
| `query` | required | |
| `after_date`, `before_date` | RFC 3339 | Filter on when the memory was stored |
| `after_event_date`, `before_event_date` | RFC 3339 | Filter on when the event happened |
| `session_id` | UUID | |
| `atomic_only` | bool | |
| `temporal_weight` | 0.3 | 0..1, [how much recency counts](concepts.md#search) |
| `limit` | 10 | Max 50 |

```json
{ "results": [ { "memory_id": "8d0e…", "content": "Shipped billing v2", "document_date": "2026-10-01T09:00:00Z",
                 "event_date": "2026-10-01T00:00:00Z", "is_atomic": false, "score": 0.74,
                 "temporal_context": "Observed: 2026-10-01, Occurred: 2026-10-01" } ],
  "total": 1 }
```

---

## Memory engine

See [Concepts → The memory engine](concepts.md#the-memory-engine).

### 🔒 `POST /mee/reason`

Body `{"query": "why did 500s spike after the billing release?"}`.

```json
{ "answer": "I've identified a causal link between your memories: …", "why": "Expanded graph from memory …",
  "based_on": ["8d0e…", "1a2b…"], "confidence": 0.82, "paths": [["8d0e…", "1a2b…"]] }
```

Rate limit: 20 per minute per agent.

### 🔒 `POST /mee/evolve`

No body. → `{"memories_evolved": 5, "patterns_identified": ["…"], "diff": {…}}`.
Rate limit: 5 per minute.

### 🔒 `POST /mee/resolve`

Body `{"memory_a_id": "…", "memory_b_id": "…"}` (both yours) →
`{"Resolved": "<winning id>"}` or `{"Ambiguous": ["<a>", "<b>", "<expires_at>"]}`.
Rate limit: 10 per minute.

### 🔒 `GET /mee/timeline/{memory_id}`

```json
{ "memory_id": "8d0e…", "events": [ { "timestamp": "2026-10-19T08:00:00Z", "action": "memory.create",
                                     "details": "Action performed on memory 8d0e…" } ] }
```

### 🔒 `GET /memory/chain/{memory_id}`

```json
{ "root_memory": { "memory_id": "8d0e…", "content": "…", "relation_type": null, "confidence": null },
  "related_memories": [ { "memory_id": "1a2b…", "content": "…", "relation_type": "updates", "confidence": 0.9 } ] }
```

### 🔒 `GET /analyze/conflicts`

`{"total_conflicts": 3, "unresolved_conflicts": 1, "avg_resolution_time_sec": 0}`

---

## Files

Multipart uploads. The request body limit is 10 MB.

### 🔒 `POST /upload`

Form fields: `file` (required), `agent_id` (required, your own).

```json
{ "file_id": "c4d5…", "file_name": "notes.pdf", "file_type": "pdf", "file_size": 48213,
  "extracted_text": "Quarterly planning starts in March…" }
```

`file_type` is `pdf`, `image`, `audio`, `text`, `docx`, `xlsx`, `pptx` or
`unknown`. Text is extracted from PDF, Word, PowerPoint, Excel and text/code
files; images are described and audio/video transcribed when the server has
an OpenAI key. `extracted_text` is null when nothing could be extracted
(legacy .doc/.xls/.ppt are stored but not read).

### 🔒 `POST /memory/from-file`

Body `{"file_id": "…", "memory_type": "semantic", "visibility": "private"}` →
`{"memory_id": "…", "memory_ids": ["…"], "chunks": 1, "file_id": "…", "content": "…"}`.
The memory's content is the extracted text. Text longer than 10,000
characters is [chunked](concepts.md#chunking) into several memories:
`memory_ids` lists them and `memory_id` is the first.

### 🔒 `POST /upload/image`

Form field `image`. Describes the image with a vision model and stores the
description as an episodic memory with `asset_url` pointing at the image.
Returns the memory. Needs an OpenAI key on the server.

### 🔒 `POST /upload/document`

Form field `document`: a PDF, Word, PowerPoint, Excel or text file (type
detected from the name, or the content). Stores the file and its text as
[chunks](concepts.md#chunking) of up to 1,500 characters, returned as an
array of memories, each with `file_id` and `metadata.chunk`/`chunks`. Each
chunk counts against the memory limit. `400` for unsupported or unreadable
files.

### `GET /assets/{file_id}`

Serves an uploaded file. No credential: the random file ID acts as the
access token.

---

## Webhooks

### 🔒 `POST /hooks`

Body `{"event_type": "memory_created", "url": "https://…"}` → `201`.
See [Webhooks](webhooks.md). A webhook added with an API key is deleted when
that key is revoked; add it with a dashboard session or a signed request to
keep it.

### 🔒 `GET /hooks/{event_type}`

The agent's webhook URLs for that event: `["https://…"]`.

### 🔒 `DELETE /hooks`

Body `{"event_type": "memory_created", "url": "https://…"}` → `204`, or `404`.

---

## Identities

### `POST /identity`

Registers a self-owned agent. Body
`{"public_key": "<64 hex chars>", "pow": {"timestamp": <unix seconds>, "nonce": "<1-64 letters or digits>"}}`
→ `{"agent_id": "<same>"}`. Registering an existing key changes nothing.

No account is needed, so registering costs a little work and is rate-limited:
- **Proof-of-work.** `pow` is a nonce whose SHA-256 over
  `memro-identity-v1:<public_key>:<timestamp>:<nonce>` starts with
  `difficulty` zero bits (see `GET /identity/pow`), with a timestamp within
  10 minutes of server time. The SDKs solve it for you (about a second). A
  proof that is sent is always checked (`400` if invalid); servers that set
  `IDENTITY_POW_REQUIRED=true` also reject registrations without one.
- **Rate limit.** Each client address may register 20 agents an hour (`429`
  beyond that).

### `GET /identity/pow`

The proof-of-work a registration needs:
`{"algorithm": "sha256", "message": "memro-identity-v1:<public_key>:<timestamp>:<nonce>", "difficulty": 20, "required": false, "max_age_seconds": 600, "nonce": "1-64 letters or digits"}`.
Servers older than this endpoint answer `404`; register without a proof.

### `GET /identity/{agent_id}`

`{"id": "…", "created_at": "…"}`: whether the agent is registered. Its
owner and tenant are not public; an agent sees its own with
`GET /auth/whoami`.

### 🔒 `DELETE /identity/{agent_id}`

Deletes the agent with all its memories, files, API keys and webhooks.
`204`. Irreversible. Needs a dashboard session or a request signed with the
agent's private key; API keys get `403`, so a key leaked from a tool's config
cannot destroy the agent.

### Agent keys (ADR 0013)

An agent's ID never changes; these are the keys allowed to sign for it.
`signing` keys act for the agent; `recovery` keys may only use the three
routes below. Name the signing key in `X-Key-Id`; without it the key is the
agent ID (the original key). Managed by the agent itself (signed request) or
its owner (dashboard session); API keys get `403`.

| | |
|---|---|
| 🔒 `GET /identity/{agent_id}/keys` | `[{"public_key", "role", "created_at", "revoked_at"}]` |
| 🔒 `POST /identity/{agent_id}/keys` | Body `{"public_key", "role", "proof", "proof_timestamp"}` → `201`. `proof` is the new key's hex signature over `"memro-agent-key\n{agent_id}\n{public_key}\n{proof_timestamp}"` (Unix seconds, within 5 minutes). `409` if the key belongs to an agent already; keys are never reused. |
| 🔒 `DELETE /identity/{agent_id}/keys/{public_key}` | `204`. `409` for the last signing key of an unowned agent with no recovery key. |

---

## API keys

Managed from the dashboard (session) or by a self-owned agent (signed
request). API keys can't manage API keys (`403`).

| | |
|---|---|
| `GET /api-keys` | `[{"id", "name", "prefix", "created_at", "last_used_at"}]` |
| `POST /api-keys` | Body `{"name"}` (≤ 60 chars) → `{"id", "name", "prefix", "key"}`. `key` is shown only here. |
| `DELETE /api-keys/{id}` | `204` |

Device login endpoints are described in
[Authentication](authentication.md#device-login-how-npx-memro-connect-works).

---

## Tenants

Platform administration (ADR 0012). Ordinary accounts live in the shared
default tenant and get `403` from all of these.

| | |
|---|---|
| 🔒 `POST /tenant` | Body `{"name", "domain"}` → `{"id", "name", "domain", "created_at"}`. Platform admins only. |
| 🔒 `GET /tenant/{id}` | Platform admins, or an agent of that dedicated tenant. |
| 🔒 `GET /tenant/{id}/usage?hours=24` | Same rule; `hours` is capped at 2160 (90 days). |

Platform admins act only through a dashboard session; their API keys and
signed requests get `403`.

---

## Connectors

Slack and GitHub connections that sync into the agent's memory (see
[Connectors](connectors.md)). Dashboard session only: API keys and signed
requests get `403`.

| | |
|---|---|
| `GET /connectors` | `{"providers": [{"name", "enabled", "sources_managed_by"}], "connections": [Connection]}` |
| `POST /connectors/{provider}/authorize` | `provider` is `slack` or `github` → `{"url"}`. Open `url` in the same browser; it sets a short-lived cookie checked on the way back. `503` if the connector isn't configured. |
| `GET /connectors/{id}/sources` | `[{"id", "name", "private", "enabled", "last_synced_at"}]` — Slack channels or GitHub repositories |
| `PUT /connectors/{id}/sources` | Slack only. Body `{"enabled": {"C0123": true, "C0456": false}}` (≤ 500) → the updated list. `400` with a message for a channel you can't use. |
| `POST /connectors/{id}/sync` | Sync now; also resumes a paused connection. `202` |
| `DELETE /connectors/{id}` | Disconnect. `?delete_memories=true` also deletes its memories. → `{"deleted_memories": n}` |

A `Connection` is `{"id", "provider", "account_id", "account_name",
"external_user_id", "external_user_name", "status", "status_detail",
"created_at", "last_synced_at", "sources_enabled", "sources_total",
"memories", "pending_jobs", "manage_url"}`. `status` is `active`, `paused`
(memory limit reached, or suspended on GitHub), `error` (sync keeps failing;
`status_detail` says why) or `revoked` (reconnect to use it again).

Called by providers, not by you: `GET /connectors/{provider}/callback`
(OAuth redirect), `POST /connectors/slack/events` (Slack signature) and
`POST /connectors/github/webhook` (`X-Hub-Signature-256`).

---

## Coordination

Multi-agent coordination for claiming work, heartbeating, releasing, handing off, and declaring non-blocking advisory intents across agents in the same account or space.

### 🔒 `POST /coordination/claim`

Acquire an exclusive atomic lease on a normalized resource path.

Request body:
```json
{
  "resource": "src/auth.go",
  "intent": "refactor oauth flow",
  "ttl_seconds": 900
}
```

Response (granted):
```json
{
  "granted": true,
  "lease_id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  "expires_at": "2026-10-23T01:15:00Z",
  "handoff_summary": "Previous agent finished interface definitions"
}
```

Response (denied - held by another agent):
```json
{
  "granted": false,
  "holder": "agent-pubkey-hex",
  "holder_session": "default",
  "intent": "implementing jwt parsing",
  "expires_at": "2026-10-23T01:10:00Z"
}
```

Re-claiming your own active lease is idempotent and extends it.

### 🔒 `POST /coordination/heartbeat`

Extend an active lease held by the calling agent.

```json
{
  "resource": "src/auth.go",
  "ttl_seconds": 1800
}
```
→ `{"renewed": true, "expires_at": "2026-10-23T01:30:00Z"}`

### 🔒 `POST /coordination/release`

Release a held lease. Status is `completed` or `abandoned`.

```json
{
  "resource": "src/auth.go",
  "status": "completed"
}
```
→ `{"released": true, "resource": "src/auth.go"}`

### 🔒 `POST /coordination/handoff`

Atomically releases the lease with status `handoff`, storing a summary and optional target agent ID for the next claimer.

```json
{
  "resource": "src/auth.go",
  "summary": "Completed token parser, needs error test coverage",
  "next_agent_id": "target-agent-id"
}
```
→ `{"handed_off": true, "resource": "src/auth.go"}`

### 🔒 `POST /coordination/check`

Advisory check for lease and intent conflicts without claiming.

```json
{
  "resource": "src/auth.go",
  "intent": "review auth flow"
}
```
→ `{"decision": "allow|warn|block", "reason": "...", "details": {...}, "intents": [...]}`.
- `allow`: Unclaimed, expired, or held by calling agent.
- `warn`: Active non-blocking intent logged by another agent in the same scope.
- `block`: Active exclusive lease held by another agent in the same scope.

### 🔒 `GET /coordination/active`

List active leases and intents in the calling agent's scope. Optional query param `?resource=prefix`.

→ `{"leases": [...], "intents": [...]}`

### 🔒 `POST /coordination/intent`

Declare a soft, non-blocking intention for a resource without acquiring a lease.

```json
{
  "resource": "src/auth.go",
  "intent": "planning migration to v2",
  "ttl_seconds": 1800
}
```
→ `{"logged": true}`

---

## Health

### `GET /health`

```json
{ "status": "ok", "db": "ok", "vector_store": "ok", "embeddings": "enabled" }
```

`503` with `"status": "degraded"` when the database is unreachable.

