# 0019. Server-Side Multi-Agent Coordination
Date: 2026-10-22 · Status: Accepted

## Context
Previously, Memro coordinated tasks across tools by saving procedural memories (`[CLAIMED] ...`, `[RELEASED] ...`, `[INTENT] ...`, `[HANDOFF] ...`) in the calling agent's private memory, with the client MCP server scanning the calling agent's last 100 procedural memories in the current session.

This approach had severe architectural shortcomings:
1. **No multi-agent visibility**: Multiple agents (and teammates) on the same user account or in the same tenant space could not see each other's claims. Claims were isolated to tools sharing a single agent identity.
2. **Race conditions**: Check-then-claim was performed on the client without database-level atomicity, allowing two agents to claim the same resource concurrently.
3. **No expiry**: If an agent process crashed or stalled, its claim remained indefinitely until manually released.
4. **Window limits**: Claims beyond the 100 most recent procedural memories became invisible.
5. **Memory pollution**: Coordination chatter polluted semantic, keyword, and temporal memory search and recall.

## Decision

### 1. Scope Rule: Who Sees Whose Claims
We reuse the identity and account model (ADR 0012, `store` package). Each coordination lease and intent is keyed by a canonical string `scope_key`:
- **Tenant / Space**: If the agent belongs to a dedicated tenant (`agent.TenantID != uuid.Nil`), the scope is `tenant:<tenant_id>`. All agents in that tenant or space share leases.
- **Owning User**: Otherwise, if the agent has an owning user (`agent.UserID != nil && *agent.UserID != uuid.Nil`), the scope is `user:<user_id>`. All agents owned by that human user (e.g. self-signup accounts with multiple agents) share leases.
- **Unclaimed / Standalone Agent**: Otherwise (self-signed-up unclaimed agents without an owner), the scope is `agent:<agent_id>`. The agent coordinates only with itself.

Claims and intents never leak across scopes. Isolation is strictly enforced in SQL queries by prefixing every predicate with `WHERE scope_key = $1`.

### 2. Dedicated Storage & Indexing
We add dedicated relational tables via migration `0030_coordination_leases.sql`:
- **`coordination_leases`**:
  - Columns: `id`, `scope_key`, `resource` (trimmed; values that look like file paths, containing `/` but not `://`, are cleaned with `path.Clean`, so URLs and plain names are kept as given), `holder_agent_id`, `holder_session`, `intent`, `created_at`, `heartbeat_at`, `expires_at`, `released_at`, `release_status` (`completed` | `abandoned` | `expired` | `handoff`), `handoff_summary`, `handoff_to`.
  - Partial Unique Index:
    ```sql
    CREATE UNIQUE INDEX idx_coordination_leases_active
    ON coordination_leases (scope_key, resource)
    WHERE released_at IS NULL;
    ```
    *Note*: Because Postgres partial unique index predicates cannot use non-immutable functions like `now()`, the partial index enforces uniqueness on `released_at IS NULL`. In the claiming transaction, any expired leases (`released_at IS NULL AND expires_at <= NOW()`) are reaped and marked with `released_at = NOW(), release_status = 'expired'`, safely clearing the partial index constraint.
- **`coordination_intents`**:
  - Columns: `id`, `scope_key`, `resource`, `agent_id`, `intent`, `created_at`, `expires_at`.
  - Non-blocking advisory declarations to detect potential overlaps without locking.

### 3. Lease Semantics
- **Atomic Claim (`POST /coordination/claim`)**:
  In a single transaction:
  1. Reaps expired leases on `(scope_key, resource)`.
  2. Queries active lease with `FOR UPDATE`. If held by the same agent, it idempotently extends the lease TTL and updates the intent.
  3. If held by another holder, returns `granted: false` with holder, holder session, intent, and expiry details.
  4. If no active lease exists, inspects the newest previous handoff on this resource to populate `handoff_summary`.
  5. Inserts the new lease using `INSERT ... ON CONFLICT (scope_key, resource) WHERE released_at IS NULL DO NOTHING RETURNING expires_at`. If a concurrent transaction won the insertion race, it gracefully falls back to returning `granted: false` with the winner's details.
  - Returns: `{ "granted": true, "lease_id": "...", "expires_at": "...", "handoff_summary": "..." }` or `{ "granted": false, "holder": "...", "intent": "...", "expires_at": "..." }`.
  - Default TTL: 15 minutes. Clamped between 1 second and 2 hours (configurable).
- **Heartbeat (`POST /coordination/heartbeat`)**:
  Holder only (or account admin). Extends `expires_at = NOW() + ttl`.
- **Release (`POST /coordination/release`)**:
  Holder only (or account admin). Sets `released_at = NOW()` and `release_status` (`completed` | `abandoned`).
- **Handoff (`POST /coordination/handoff`)**:
  Holder only. Sets `released_at = NOW(), release_status = 'handoff'`, saving `handoff_summary` and optional `handoff_to` (`next_agent_id`). When the next claimer claims the resource, `handoff_summary` is returned in their claim response. With no active lease (work that only had an intent), the handoff note is still recorded, as an already-released lease row; it fails with `409` only when another holder has the resource.
- **Check (`POST /coordination/check`)**:
  Non-blocking check returning:
  - `allow`: Unclaimed, expired, or currently held by the calling agent.
  - `warn`: Active intents on this resource declared by other agents in the same scope.
  - `block`: Active lease held by another agent in the same scope.
- **Active (`GET /coordination/active`)**:
  Returns all active leases and non-expired intents in the scope, optionally filtered by resource prefix.

### 4. Legacy Data Exclusion
Existing `[CLAIMED]` procedural memories in the database remain intact, but all search queries (`semanticIDs`, `keywordHits`, `TemporalSearch`) exclude memories where `metadata->>'action' IN ('claim', 'release', 'intent', 'handoff')`. This completely prevents coordination logs from polluting memory recall and synthesis.

### 5. Client Implementations
- **MCP Server (`memro_mcp`)**: Deleted the in-memory `CoordinationEngine`. Tool handlers (`claim_task`, `release_task`, `handoff_task`, `pre_action_check`, `log_intent`, `get_active_work`, `get_active_context`) now route directly to the new API endpoints via the Agent SDK, and a new `heartbeat_task` tool was added. Tool descriptions updated to explicitly state that claims are shared across agents in the same account/space and expire after the TTL.
- **SDKs (Go, Python, TypeScript)**: Added `Claim`, `Heartbeat`, `Release`, `Handoff`, `Check`, `Active`, and `LogIntent` methods. Python SDK provides a `with agent.claim("resource", intent=...):` context manager with automatic background heartbeating and release on exit.

### 6. Amendment (2026-10-23): telling tools apart, and folders
Found in production on the first two-tool test (Claude Code and Antigravity on one account):
- **Every tool was the same holder.** The hosted MCP runs a stateless Streamable HTTP transport, so it never issues an `Mcp-Session-Id`. Every tool connected with one API key landed in session `default` and could claim straight over another tool. The session is now resolved as: `?session_id=` on the MCP URL, then `Mcp-Session-Id`, then the client's User-Agent product (`ua:claude-code`; generic HTTP libraries are ignored), then `default`. `memro connect` writes `?session_id=<tool id>` into every tool's URL.
- **A folder claim didn't cover its files.** Leases matched the exact resource string, so a claim on `memro/bench` didn't block `memro/bench/run.py`. Claims and checks now treat path resources as a tree: a lease on a folder blocks claims and checks on paths inside it, and a lease on a path blocks claiming a folder that contains it. URLs and plain names still match exactly. Claims within one scope take a transaction-level advisory lock, so two overlapping claims can't both pass. A denied claim returns `holder_resource` when the conflicting lease is on a different path.
- **Messages name the tool.** Tools on one account share an agent ID, so MCP block messages name the holder's session (the tool) instead.

### 7. Amendment (2026-11-16): claims made by hooks, not by the model
Agents often skipped `pre_action_check` and `claim_task`, so a lease existed only when the model remembered it. And every Claude Code window shared the holder `claude-code` (the MCP URL's `?session_id=`), so two windows could edit one file. `memro connect` now installs a Claude Code hook (`cli/src/hook.js`, copied to `~/.memro/hooks/`) that:
- claims the file before every Edit, Write, MultiEdit and NotebookEdit, as holder `claude-code:<first 8 characters of the Claude Code session id>`, and denies the edit when another holder has it. Re-claiming extends the lease, so editing is the heartbeat.
- names the file `<origin repo name>/<path in repo>`, so every clone and worktree of a repo claims the same resource, matching the `memro/cli` style agents already use.
- releases the window's leases on `SessionEnd`.
- lets the edit through on any error (no key, offline, timeout of 3 s): Memro being down must not stop work.

The hooks also give agents status and messages, without new tables:
- **Status** is a lease on `agents/<session>` (TTL 2 hours, released on `SessionEnd`) whose intent is `<state> · <folder>`: `working` on `UserPromptSubmit`, `waiting for you` on `Notification`, `idle` on `Stop`. Re-claiming updates the intent. `memro agents` lists the status leases and the file leases by session.
- **Messages** are intents on `inbox/<session>` (TTL 2 hours), sent with `memro send`. The hook reads the inbox on `UserPromptSubmit` (added as context) and on `Stop` (returned as a block, which wakes the agent to act on it), remembering seen ids in `~/.memro/hooks/`. A stop that a message already restarted (`stop_hook_active`) doesn't read again, so agents can't wake each other in a loop. Messages are labelled as coming from other agents, not the user. An agent sitting idle at its prompt runs no hooks; outside herdr it reads the message when it next gets a prompt.

Refinements from a live test with three Claude Code agents in [herdr](https://herdr.dev) panes:
- **Files are held for one turn.** A `Stop` with no new messages releases the window's file leases (the status lease stays). The agent never knew it held them, so it couldn't honour "please release"; now an idle agent blocks nobody, and a refused agent is told the file frees up when the holder's turn ends.
- **Worktrees warn instead of deny.** The lease intent ends with `[<checkout folder>@<host>]`. When the holder is in a different checkout, the edit goes ahead with `additionalContext` warning of a merge conflict; separate branches only meet at merge time, and worktrees are how herdr and our own rules keep agents apart.
- **herdr wake.** Inside herdr (`HERDR_ENV=1`) the status intent ends with `[herdr <session> <pane>@<host>]`. `memro send` to an idle agent on the same machine runs `herdr agent prompt <pane>` with a short nudge, which fires the agent's `UserPromptSubmit` hook and shows it the message. Across machines it waits for the next prompt.
- The location tags live in free-text intents until leases get a metadata field.

**Codex** uses the same script (`~/.memro/hooks/memro-hook.mjs codex`) from `~/.codex/hooks.json`, since Codex hooks take the same input and output as Claude Code's. Differences: every file edit is `apply_patch`, so the hook claims each path in the patch's `*** Add/Update/Delete File:` and `*** Move to:` lines; there is no Notification event; SessionEnd may run for at most 3 seconds; and Codex skips a new or changed hook until the user trusts it in `/hooks`. Holders are `codex:<session>`.

A lease held by the shared MCP session `claude-code` doesn't block the hook, because it may be the same window. The reverse remains: a window's MCP `claim_task` on a folder is refused if the same window's hook already holds a file inside it. Both go away once the MCP connection carries a per-window session.

## Consequences
- Multi-agent teams sharing a tenant space or user account work safely without collision or race conditions.
- Abandoned leases from crashed agents or sessions automatically expire after the TTL without blocking resources.
- Search and recall quality is preserved without coordination memory noise.
- Prometheus counters `memro_coordination_claims_total`, `memro_coordination_expirations_total`, and `memro_coordination_handoffs_total` provide real-time observability into multi-agent coordination.

## Holders are agent + session (amendment)
The tools on one account (Claude Code, Cursor, Codex connected with `memro-cli connect`) share one agent and one API key. If a lease belonged to the agent alone, one tool's claim would not block the others, the very case coordination exists for. So a lease belongs to **agent + session**:
- The session is the request's `session_id`, else the `X-Session-Id` header, else the credential (`key:<API key id>`, `sig:<signing key>`), else `dashboard` for dashboard sessions.
- The hosted MCP server sends each client connection's `Mcp-Session-Id` (as `mcp:<id>`), unless the connection URL sets `?session_id=`.
- Re-claim, `check` → `allow`, and heartbeat/release/handoff **by resource** match agent and session. By `lease_id` (an unguessable ID only the holder received) the agent must match, so a restarted process can still release its lease.
- Intents carry their session too; `check` warns about intents from every other agent or session.

## Retention
Released leases and expired intents are purged after 30 days by an hourly job (`RunCoordinationPurge`). Active leases are never purged.
