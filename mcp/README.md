# memro MCP Server

Gives any MCP-compatible AI (Claude Desktop, Cursor, Zed, etc.) persistent memory via the memro protocol.

## Install (Local)

hhhhhhhhggg

pip install memro-mcp

## Install (Docker)
ojkjdnksdnodk;lka;dljladnaljdnalkdnaldnadnkadnjkadnlkdjwk
docker build -t memro-mcp .
# Local Mode (Default)
docker run -e MEMRO_BASE_URL=http://your-backend-url memro-mcp
# Cloud Mode (SSE)
docker run -p 8080:8080 -e MCP_TRANSPORT=sse -e MEMRO_BASE_URL=http://your-backend-url memro-mcp
```

## Managed Mode (SaaS)

For multi-tenant or SaaS deployments where you want a single MCP server instance to serve multiple users:

1. Start the server with `MCP_TRANSPORT=sse`.
2. The agent must call the `initialize_session` tool first to register its `agent_id` and `private_key`.
3. All subsequent calls in that session will use those credentials.

See [setup_managed.md](setup_managed.md) for a detailed configuration guide.

## Setup with Claude Desktop

Add to `~/.claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "memro": {
      "command": "memro-mcp",
      "env": {
        "MEMRO_AGENT_ID": "your_agent_id_here",
        "MEMRO_PRIVATE_KEY": "your_private_key_here",
        "MEMRO_BASE_URL": "http://localhost:8081"
      }
    }
  }
}
```

> First run without `MEMRO_AGENT_ID` set will auto-create a new agent and print the credentials to stderr.

## Available Tools

Once connected, Claude (or any MCP client) can use:

| Tool | What it does |
|------|-------------|
| `remember` | Store a new memory (episodic, semantic, procedural, or profile) |
| `recall` | Semantic search across all memories |
| `get_recent_memories` | Get most recent memories chronologically |
| `delete_memory` | Delete a specific memory by ID |
| `export_memories` | Export a summary of all stored memories |

## Example Usage

Once configured, Claude will automatically use memory:

```
You: Remember that I prefer TypeScript over JavaScript
Claude: [calls remember("User prefers TypeScript over JavaScript", type="profile")]
        Got it, I'll remember that.

You: What's my preferred language?
Claude: [calls recall("preferred programming language")]
        Based on what I remember, you prefer TypeScript over JavaScript.
```

## Self-host the memro backend

```bash
cd memrohq
docker-compose up -d

# Backend runs at http://localhost:8081
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `MEMRO_AGENT_ID` | No* | auto-created | Your agent's public key |
| `MEMRO_PRIVATE_KEY` | No* | auto-created | Your agent's private key |
| `MEMRO_BASE_URL` | No | `http://localhost:8081` | memro backend URL |

*First run will auto-create and print credentials. Save them for future runs.
