# Claude Code Integration with Memro

This example demonstrates giving [Claude Code](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview) persistent, cross-session memory using Memro.

## Prerequisites

- [Claude Code](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview) CLI installed (`npm install -g @anthropic-ai/claude-code`)
- A Memro API key (get one free at [memro.co](https://memro.co) or via self-hosting)

## Setup (One Command)

From your project directory:

```bash
npx memro-cli connect --project --client claude
```

This automatically creates or updates:
1. `./.mcp.json` with the Memro MCP server entry.
2. `./CLAUDE.md` with instructions directing Claude Code to recall relevant context on task startup and remember durable decisions.

Alternatively, manually configure the files as shown below.

### Manual Configuration

#### 1. `.mcp.json`
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

#### 2. `CLAUDE.md`
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

---

## Verification Lifecycle

Follow this 5-step test to verify persistent cross-session memory:

### Step 1: Launch First Claude Code Session
```bash
claude
```

### Step 2: Teach Claude a Fact
Prompt:
> *"Remember that our primary database is PostgreSQL 16 with pgvector and our API server listens on port 8081."*

Claude will call the `remember` MCP tool:
```json
{
  "content": "Our primary database is PostgreSQL 16 with pgvector and our API server listens on port 8081.",
  "type": "semantic"
}
```
Claude confirms: *"Got it, I've stored that in Memro memory."*

### Step 3: End the Session
Type `/exit` or press `Ctrl+C` to terminate the Claude Code process.

### Step 4: Launch a Fresh Session
Launch a completely new Claude Code session in a clean terminal:
```bash
claude
```

### Step 5: Ask Claude to Recall
Prompt:
> *"What database and port does our API server use?"*

Claude automatically calls the `recall` MCP tool:
```json
{
  "query": "database and port API server"
}
```
Claude answers accurately from Memro memory:
> *"Based on project memory, our primary database is PostgreSQL 16 with pgvector, and the API server runs on port 8081."*

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
