# 0018. Tenant and Agent Scoping Audit
Date: 2026-10-22 · Status: Accepted

## Context
Memro stores sensitive agent memories, documents, uploaded files, and connector credentials across multi-agent and multi-tenant environments. To guarantee strict isolation, every query touching `memories`, `files`, `sessions`, and `connector_connections` must be scoped by `agent_id` (and by `tenant_id` where applicable). In particular, lookup queries using `WHERE id = ANY($1)` must only process IDs that originate from an already-scoped query.

## Audit Findings

### 1. `memories` SQL Statements
- `store.go:InsertMemory`: Scoped by `agent_id` and `tenant_id`.
- `store.go:CountMemories`: Scoped by `agent_id = $1`.
- `store.go:ListMemories`: Scoped by `agent_id = $1 AND tenant_id = $2` (plus optional filters).
- `store.go:AllMemories`: Scoped by `agent_id = $1 AND tenant_id = $2`.
- `store.go:GetMemory`: Scoped by `id = $1 AND agent_id = $2 AND tenant_id = $3`.
- `store.go:DeleteMemory`: Scoped by `id = $1 AND agent_id = $2 AND tenant_id = $3`.
- `store.go:UpsertEmbedding`: Scoped by `agent_id`, `tenant_id`, and `memory_id`.
- `store.go:MemoriesNeedingEmbedding`: Background embedding maintenance; joins `memory_embeddings` to find backfill candidates.
- `engine.go:Search` (`semanticIDs` & `keywordHits`): Both strictly scoped by `agent_id = $1 AND tenant_id = $2`.
- `engine.go:diversify`: `WHERE id = ANY($1)` receives `order` derived directly from `semanticIDs` and `keywordHits`.
- `engine.go:loadResults`: `UPDATE memories ... WHERE id = ANY($1)` and `SELECT ... WHERE id = ANY($1)` receive IDs from `Search` after scoping.
- `engine.go:TemporalSearch`: Strictly scoped by `m.agent_id = $1 AND m.tenant_id = $2`.
- `engine.go:ResolveConflict`: Strictly scoped by `WHERE id IN ($1, $2) AND agent_id = $3 AND tenant_id = $4`.
- `engine.go:Evolve`: Episodic query and profile query are both scoped by `agent_id = $1 AND tenant_id = $2`. `UPDATE memories ... WHERE id = ANY($1)` receives IDs directly from the episodic query.
- `engine.go:Timeline`: Scoped by `agent_id` and `tenant_id` on memory/audit ownership check. The subsequent audit_log query was hardened with `AND agent_id = $3`.
- `engine.go:MemoryChain`: Hardened by adding `AND m.agent_id = $3` to the joined memory query to prevent leaking related memories across agents in the same tenant.
- `engine.go:TemporalStats`: Strictly scoped by `agent_id = $1 AND tenant_id = $2`.
- `connectors/ingest.go:ingestItem`: `SELECT COUNT(*) WHERE agent_id = $1`; `DELETE FROM memories WHERE id = ANY($1) AND connection_id = $2` receives `remove` from `itemMemories(c.ID, it.Key)`.
- `connectors/manage.go:List`: Scoped by `agent_id = $1`.
- `connectors/manage.go:Disconnect`: Scoped by `connection_id = $1 AND agent_id = $2`.
- `connectors/links.go`: Cross-reference query and update hardened with `tenant_id` and `agent_id` scoping.

### 2. `files` SQL Statements
- `store.go:TotalFileSize`: Scoped by `agent_id = $1`.
- `store.go:InsertFile`: Scoped by `agent_id` and `tenant_id`.
- `store.go:DeleteFileIfOrphan`: Scoped by `f.id = $1 AND f.agent_id = $2`.
- `store.go:DeleteFilesForAgent`: Scoped by `agent_id = $1`.
- `store.go:GetFile`: Looked up by `id = $1`. Added `GetFileForAgent(ctx, id, agentID, tenantID)` scoped in SQL for agent operations; public `GET /assets/{id}` capability endpoint retains capability-based access.

### 3. `sessions` & `connector_connections` SQL Statements
- `sessions`: Token authentication table hashed with cryptographically secure single-use session tokens (`token_hash = $1`).
- `connector_connections`: Every agent-facing management endpoint (`Sources`, `UpdateSources`, `SyncNow`, `Disconnect`) calls `m.owned(ctx, agentID, id)` which enforces `WHERE c.id = $1 AND c.agent_id = $2`.

### 4. Self-Signup Users and Shared Visibility Observations
- **Shared Tenant**: Self-signup users and unclaimed agents are assigned `domain.DefaultTenantID` (`uuid.Nil`). Multiple unrelated users share this default tenant ID, so `agent_id` is the primary isolation boundary.
- **Shared Visibility**: While `domain.Shared` (`"shared"`) visibility exists in data models, all search and read endpoints strictly enforce `m.agent_id = $1`, meaning `'shared'` memories are currently never surfaced to other agents in the same tenant.
- **Proposed Future Architecture (Non-Breaking, Not Implemented in this PR)**:
  1. Assign a unique tenant UUID to each self-signup account during registration rather than sharing `uuid.Nil`.
  2. Implement an explicit Spaces / Team Sharing read model with an authorized membership join table before enabling `'shared'` visibility reads.

## Consequences
- Added defense-in-depth scoping in `MemoryChain`, `Timeline`, `links.go`, and `handlers_files.go`.
- Added automated cross-agent isolation integration tests covering Search, Get, Delete, and Connectors in `server/internal/api/cross_agent_isolation_integration_test.go`.
