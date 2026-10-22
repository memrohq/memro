# MCP server

Memro's MCP server lets any [Model Context Protocol](https://modelcontextprotocol.io)
client store and recall memories: Claude Code, Codex, Cursor, VS Code,
Windsurf, Gemini CLI, OpenCode, Antigravity, Claude Desktop and others.

| | Memro Cloud | Self-hosted |
|---|---|---|
| Streamable HTTP (recommended) | `https://mcp.memro.co/mcp` | `http://localhost:8080/mcp` |
| SSE (legacy clients) | `https://mcp.memro.co/sse/` | `http://localhost:8080/sse/` |

Every connection authenticates with an [API key](authentication.md#api-keys):
`Authorization: Bearer memro_sk_…`. Only the legacy SSE endpoint also accepts
`?token=memro_sk_…`, for clients that can't set headers; `/mcp` requires the
header, which keeps the key out of logs. A
connection always acts as the key's agent and can't reach any other agent's
memories. Requests without a valid key get `401`.

## Set up

`npx memro-cli connect` configures every tool it finds. To do it by hand:

**Claude Code**

```bash
claude mcp add --transport http --scope user memro https://mcp.memro.co/mcp \
  --header "Authorization: Bearer memro_sk_..."
```

**Codex** (`~/.codex/config.toml`)

```toml
[mcp_servers.memro]
url = "https://mcp.memro.co/mcp"
http_headers = { Authorization = "Bearer memro_sk_..." }
```

**Cursor** (`~/.cursor/mcp.json`), **Windsurf** (`~/.codeium/windsurf/mcp_config.json`, use `serverUrl`),
**Gemini CLI** (`~/.gemini/settings.json`, use `httpUrl`)

```json
{ "mcpServers": { "memro": { "url": "https://mcp.memro.co/mcp",
    "headers": { "Authorization": "Bearer memro_sk_..." } } } }
```

**VS Code** (user `mcp.json`)

```json
{ "servers": { "memro": { "type": "http", "url": "https://mcp.memro.co/mcp",
    "headers": { "Authorization": "Bearer memro_sk_..." } } } }
```

**Claude Desktop** has no remote-server headers, so it goes through
`mcp-remote`:

```json
{ "mcpServers": { "memro": { "command": "npx",
    "args": ["-y", "mcp-remote", "https://mcp.memro.co/mcp", "--header", "Authorization:${MEMRO_AUTH}"],
    "env": { "MEMRO_AUTH": "Bearer memro_sk_..." } } } }
```

Then ask the tool to remember something, or tell it in your project
instructions (`CLAUDE.md`, `AGENTS.md`, …):

> Use the memro tools: `recall` relevant context before starting a task and
> `remember` decisions, preferences and project facts when you learn them.

## Tools

**Memory**

| Tool | Arguments | |
|---|---|---|
| `remember` | `content`, `type?`, `visibility?`, `event_date?`, `session_id?`, `is_atomic?` | Store a memory |
| `remember_with_context` | `content`, `context_metadata?`, `type?`, `event_date?`, `session_id?` | Store with metadata (file, branch, function) and when it happened |
| `recall` | `query`, `limit?` (5), `type?` | Hybrid search |
| `recall_multi_session` | `query`, `limit?` (15) | Search across all sessions |
| `search_temporal` | `query`, `after_date?`, `before_date?`, `after_event_date?`, `before_event_date?`, `temporal_weight?`, `atomic_only?`, `limit?` | Search by time |
| `get_recent_memories` | `limit?`, `type?` | Newest first |
| `delete_memory` | `memory_id` | |
| `export_memories` | | Count of all memories |
| `get_temporal_stats` | | Totals by period and type |
| `generate_atomic_facts` | `text` | Split text into facts (not stored) |

**Memory engine**

| Tool | Arguments | |
|---|---|---|
| `mee_reason` | `query` | Answer from memory with the memories and graph path used |
| `mee_evolve` | | Consolidate episodes into semantic memories |
| `mee_resolve` | `memory_a_id`, `memory_b_id` | Pick the more trustworthy of two memories |
| `get_memory_chain` | `memory_id` | A memory and everything linked to it |
| `get_timeline` | `memory_id` | Audit history of a memory |
| `find_contradictions` | | How many conflicts exist and how many are unresolved |

**Coordinating several agents** on one codebase with atomic server-side leases
and non-blocking intents. Claims are shared with agents in the same account/space
and expire after the TTL (default 15 minutes, max 2 hours). A release or handoff
frees the resource for a new claim, and handoff summaries are passed to the next claimer.
Coordination records use dedicated tables rather than procedural memories, keeping
memory search and recall clean.

| Tool | Arguments | |
|---|---|---|
| `pre_action_check` | `resource`, `intent` | `allow`, `warn` (overlapping intent) or `block` (claimed) |
| `claim_task` | `resource`, `intent`, `ttl?` | Claim a file or task (shared in account/space, expires after TTL) |
| `heartbeat_task` | `resource`, `ttl?` | Extend an active lease before it expires |
| `release_task` | `resource`, `status?` | Release a claim (`completed` or `abandoned`) |
| `log_intent` | `resource`, `intent`, `ttl?` | Announce planned work (non-blocking) |
| `handoff_task` | `resource`, `summary`, `next_agent_id?` | Hand off and release with summary for next claimer |
| `get_active_work`, `get_active_context` | `resource?` | Current active claims and intents in scope |
| `get_system_status` | | Server version, agent and API health |

## Run it yourself

```bash
pip install -e sdk/python -e mcp            # from a clone of memrohq/memro
MEMRO_BASE_URL=http://localhost:8081 MCP_TRANSPORT=sse memro-mcp   # HTTP on :8080
MEMRO_API_KEY=memro_sk_... memro-mcp                                # stdio, one user
```

Over HTTP the server never uses its own environment credentials; every
connection brings its own key. In stdio mode it uses `MEMRO_API_KEY` (or
`MEMRO_AGENT_ID` + `MEMRO_PRIVATE_KEY`). See [mcp/README.md](../mcp/README.md)
for all settings.
