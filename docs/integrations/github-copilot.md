Integrating Memro with GitHub Copilot
=====================================

GitHub Copilot in VS Code is an AI pair programmer that can leverage Memro's long-term memory to maintain persistent context across coding sessions. By integrating Memro, Copilot can recall architecture decisions, preferences, and operational rules without needing them re-injected every time.

Quick Setup
-----------

Run the automated CLI connector from your project root:

```
npx memro-cli connect
```

This performs three actions for VS Code (Copilot):

1. Writes an MCP config file that VS Code uses to register the Memro memory server.
2. Writes a user-level instruction file at `~/.copilot/instructions/memro.instructions.md`.
3. Writes a project-level instruction file at `.github/instructions/memro.instructions.md`.

Manual Configuration
--------------------

If configuring manually, create or edit the following files:

### 1. MCP Config (VS Code)

`memro-cli connect` writes the MCP configuration automatically. This registers the Memro MCP server in VS Code's configuration, allowing GitHub Copilot to use Memro's memory tools.

### 2. User-level Instruction (`~/.copilot/instructions/memro.instructions.md`)

This file provides Memro instructions for all projects:

```markdown
## Memro (long-term memory)

You have access to a long-term memory system (MCP server `memro`) that stores facts, decisions, and preferences across sessions.

At the start of a task, call `recall` with a short query about the project and use what comes back.
When a decision is made, a preference is stated, you are corrected, or you learn a lasting fact, call `remember` with one short, self-contained sentence that names the project.
Before answering about earlier work ("what did we decide about X?"), `recall` first.

Never store secrets (keys, passwords, tokens), one-off chatter, or anything the project's rules keep private.
```

The `applyTo` setting is set to `"**"` so these instructions apply to every file and folder in the workspace.

### 3. Project-level Instruction (`.github/instructions/memro.instructions.md`)

This file provides Memro instructions scoped to the current repository:

```markdown
## Memro (long-term memory)

You have access to a long-term memory system (MCP server `memro`) that stores facts, decisions, and preferences across sessions.

At the start of a task, call `recall` with a short query about the project and use what comes back.
When a decision is made, a preference is stated, you are corrected, or you learn a lasting fact, call `remember` with one short, self-contained sentence that names the project.
Before answering about earlier work ("what did we decide about X?"), `recall` first.

Never store secrets (keys, passwords, tokens), one-off chatter, or anything the project's rules keep private.
```

The `applyTo` setting is `"**"` so these instructions apply to every file in the project.

VS Code Window Reload
---------------------

After Memro writes or updates instruction files, you must **reload the VS Code window** for Copilot to pick up the changes:

1. Press `Cmd+Shift+P` (macOS) or `Ctrl+Shift+P` (Windows/Linux)
2. Type "Reload Window" and select it

This ensures Copilot reads the latest instruction files.

Complete Verification Lifecycle
-------------------------------

To verify persistent cross-session memory end-to-end:

1. **Reload VS Code** after running `memro-cli connect` so Copilot picks up the new instructions.
2. **Teach Copilot a Fact** in one session:
   > "Remember that our primary database is PostgreSQL 16 with pgvector and our API server listens on port 8081."
   > Copilot calls `remember` and confirms storage.
3. **Close the VS Code window.**
4. **Reopen VS Code** and start a new session.
5. **Recall the Fact**:
   > "What database and port does our API server use?"
   > Copilot calls `recall` and responds with the accurate stored context.

See the complete runnable example at [`examples/github-copilot/`](/memrohq/memro/blob/main/examples/github-copilot).

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: https://memro.co
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
  
