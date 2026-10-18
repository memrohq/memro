# 0006. API keys and device-code login for `memro connect`
Date: 2026-10-19 · Status: Accepted

## Context
Connecting tools (Claude Code, Codex, Cursor, Antigravity, OpenCode, …)
required pasting an agent's raw private key, and the hosted MCP server took
it in the URL.

## Decision
- `memro_sk_` API keys, stored as SHA-256 hashes, revocable, scoped to an
  agent. Only dashboard sessions can create/revoke keys or approve logins.
- RFC 8628 device flow: `memro connect` gets a code, the user approves it at
  `app.memro.co/connect`, and the CLI receives a key once.
- The hosted MCP server accepts API keys over Streamable HTTP (`/mcp`), and the
  CLI writes each tool's MCP config.

## Consequences
No private keys in tool configs or URLs. Next: MCP OAuth so clients can sign
in without any key; organizations/teams on top of keys.
