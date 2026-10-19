# 0010. Connector hardening: untrusted memory content, working claims, safer config writes
Date: 2026-10-19 · Status: Accepted

## Context
A review of the tool connectors (the `memro` CLI, the hosted MCP server, the
VS Code extension and the Acode plugin) found:

- The VS Code Memory Explorer put memory content into `innerHTML` with
  scripts enabled and no Content-Security-Policy. Memories can be written by
  any agent or extracted from uploaded documents, so a stored payload could
  insert text into the user's editor or open files outside the workspace.
- The Acode plugin applied model-issued writes and deletes without asking,
  executed JSON found in reply text, and accepted `..` paths.
- MCP claims were never released: `release_task` and `handoff_task` added a
  memory but the claim stayed `active`, so a file stayed blocked until it
  scrolled out of the last 50 memories. A procedural memory without metadata
  crashed every coordination tool.
- The VS Code extension prompted for a key every 10 seconds when none was set,
  and by default sent every opened file path and claimed every edited file.
- `/mcp` accepted `?token=`; the MCP server trusted any `X-Real-IP`; its key
  cache held raw keys with no expiry.
- The CLI rewrote configs in place, accepted `http://` for remote APIs,
  minted a second key when the API returned 5xx, wrote configs without asking
  when stdin was not a terminal, and ran an unpinned `mcp-remote`.

## Decision
- **Memory content is untrusted in every client.** Webviews escape all
  interpolated values under a nonce-based CSP with no inline handlers; actions
  that write to the user's files ask first; paths from memories or models must
  resolve inside the workspace or project folder.
- **Claims follow the newest action per resource**: a release or handoff
  settles every older claim and intent on it. The MCP server reads the last
  100 procedural memories (was 50 of any type). This is an interim fix; claims
  become a server primitive with leases (see Consequences).
- **Coordination is opt-in in VS Code** (`memro.checkOnOpen`,
  `memro.autoClaimOnEdit`, both off). Background work never prompts for a key.
  The session is sent as `?session_id=` on the MCP URL, which is what the
  server reads.
- **MCP server**: header-only credentials on `/mcp` (`?token=` stays on
  legacy `/sse/` only); `X-Real-IP` only with `MCP_TRUST_PROXY_HEADERS=true`;
  the key cache is an LRU keyed by SHA-256 with a 300 s TTL; tool limits are
  capped at 50; `remember*` pass through every field their schemas advertise;
  unexpected errors return a reference ID instead of exception text; uvloop
  is skipped on Windows; dependencies have bounds.
- **CLI**: configs are replaced atomically (temp file + rename); remote APIs
  and the returned MCP URL must be HTTPS; only 401/403 triggers a new sign-in;
  sign-in polling survives dropped connections and backs off on 5xx/429;
  non-interactive runs need `--yes`; `mcp-remote` is pinned; on Windows the
  Claude Code config is edited directly instead of going through `cmd.exe`.

## Alternatives considered
- **Sanitising HTML (DOMPurify) instead of escaping.** Memory content is
  plain text; rendering it as text removes the class of bug rather than
  filtering it.
- **Unpublishing the Acode plugin.** Still an option; the plugin is not a
  Memro memory client. Hardening it removes the immediate risk while that is
  decided.
- **Fixing claims with a database table now.** Correct long term, but needs a
  migration and API endpoints; the interim fix stops permanent blocks today.

## Consequences
- Deploy order: memro-cloud must set `MCP_TRUST_PROXY_HEADERS=true` before or
  with the new MCP image, or all MCP users share one API rate-limit bucket.
- A revoked key can still open an MCP session for up to 5 minutes (its tool
  calls fail immediately).
- Claims are still check-then-write and still live in memories. Next step: a
  `claims` table with a unique `(agent, session, resource)` row, a lease TTL
  and atomic claim/release endpoints, used by the MCP tools.
- Still open: one key per tool and a self-revoke endpoint so `memro
  disconnect` can revoke; JSONC-preserving edits for VS Code `mcp.json`; the
  key is still on `claude mcp add`'s argv on macOS/Linux.
