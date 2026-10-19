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
{ "memory_nodes": 128, "graph_edges": 14, "reason_calls": 3, "deep_reason_calls": 0,
  "evolve_calls": 1, "warning": null }
```

`warning` is set when the agent passes 80% of its memory limit.

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
| `agent_id` | yours | |

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
`unknown`. Text is extracted from PDFs and text/code files; images are
described and audio/video transcribed when the server has an OpenAI key.
`extracted_text` is null when nothing could be extracted.

### 🔒 `POST /memory/from-file`

Body `{"file_id": "…", "memory_type": "semantic", "visibility": "private"}` →
`{"memory_id": "…", "file_id": "…", "content": "…"}`. The memory's content is
the extracted text (first 10,000 characters).

### 🔒 `POST /upload/image`

Form field `image`. Describes the image with a vision model and stores the
description as an episodic memory with `asset_url` pointing at the image.
Returns the memory. Needs an OpenAI key on the server.

### 🔒 `POST /upload/document`

Form field `document` (PDF). Splits the text into ~1,500-character semantic
memories and returns them as an array. Counts each chunk against the memory
limit.

### `GET /assets/{file_id}`

Serves an uploaded file. No credential: the random file ID acts as the
access token.

---

## Webhooks

### 🔒 `POST /hooks`

Body `{"event_type": "memory_created", "url": "https://…"}` → `201`.
See [Webhooks](webhooks.md).

### 🔒 `GET /hooks/{event_type}`

The agent's webhook URLs for that event: `["https://…"]`.

### 🔒 `DELETE /hooks`

Body `{"event_type": "memory_created", "url": "https://…"}` → `204`, or `404`.

---

## Identities

### `POST /identity`

Registers a self-owned agent. Body `{"public_key": "<64 hex chars>"}` →
`{"agent_id": "<same>"}`. Registering an existing key changes nothing.

### `GET /identity/{agent_id}`

`{"id": "…", "user_id": null, "tenant_id": "…", "created_at": "…"}`

### 🔒 `DELETE /identity/{agent_id}`

Deletes the agent with all its memories, files, API keys and webhooks.
`204`. Irreversible.

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

## Health

### `GET /health`

```json
{ "status": "ok", "db": "ok", "vector_store": "ok", "embeddings": "enabled" }
```

`503` with `"status": "degraded"` when the database is unreachable.
