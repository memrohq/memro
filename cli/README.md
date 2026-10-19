# memro

Connect your AI tools to [Memro](https://memro.co) memory with one command.

```bash
npx memro-cli connect
```

1. Shows a code and opens the Memro dashboard. Sign in and press **Connect**.
2. Receives a new API key (revocable in the dashboard → API keys) and stores
   it in `~/.memro/credentials.json` (readable only by you).
3. Finds the tools on this machine and adds the Memro MCP server to each:

| Tool | Config written |
|---|---|
| Claude Code | `claude mcp add --scope user` (or `~/.claude.json`) |
| Codex | `~/.codex/config.toml` |
| Cursor | `~/.cursor/mcp.json` |
| Antigravity | `~/.gemini/antigravity/mcp_config.json` |
| OpenCode | `~/.config/opencode/opencode.json` |
| Gemini CLI | `~/.gemini/settings.json` |
| VS Code | user `mcp.json` |
| Windsurf | `~/.codeium/windsurf/mcp_config.json` |
| Claude Desktop | `claude_desktop_config.json` (via `mcp-remote`) |

Existing settings and other servers are kept; the original file is saved
once as `<file>.memro-backup`. Restart the tool afterwards.

## Commands

```
npx memro-cli connect [--client cursor,codex] [--yes]
npx memro-cli status
npx memro-cli disconnect [--client ...]
```

`--staging`, `--local` or `--api <url>` target another Memro server
(self-hosted: `--api https://memro.example.com`).

## Any other MCP client

Streamable HTTP at `https://mcp.memro.co/mcp` with the header
`Authorization: Bearer <memro_sk_ key>`.
