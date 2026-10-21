# 0015. Verified memory for coding agents: anchors, validity and provenance
Date: 2026-10-21 · Status: Proposed · Builds on 0011 (connectors)

## Context
Memro already gives coding agents shared, persistent memory: Claude Code,
Cursor, Codex and the other MCP clients that `memro-cli` sets up read and
write the same store. That alone is not a moat; Mem0, Zep and Letta store
and retrieve memories too.

What coding agents lack is memory they can **trust as code changes**. A
memory such as "auth middleware must run before the rate limiter" is true at
one commit and may be false three weeks later. Today Memro cannot tell:

- **No link to code.** A `remember` from a coding agent stores text, a type
  and dates. Nothing records the repository, commit or files it is about.
- **No provenance at recall.** Search results return content, type, score and
  date. Even connector memories, which carry `metadata.source` and a URL, lose
  it at recall: `SearchResult` has no metadata.
- **No validity.** Old and current decisions rank by similarity alone. The
  LongMemEval error analysis (2026-10-21) shows the cost: knowledge-update
  questions fail mostly by choosing an outdated fact (11 of 13 answering
  errors), and half of all wrong answers had the right evidence retrieved.

The hosted MCP server (`mcp.memro.co`) cannot read a user's repository, so
anything about the current state of the code has to come from the agent,
which can run `git`, or from a repository connected through the GitHub
connector.

## Decision

### 1. Memories can carry a code anchor
An optional, structured anchor saved with the memory:

```json
"anchor": {
  "repo": "github.com/acme/api",
  "commit": "9f2c1e0…",
  "paths": [
    {"path": "server/middleware/auth.go", "blob": "a1b2c3…"},
    {"path": "server/router.go", "blob": "d4e5f6…"}
  ]
}
```

- `repo` is the normalized remote URL (no credentials, no `.git`). `blob` is
  the git object ID of the file's content at `commit`
  (`git rev-parse <commit>:<path>`).
- **No code is stored**: only paths and hashes.
- Stored in a new `memory_anchors` table (memory ID, repo, commit, path, blob),
  indexed by `(agent, repo, path)`. Keeping it out of the metadata JSON makes
  it queryable.
- **Who fills it in.** The MCP `remember` tool gains an `anchor` argument, and
  its description tells coding agents how to fill it (one `git` command).
  `memro-cli` adds the same instruction to the project notes it writes. The
  SDKs get a `codeAnchor(paths)` helper that runs `git` locally.

### 2. Recall reports validity, with evidence
Each result gains `validity` and `provenance`:

| `validity.status` | Meaning |
|---|---|
| `current` | Every anchored file still has the same content |
| `changed` | One or more anchored files changed since the memory was saved; listed with old and new blob |
| `missing` | An anchored file no longer exists (deleted or renamed) |
| `superseded` | A newer memory with an overlapping anchor or contradicting content replaced it (existing contradiction detection) |
| `unchecked` | Anchored, but the current state is unknown (the agent supplied none) |
| `unanchored` | Not about code, or saved without an anchor |

- **How the current state reaches the server:**
  1. **Agent-supplied (works everywhere).** `recall` accepts an optional
     `repo_state: {repo, head, blobs: {path: blob}}`. Because the agent cannot
     know which files matter before searching, recall returns `unchecked`
     memories together with the paths to check, and a new MCP tool
     `check_memories(repo_state)` returns their status. The agent runs one
     command: `git rev-parse HEAD:<path> …`.
  2. **Connector-backed (no agent work).** When the repository is connected
     through the GitHub connector, the server asks GitHub for the current blob
     of each anchored path on the default branch, with a short cache, and
     fills `validity` itself.
- **Stale memories are labeled, never hidden.** Ranking demotes `changed`,
  `missing` and `superseded` (a multiplier on the fused score, tuned on the
  benchmark) but keeps them, so an agent can still see why something used to
  be true. A `changed` memory is often still right; the label invites a
  check.
- **Provenance on every result:** who saved it (agent ID and display name,
  connector or tool), when, the session, `metadata.source` and URL for
  connector memories, and the anchor. `SearchResult` gains `metadata` and
  `provenance`, and the MCP `recall` output includes both.

### 3. Measure it before claiming it
No positioning claim about verification ships until the code-memory
benchmark ([design](../benchmarks/code-memory.md)) shows, with paired tests,
fewer stale answers than plain retrieval at no loss of correct recall. The
benchmark and its limitations are published with the results.

### Out of scope here
- Re-validating memory *content* against code semantically (only "the files
  changed").
- Anchoring to symbols or line ranges (paths first; symbols may follow if the
  benchmark shows file-level checks are too coarse).
- Non-git sources (documents, tickets): connector memories already carry
  provenance; their validity is a later decision.

## Alternatives considered
- **Server clones or reads the repository.** Rejected for the hosted service:
  it needs code access many users will not grant, and it is slow. Kept as the
  connector-backed path for repositories already connected via GitHub.
- **Timestamps only** ("older than N days is stale"). Cheap but wrong both
  ways: an old memory about untouched code is fine, and a new one is stale the
  moment the file changes.
- **Store the code snippet and diff it.** Precise, but stores source code in
  the memory service, which many teams will not accept. Hashes give the same
  "did it change" signal without the code.
- **Drop stale memories from results.** Hides history an agent may need
  ("we tried X, it broke Y"). Labeling keeps it and lets ranking demote it.

## Consequences
- Coding agents get answers they can act on: current decisions first, and the
  evidence to check anything that might be out of date.
- One extra round trip (`check_memories`) for agents without a connected
  repository; one `git` command, no file contents sent.
- New storage: one row per anchored path (tens of bytes each).
- `SearchResult` grows (`metadata`, `provenance`, `validity`); clients ignore
  unknown fields, so this is backward compatible.
- The MCP tool descriptions and the CLI's project notes change. Agents that
  ignore them get today's behaviour (`unanchored`).
- Order of work: (1) provenance in search results; (2) anchors on save plus
  `memory_anchors`; (3) `check_memories` and validity in recall; (4) the
  benchmark, then ranking weights tuned on it; (5) the connector-backed check.
