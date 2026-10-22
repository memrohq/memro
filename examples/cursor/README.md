# Cursor Integration with Memro

This example demonstrates integrating [Cursor](https://cursor.com) with Memro to give Cursor Composer and Chat persistent memory across editing sessions.

## Prerequisites

- Cursor IDE installed
- A Memro API key (get one free at [memro.co](https://memro.co) or via self-hosting)

## Setup (One Command)

From your project directory:

```bash
npx memro-cli connect --project --client cursor
```

This automatically configures `./.cursor/mcp.json` with the Memro MCP server.

---

### Manual Configuration

Create or update `.cursor/mcp.json` in your workspace root:

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

If you prefer using the Python MCP server:
```json
{
  "mcpServers": {
    "memro": {
      "command": "memro-mcp",
      "env": {
        "MEMRO_API_KEY": "memro_sk_your_api_key_here"
      }
    }
  }
}
```

---

## Verification Lifecycle

Follow this 5-step test to verify persistent cross-session memory in Cursor:

### Step 1: Open Project in Cursor
Open the project folder containing `.cursor/mcp.json`. Verify in Cursor Settings → Features → MCP that `memro` is listed with a green status indicator and tools (`remember`, `recall`, etc.) are detected.

### Step 2: Teach Cursor a Project Fact
Open Cursor Composer (`Cmd+I` / `Ctrl+I`) or Chat (`Cmd+L` / `Ctrl+L`):
> *"Remember that this project uses Go 1.22 with pgvector 16, and our smoke test script is at ./scripts/smoke_packages.sh."*

Cursor invokes the `remember` tool:
```json
{
  "content": "This project uses Go 1.22 with pgvector 16, and our smoke test script is at ./scripts/smoke_packages.sh.",
  "type": "procedural"
}
```
Cursor confirms the memory was saved.

### Step 3: Clear Session / Restart Cursor
Press `Cmd+N` / `Ctrl+N` to start a completely new Composer conversation, or restart Cursor.

### Step 4: Ask Cursor to Recall
In the brand-new session:
> *"What Go version and smoke test script do we use for this project?"*

Cursor calls `recall`:
```json
{
  "query": "Go version smoke test script"
}
```

### Step 5: Verify Retrieval
Cursor answers accurately using the retrieved context from Memro:
> *"This project uses Go 1.22 with pgvector 16, and the smoke test script is located at `./scripts/smoke_packages.sh`."*

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
