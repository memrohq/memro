# Memro MCP Server

Verified memory infrastructure for AI agents. Gives any Model Context Protocol (MCP) compatible AI tool (Claude Code, Cursor, Codex, Zed, Claude Desktop) persistent memory across sessions.

## Install

```bash
# PyPI (once published via CI):
pip install memro-mcp

# Or install from source:
pip install .
```

## Quick Example (MCP Config)

Add to your MCP client config (e.g., `~/.claude/claude_desktop_config.json` or `.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "memro": {
      "command": "memro-mcp",
      "env": {
        "MEMRO_API_KEY": "memro_sk_..."
      }
    }
  }
}
```

Or connect with a single command via the CLI:
```bash
npx memro-cli connect
```

## Available MCP Tools

Once connected, agents have direct access to:

| Tool | Description |
|------|-------------|
| `remember` | Store a new memory (`semantic`, `episodic`, `procedural`, or `profile`) |
| `recall` | Hybrid semantic + keyword retrieval across stored memories |
| `get_recent_memories` | Retrieve recent memories chronologically |
| `delete_memory` | Delete a specific memory by ID |
| `export_memories` | Export agent memory state |
| `search_temporal` | Query memories with explicit time-range weighting |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MEMRO_API_KEY` | — | Memro API key (`memro_sk_...`) |
| `MEMRO_BASE_URL` | `http://localhost:8081` | Memro backend URL (default for local self-hosting) |
| `MEMRO_AGENT_ID` | auto | Sovereign agent Ed25519 public key |
| `MEMRO_PRIVATE_KEY` | auto | Sovereign agent Ed25519 private key |
| `MCP_TRANSPORT` | `stdio` | `stdio` for local CLI tools, or `sse` for HTTP SSE transport |
| `MCP_LOG_LEVEL` | `INFO` | Logging level (`DEBUG`, `INFO`, `WARNING`, `ERROR`) |

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
