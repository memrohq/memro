# Memro AI Agent Integrations

Memro connects to leading agent development environments and MCP clients with zero friction. Each integration enables persistent, cross-session memory retrieval and automatic durable storage.

## Supported Integrations

- [Claude Code Integration Guide](claude-code.md): Complete setup and session verification lifecycle for Anthropic's Claude Code CLI.
- [Cursor Integration Guide](cursor.md): Configuration and verification steps for Cursor Composer and Chat.
- [OpenAI Codex Integration Guide](codex.md): Setup and verification steps for OpenAI Codex CLI and agents.

## One-Command Setup

The easiest way to connect all your installed AI developer tools is via the official Memro CLI:

```bash
# Global configuration for all detected tools
npx memro-cli connect

# Or project-scoped configuration in the current repository
npx memro-cli connect --project
```

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
