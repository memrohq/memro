# memro-cli

Connect your AI tools to [Memro](https://memro.co) memory with one command.

```bash
npx memro-cli connect
```

1. Lists the supported tools with the ones on this machine already ticked.
   Use ↑/↓ to move, space to tick, `a` for all, Enter to confirm (Esc cancels
   without signing in).
2. Shows a code and opens the Memro dashboard. Sign in and press **Connect**.
3. Receives a new API key (revocable in the dashboard → API keys) and stores
   it in `~/.memro/credentials.json` (readable only by you).
4. Adds the Memro MCP server to each tool you picked:

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

`--client` picks tools without the list; `--yes` takes every detected tool
(both work in scripts, where there is no list to show).
`--staging`, `--local` or `--api <url>` target another Memro server
(self-hosted: `--api https://memro.example.com`).

## Any other MCP client

Streamable HTTP at `https://mcp.memro.co/mcp` with the header
`Authorization: Bearer <memro_sk_ key>`.
