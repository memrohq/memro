# Integrating Memro with Claude Code

Anthropic's [Claude Code](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview) CLI is an agentic coding environment that excels at codebase exploration and modification. By integrating Memro, Claude Code remembers architecture decisions, preferences, and operational rules across sessions without needing context injected every time.

## Quick Setup

Run the automated CLI connector from your project root:

```bash
npx memro-cli connect --project --client claude
```

This performs two actions:
1. Adds Memro to `./.mcp.json` so Claude Code registers tools on launch.
2. Injects instructions into `./CLAUDE.md` between `<!-- memro:start -->` and `<!-- memro:end -->` teaching Claude to recall context on task startup and save lasting facts.

## Manual Configuration

If configuring manually, create or edit the following files:

### 1. `.mcp.json` (Project MCP Config)
```json
{
  "mcpServers": {
    "memro": {
      "command": "npx",
      "args": ["-y", "memro-cli", "mcp"],
      "env": {
        "MEMRO_API_KEY": "memro_sk_your_api_key_here"
      }
    }
  }
}
```

### 2. `CLAUDE.md` (Guidance Block)
```markdown
<!-- memro:start -->
## Memro (long-term memory)

Memro is the long-term memory for this project (MCP server `memro`). Use it without being asked:
- At the start of a task, call `recall` with a short query about it and use what comes back.
- When a decision is made, a preference is stated, you are corrected, or you learn a lasting fact about this project, call `remember` with one short, self-contained sentence that names the project.
- Before answering about earlier work ("what did we decide about X?"), `recall` first.

Never store secrets (keys, passwords, tokens), one-off chatter, or anything this project's rules keep private. Don't announce routine saves.
<!-- memro:end -->
```

## Complete Verification Lifecycle

To verify persistent cross-session memory end-to-end:

1. **Launch Session 1**:
   ```bash
   claude
   ```
2. **Teach Claude a Fact**:
   > *"Remember that our primary database is PostgreSQL 16 with pgvector and our API server listens on port 8081."*
   Claude calls `remember` and confirms storage.
3. **Exit Session**:
   Type `/exit`.
4. **Launch Session 2**:
   ```bash
   claude
   ```
5. **Recall the Fact**:
   > *"What database and port does our API server use?"*
   Claude calls `recall` and responds with the accurate stored context.

See the complete runnable example at [`examples/claude-code/`](../../examples/claude-code/).

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
