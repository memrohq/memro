# memro-cli

Connect your AI tools to [Memro](https://memro.co) memory with one command.

```bash
# Global configuration (default)
npx memro-cli connect

# Project-level configuration (current directory)
npx memro-cli connect --project
```

1. Lists the supported tools with the ones on this machine already ticked.
   Use ↑/↓ to move, space to tick, `a` for all, Enter to confirm (Esc cancels
   without signing in).
2. Shows a code and opens the Memro dashboard. Sign in and press **Connect**.
3. Receives a new API key (revocable in the dashboard → API keys) and stores
   it in `~/.memro/credentials.json` (readable only by you). Reuses existing credentials
   by default, or accepts `--key <key>` for project-isolated credentials.
4. Adds the Memro MCP server to each tool you picked:

| Tool | Global (`--scope user`) | Project (`--project`) |
|---|---|---|
| Claude Code | `claude mcp add --scope user` (or `~/.claude.json`) | `claude mcp add --scope project` (or `./.mcp.json`) |
| Cursor | `~/.cursor/mcp.json` | `./.cursor/mcp.json` |
| Codex | `~/.codex/config.toml` | `./.codex/config.toml` |
| VS Code | User `mcp.json` | `./.vscode/mcp.json` |
| Antigravity | `~/.gemini/config/mcp_config.json` | `./.agents/mcp_config.json` |
| OpenCode | `~/.config/opencode/opencode.json` | `./opencode.json` |
| Gemini CLI | `~/.gemini/settings.json` | `./.gemini/settings.json` |
| Windsurf | `~/.codeium/windsurf/mcp_config.json` | — (Windsurf has no per-project MCP config) |
| Claude Desktop | `claude_desktop_config.json` (via `mcp-remote`) | *(Global only)* |

5. Teaches each tool to use Memro without being asked: recall at the start of
   a task, remember preferences, decisions and lasting facts, never store
   secrets. The Memro server sends this guidance to every client on connect;
   a managed note is also added between `<!-- memro:start -->` and `<!-- memro:end -->`:

   | Tool | Global (`--scope user`) | Project (`--project`) |
   |---|---|---|
   | Claude Code | `~/.claude/CLAUDE.md` | `./CLAUDE.md` |
   | Codex | `~/.codex/AGENTS.md` | `./AGENTS.md` |
   | OpenCode | `~/.config/opencode/AGENTS.md` | `./AGENTS.md` |
   | Gemini CLI | `~/.gemini/GEMINI.md` | `./GEMINI.md` |
   | Antigravity | `~/.gemini/config/rules/memro.md` (always on) | `./.agents/rules/memro.md` (always on) |
   | Windsurf | `~/.codeium/windsurf/memories/global_rules.md` | — |

   Untick "Teach these tools…" in the list, or pass `--no-instructions`, to
   skip it. `disconnect` removes the note and leaves the rest of the file.

Existing settings and other servers are kept; the original file is saved
once as `<file>.memro-backup`. Restart the tool afterwards.

---

## Configuration Scope vs. Memory Identity

> **Important**: Using `--project` changes where the MCP integration and instructions are configured (in the current directory rather than your user home folder). However, **memories are tied to your Agent ID and API key**, not the project directory.
> 
> * If two projects use the same API key, they share the same agent memory.
> * For true isolation between distinct client projects, supply a separate API key created for a distinct agent:
>   ```bash
>   npx memro-cli connect --project --key memro_sk_client_specific_key
>   ```

---

## Commands

```bash
# Connect globally (default)
npx memro-cli connect [--client cursor,claude] [--yes]

# Connect for the current project
npx memro-cli connect --project [--client cursor,claude] [--yes]

# Explicit scope
npx memro-cli connect --scope user
npx memro-cli connect --scope project

# Status and disconnect
npx memro-cli status [--project]
npx memro-cli disconnect [--project] [--client ...]
```

### Options

* `--project`: Configure the current working directory instead of global user configs.
* `--scope <user|project>`: Explicitly select configuration scope (`user` default).
* `--client <ids>`: Only configure these tools, comma-separated (e.g. `--client claude,cursor`).
* `--key <key>`: Use a specific API key (`memro_sk_...`) instead of default credentials.
* `-y, --yes`: Configure every detected tool without prompting.
* `--no-instructions`: Do not add Memro guidance to instruction files (`CLAUDE.md`, `AGENTS.md`, etc.).
* `--staging`, `--local` or `--api <url>`: Target another Memro server (e.g. self-hosted: `--api https://memro.example.com`).

---

## Any other MCP client

Streamable HTTP at `https://mcp.memro.co/mcp` with the header
`Authorization: Bearer <memro_sk_ key>`.

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
