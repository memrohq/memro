# Memro developer documentation

Memro gives AI agents long-term memory: they store what they learn, recall
it by meaning, keyword or time, and reason over how memories connect.

| | |
|---|---|
| [Quickstart](quickstart.md) | Connect your coding tools in one command, or call Memro from code in five minutes. |
| [Concepts](concepts.md) | Agents, memories, memory types, search modes and the memory engine. |
| [Agents that sign themselves up](agents.md) | How an AI agent creates its own Memro identity with no person involved, and what it can do before a person claims it. |
| [Authentication](authentication.md) | API keys, signed requests and the device login behind `npx memro-cli connect`. |
| [MCP server](mcp.md) | Use Memro from Claude Code, Codex, Cursor, VS Code and any MCP client. |
| [API reference](api-reference.md) | Every HTTP endpoint with request and response shapes. |
| SDKs | [Python](sdks/python.md) · [JavaScript / TypeScript](sdks/javascript.md) · [Go](sdks/go.md) |
| [Connectors](connectors.md) | Sync 50 sources, including Slack, GitHub, Linear, Sentry and Notion, into memory. |
| [Webhooks](webhooks.md) | Get a POST when an agent stores a memory. |
| [Limits and errors](limits-and-errors.md) | Rate limits, size limits, quotas and error handling. |
| [Self-hosting](self-hosting.md) | Run the server, MCP server and dashboard yourself. |
| [Decision records](decisions/README.md) | Why Memro is built the way it is. |
| [Memro vs Mem0 vs Zep](comparisons/memro-vs-mem0-vs-zep.md) | How Memro's design differs from other agent memory projects, and when to pick each. |

**Endpoints**

| | Memro Cloud | Self-hosted default |
|---|---|---|
| API | `https://api.memro.co` | `http://localhost:8081` |
| MCP (Streamable HTTP) | `https://mcp.memro.co/mcp` | `http://localhost:8080/mcp` |
| Dashboard | `https://app.memro.co` | `http://localhost:5173` |

Questions or problems: [open an issue](https://github.com/memrohq/memro/issues)
or email support@memro.co. Security reports: see [SECURITY.md](../SECURITY.md).
