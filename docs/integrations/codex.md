# Integrating Memro with OpenAI Codex

OpenAI Codex and autonomous coding agents benefit from persistent memory across complex refactoring runs and terminal sessions.

## Quick Setup

Run the automated CLI connector from your project root:

```bash
npx memro-cli connect --project --client codex
```

This configures:
1. `./.codex/config.toml` with the Memro MCP server.
2. `./AGENTS.md` with guidelines directing Codex to retrieve relevant memory before modifying code and store permanent decisions.

## Manual Configuration

### 1. `.codex/config.toml`
```toml
[mcp.servers.memro]
command = "npx"
args = ["-y", "memro-cli", "mcp"]

[mcp.servers.memro.env]
MEMRO_API_KEY = "memro_sk_your_api_key_here"
```

### 2. `AGENTS.md`
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

1. **Launch Codex**:
   ```bash
   codex
   ```
2. **Teach Codex a Fact**:
   > *"Remember that this codebase is licensed under PolyForm Noncommercial 1.0.0 (source available), and commercial use requires Memro Cloud."*
3. **Exit Session**:
   Terminate the process.
4. **Launch Fresh Session**:
   ```bash
   codex
   ```
5. **Recall**:
   > *"What license governs this repository and what are the commercial terms?"*
   Codex calls `recall` and responds with the accurate stored context.

See the complete runnable example at [`examples/codex/`](../../examples/codex/).

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
