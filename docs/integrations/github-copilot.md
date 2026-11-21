# Integrating Memro with GitHub Copilot

GitHub Copilot in VS Code can use Memro's long-term memory to keep context across coding sessions. With Memro connected, Copilot recalls architecture decisions, preferences and project rules without them being repeated every time.

## Quick Setup

Run the automated CLI connector from your project root:

```bash
npx memro-cli connect --project --client vscode
```

This configures:
1. `./.vscode/mcp.json` with the Memro MCP server.
2. `./.github/instructions/memro.instructions.md` with guidelines directing Copilot to recall memory before starting a task and to store lasting decisions.

Without `--project`, it writes VS Code's user-level `mcp.json` (on macOS, `~/Library/Application Support/Code/User/mcp.json`) and `~/.copilot/instructions/memro.instructions.md`, so Memro applies to every workspace.

## Manual Configuration

### 1. `.vscode/mcp.json`

```json
{
  "servers": {
    "memro": {
      "type": "http",
      "url": "https://mcp.memro.co/mcp",
      "headers": {
        "Authorization": "Bearer memro_sk_your_api_key_here"
      }
    }
  }
}
```

### 2. `.github/instructions/memro.instructions.md`

Copilot Chat reads `*.instructions.md` files from `.github/instructions/` (project) and `~/.copilot/instructions/` (user). The frontmatter's `applyTo: "**"` applies the instructions to every file:

```markdown
---
description: Memro long-term memory
applyTo: "**"
---
<!-- memro:start -->
## Memro (long-term memory)

Memro is the long-term memory for this project (MCP server `memro`). Use it without being asked:
- At the start of a task, call `recall` with a short query about it and use what comes back.
- Call `remember` with one short, self-contained sentence when something will matter in a later session.
<!-- memro:end -->
```

`memro-cli connect` keeps the full guidance between the `memro:start` / `memro:end` markers up to date (the text is in [`cli/src/instructions.js`](../../cli/src/instructions.js)), and `memro-cli disconnect` removes it.

### 3. Reload VS Code

VS Code reads the MCP config and instruction files when a window opens. After `connect` or a manual edit, open the Command Palette (`Cmd+Shift+P` / `Ctrl+Shift+P`) and run **Developer: Reload Window**.

## Complete Verification Lifecycle

1. **Reload VS Code** after running `memro-cli connect`.
2. **Teach Copilot a Fact**:
   In Copilot Chat (agent mode):
   > *"Remember that our primary database is PostgreSQL 16 with pgvector and our API server listens on port 8081."*
   Copilot invokes `remember` and acknowledges storage.
3. **Start Fresh**:
   Close the window, reopen the project and start a new chat.
4. **Recall the Fact**:
   > *"What database and port does our API server use?"*
   Copilot invokes `recall` and responds with the stored facts.

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
