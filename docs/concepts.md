# Concepts

## Agents

An **agent** owns memories. Every request acts as exactly one agent, and an
agent can only ever read, search or change its own memories.

An agent's ID is the hex form of an ed25519 public key. There are two ways
to have one:

- **Account agent.** Signing in to the dashboard with your email creates
  one agent for your account. Your tools reach it with
  [API keys](authentication.md#api-keys) from `npx memro connect` or the
  dashboard. This is what most people use.
- **Self-owned agent.** An SDK generates a key pair on your machine
  (`Agent.create()`) and registers only the public key. Requests are
  [signed](authentication.md#signed-requests) with the private key, which
  never leaves your machine. Useful for autonomous agents and for one agent
  per end user in your own product.

## Memories

A memory is up to 10,000 characters of text plus metadata:

| Field | |
|---|---|
| `id` | UUID |
| `content` | The text |
| `memory_type` | `episodic`, `semantic`, `procedural` or `profile` (below) |
| `visibility` | `private` (default), `shared` or `public` |
| `created_at`, `document_date` | When it was stored |
| `event_date` | When the thing it describes happened, if known |
| `session_id` | Optional UUID grouping memories from one conversation |
| `is_atomic` | True for a single extracted fact rather than a raw chunk |
| `entity_tags` | Names, products and places found in the text |
| `confidence` | 0–1. Lowered when the engine consolidates or supersedes the memory |
| `access_count`, `last_accessed_at` | How often search returned it |
| `metadata` | Any JSON object you attach |
| `file_id`, `asset_url` | Set when the memory came from an uploaded file |

If you don't send `event_date`, Memro looks for one in the text ("last
Tuesday", "on 3 March 2026"). Entity tags are always extracted.

Visibility is stored and can be filtered on. Today every memory is readable
only by the agent that owns it, whatever its visibility.

### Memory types

| Type | Holds | Example |
|---|---|---|
| `episodic` | What happened | "We shipped billing v2 and it caused a spike in 500s." |
| `semantic` | What is true | "The billing service is written in Go." |
| `procedural` | How to act | "Run migrations before deploying the API." |
| `profile` | Who someone is | "Ana prefers short answers with code first." |

Send `memory_type: "auto"` (or leave it out over the REST API) and the
server classifies the text for you.

## Search

`GET /search` has three modes:

| Mode | How it ranks | Best for |
|---|---|---|
| `hybrid` (default) | Combines semantic similarity and keyword rank | General recall |
| `semantic` | Embedding similarity only | Paraphrased questions ("who runs infra?") |
| `keyword` | Postgres full-text rank | Exact names, IDs, error strings |

Search covers memories from the last 90 days. Semantic search needs an
embedding provider. Memro Cloud has one; a self-hosted server without one
falls back to keyword search. A newly stored memory is searchable by keyword
at once and by meaning a moment later, once its embedding is computed.

**Temporal search** (`GET /search/temporal`) adds date filters (created or
event date, before/after), session and atomic-only filters, and a recency
boost:

```
score = similarity × (1 − w) + recency × w     recency = 1 / (1 + age_in_days / 30)
```

`w` (`temporal_weight`) is 0.3 by default. Use 0 to ignore age, 1 to rank
purely by recency.

## The memory engine

On top of storage and search, Memro keeps a graph of how memories relate
(`updates`, `extends`, `derives`) and uses it:

| Endpoint | Does |
|---|---|
| `POST /mee/reason` | Answers a question from memory. Simple questions return the best match; *why/how* questions follow the graph from the top hits and return the path. |
| `POST /mee/evolve` | When there are 3+ episodic memories from the last 30 days, writes one consolidated semantic memory (linked to them with `derives`) and halves the episodes' confidence. Also links contradicting profile/semantic memories with `updates`. |
| `POST /mee/resolve` | Given two conflicting memories, picks the more trustworthy (recency, access frequency, confidence). |
| `GET /memory/chain/{id}` | A memory and every memory linked to it. |
| `GET /mee/timeline/{id}` | The audit history of a memory (for example when it was created). |
| `GET /analyze/conflicts` | How many contradictions exist and how many are unresolved. |

## Files

`POST /upload` stores a file and extracts its text: PDFs, plain text and
source code; images are described with a vision model and audio/video is
transcribed (both need the OpenAI provider). `POST /memory/from-file` then
turns that text into a memory. Shortcuts: `POST /upload/image` describes an
image and stores the description as a memory, and `POST /upload/document`
splits a PDF into ~1,500-character memories.

Uploaded files are served at `GET /assets/{file_id}`. The random file ID
works as a capability URL (usable in `<img>` tags), so treat it as a secret
if the file is. A file is deleted with the last memory made from it, and all
of an agent's files are deleted with the agent.
