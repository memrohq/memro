# OpenAI Codex Integration with Memro

This example demonstrates integrating OpenAI Codex / Agent tools with Memro for cross-session persistent memory.

## Prerequisites

- Codex CLI or agent environment installed
- A Memro API key (get one free at [memro.co](https://memro.co) or via self-hosting)

## Setup (One Command)

From your project directory:

```bash
npx memro-cli connect --project --client codex
```

This automatically configures:
1. `./.codex/config.toml` with the Memro MCP server table.
2. `./AGENTS.md` with instructions guiding the agent to recall context on task startup and persist decisions.

---

### Manual Configuration

#### 1. `.codex/config.toml`
```toml
[mcp.servers.memro]
command = "npx"
args = ["-y", "memro-cli", "mcp"]

[mcp.servers.memro.env]
MEMRO_API_KEY = "memro_sk_your_api_key_here"
```

#### 2. `AGENTS.md`
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

Follow this 5-step test to verify persistent cross-session memory in Codex:

### Step 1: Launch First Codex Session
```bash
codex
```

### Step 2: Teach Codex a Fact
Prompt:
> *"Remember that this codebase is licensed under PolyForm Noncommercial 1.0.0 (source available), and commercial use requires Memro Cloud."*

Codex invokes `remember`:
```json
{
  "content": "This codebase is licensed under PolyForm Noncommercial 1.0.0 (source available), and commercial use requires Memro Cloud.",
  "type": "profile"
}
```
Codex acknowledges: *"Stored in persistent memory."*

### Step 3: Exit Codex Session
Exit the Codex process (`exit` or `Ctrl+D`).

### Step 4: Launch Fresh Codex Session
Launch a new session with no prior conversation state:
```bash
codex
```

### Step 5: Ask Codex to Recall
Prompt:
> *"What license governs this repository and what are the commercial terms?"*

Codex automatically calls `recall`:
```json
{
  "query": "license terms repository commercial"
}
```
Codex replies accurately:
> *"This codebase is licensed under PolyForm Noncommercial 1.0.0 (source available). It is free for non-commercial use, and commercial use requires Memro Cloud or a commercial license."*

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
