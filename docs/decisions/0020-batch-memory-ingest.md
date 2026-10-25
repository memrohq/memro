# 0020. Batch Memory Ingest Endpoint
Date: 2026-10-23 · Status: Accepted

## Context

Previously, agents and benchmarks ingested memories sequentially via `POST /memory` (or through connectors). While single-item ingestion is well-suited for interactive turn-by-turn agent loops, it is inefficient for bulk operations:
1. **Benchmark and migration overhead**: In benchmark harnesses like LongMemEval (500 sessions, thousands of conversation turns) or when importing conversation histories, single-item HTTP calls introduce significant round-trip network latency, repeated HTTP header parsing, and repetitive authorization checks.
2. **Database round-trips**: Running individual inserts sequentially strains connection pools and increases cumulative write latency.
3. **Lack of atomic batch boundaries**: In multi-turn dialogue ingestion, an agent may want to persist an entire session's turns atomically. If ingestion fails midway, partial writes leave corrupted or incomplete conversational context.

## Decision

We introduce a dedicated atomic batch memory ingestion endpoint: `POST /memories/batch`.

### 1. Endpoint Interface & Payload Formats

The endpoint accepts up to 500 items per request and supports both raw array and wrapped object formats:
- Raw array: `[ { "content": "...", "type": "semantic", ... }, ... ]`
- Wrapped object: `{ "memories": [ ... ] }` or `{ "items": [ ... ] }`

Each memory item can specify its own:
- `content` (required, 1 to 10,000 characters)
- `type` / `memory_type` (`semantic` | `episodic` | `procedural`; default: `semantic`)
- `visibility` (`public` | `shared` | `private`; default: `private`)
- `session_id` (UUID format; automatically assigned a new UUID if omitted)
- `event_date` (RFC3339 timestamp string; default: `NOW()`)
- `metadata` (arbitrary JSON object)
- `agent_id` (optional; if provided, must match the authenticated agent)

### 2. Authentication & Authorization

The endpoint enforces identical authentication rules to `POST /memory`:
- **Signed Agent Requests**: Verified against the agent's registered public Ed25519 key (v1 or v2 request signing).
- **API Key Authentication**: Memro Cloud Bearer token authentication (`memro_sk_...`) resolving to the caller's agent identity.

### 3. Strict All-or-Nothing Pre-Validation

To guarantee transactional integrity:
1. The request payload is validated upfront before opening a database transaction.
2. The batch size is strictly bounded between 1 and 500 items. Empty batches or batches exceeding 500 items return HTTP 400 (`Bad Request`).
3. Each individual item is checked for valid content length, supported memory type, valid visibility, RFC3339 event date, valid UUID session format, and agent identity consistency.
4. If **any single item** fails validation, the entire request is rejected immediately with HTTP 400. No partial writes occur.

### 4. Quota and Cap Enforcement

Before committing writes:
- **Rate limits & token quotas**: The API gateway calculates the cumulative token count across all items in the batch and checks against tenant/user rate limits and monthly quotas (`gate.AllowMemoryTokens`).
- **Unclaimed agent caps**: For self-signed unclaimed agents, the system validates that adding `N` memories will not exceed the 100-memory lifetime cap (`allowWrite`).

### 5. Single Database Transaction & Pipelined Execution

- Ingestion executes within a single Postgres transaction (`pgx.Tx`).
- Writes are executed using `pgx.Batch` via `SendBatch`, streaming all `INSERT INTO memories (...)` queries to PostgreSQL in a single network round-trip.
- If any database error occurs during execution, the entire transaction rolls back.

### 6. Response Contract & Output Ordering

The endpoint responds with HTTP 201 Created and JSON payload:
```json
{
  "ids": ["uuid-1", "uuid-2", ...],
  "count": 2,
  "memories": [...]
}
```
The returned `ids` and `memories` lists **strictly preserve the exact input order** of the submitted items.

### 7. Asynchronous Indexing & Side Effects

To keep response latencies sub-second:
- All created memory IDs are enqueued to the asynchronous background indexer (`indexer.Enqueue`), which batches embedding generation and vector index updates.
- Query caches are invalidated.
- Outbound webhooks and usage metrics are emitted.
- An audit log entry is recorded for the batch operation.

### 8. SDK Implementations

Unified SDK methods are added across all supported languages:
- **Python**: `client.create_memories_batch(agent_id, private_key, memories)`, `agent.remember_batch(memories)`
- **TypeScript / JavaScript**: `client.createMemoriesBatch(memories, signer)`, `agent.rememberBatch(items)`
- **Go**: `client.CreateMemoriesBatch(ctx, memories, signer)`, `agent.RememberBatch(ctx, items)`

## Alternatives Considered

1. **Postgres `COPY FROM`**: While bulk `COPY` has minimal overhead for millions of records, it bypasses fine-grained validation, complicates returning generated UUIDs in input order, and is unnecessary for typical 100–500 item operational batches.
2. **Synchronous embedding generation**: Generating vector embeddings for 500 items synchronously inside the HTTP handler would cause 10–30 second request latencies and frequent HTTP gateway timeouts. Queuing to the existing background indexer maintains sub-second HTTP responses.
3. **Partial success / error reporting**: Returning individual success/error arrays (e.g. 207 Multi-Status) leaves partial states in multi-turn dialogues and complicates client retry logic. An all-or-nothing transactional boundary guarantees predictable consistency.

## Consequences

- Significantly faster ingestion for bulk tasks (migrations, conversational imports, batch connectors).
- Zero changes to existing single-item `POST /memory` behavior.
- **Benchmarking rule**: Per benchmark protocol, the LongMemEval benchmark harness will not be switched to the batch endpoint until a 100-question run confirms exact parity with `baseline-100` scores.
