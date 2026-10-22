# Integrating Memro with Cursor

[Cursor](https://cursor.com) is an AI-powered code editor built on VS Code. Integrating Memro gives Cursor Composer and Chat persistent memory of your conventions, tech stack nuances, and debugging decisions across edits.

## Quick Setup

Run the automated CLI connector from your project root:

```bash
npx memro-cli connect --project --client cursor
```

This creates or updates `./.cursor/mcp.json`.

## Manual Configuration

Create or edit `.cursor/mcp.json`:

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

Ensure in Cursor Settings → Features → MCP that `memro` appears enabled with the green dot.

## Complete Verification Lifecycle

1. **Open Cursor Project**:
   Open the folder with `.cursor/mcp.json`.
2. **Teach Cursor a Decision**:
   In Composer (`Cmd+I` / `Ctrl+I`) or Chat (`Cmd+L` / `Ctrl+L`):
   > *"Remember that this project uses Go 1.22 with pgvector 16, and our smoke test script is at ./scripts/smoke_packages.sh."*
   Cursor invokes `remember` and acknowledges storage.
3. **Restart / Start Fresh Thread**:
   Open a new Composer conversation (`Cmd+N`).
4. **Recall the Decision**:
   > *"What Go version and smoke test script do we use for this project?"*
   Cursor invokes `recall` and responds with the accurate facts.

See the complete runnable example at [`examples/cursor/`](../../examples/cursor/).

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
