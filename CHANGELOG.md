# Changelog

All notable changes to Memro will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and every release and package raises only the last number of its version (see RELEASING.md).

## [Unreleased]

### Added
- **Rules live in the repo, in `.memro/rules.md`, and `memro rules` manages them**: rules the hooks catch ("never…", "always…") are appended to `.memro/rules.md` instead of your Memro account, so you see each one in `git diff`, fix it by editing a line and share it by committing the file. `npx memro-cli rules` lists them, `rules edit <n> <text>` and `rules delete <n>` change one, and `rules import` moves rules saved in your account earlier into the file. Rules already in your account keep reaching agents until imported (ADR 0037).
- **Hooks remember the rules you state, and show them**: when a prompt says "never…", "always…", "from now on…" or "as a rule…", the prompt hook saves that sentence and the next one to your Memro account as `Rule for <repo>: …`, and every later prompt in that repo, in any tool, is shown the repo's saved rules. The first edit of a file also shows what Memro holds about it (similarity ≥ 0.4). In the relay benchmark with 300 facts in memory, this took Memro from 6/10 to 10/10 traps avoided (a notes file: 9/10).
- **`memro land`: worktree branches land one at a time**: run in a git worktree, it waits its turn on a `land/<repo>` lease, rebases the branch on the main checkout's branch and fast-forwards it. On a conflict it stops, keeps its turn, and lists the commits that landed on the conflicted files, so the agent knows what the other change was for. In the launch benchmark, 4 of 5 worktree branches conflicted at merge time every run.
- **Busy files free up sooner, and refused agents retry**: each edit holds its file for 45 seconds (renewed by the next edit, `MEMRO_HOOK_HOLD_SECONDS`) instead of the whole turn, and a refused agent is told to finish the rest of its task and retry rather than stop and ask the user. In the launch benchmark, turn-long holds doubled run time and every lost task was an agent that gave up.
- **Coordination hooks for Cursor, Gemini CLI and OpenCode**: `memro connect` adds the same file claims, git guard, status and handoffs to Cursor (`~/.cursor/hooks.json`), Gemini CLI (`~/.gemini/settings.json`) and OpenCode (a plugin in `~/.config/opencode/plugins/`). What each tool can show its agent differs: Gemini CLI gets briefings and messages but no notes on edits; Cursor and OpenCode get refusals but no briefings or notes; OpenCode can't be woken by a message.
- **Handoff notes**: at the end of each turn the hooks hand the files the agent changed off with a note (its session, task and the start of its last message), and the next agent to edit one of those files is shown the note once. Notes are kept 30 days with the other coordination history.
- **Agents see each other's tasks before they start**: when a prompt is submitted, the Claude Code and Codex hooks record its first line as the agent's task (`memro agents` shows it) and brief the agent on who else works in the repo, what they're doing and which files they hold, flagging files the prompt names that someone holds. The first ~80 characters of each prompt are stored in the account as the task label.
- **A window's MCP claims and its hooks' claims are one holder**: the coordination MCP tools take an optional `holder_session`, which the Memro hooks fill in with the window's session (`claude-code:<id>`, `codex:<id>`). Before, every window's MCP calls shared the tool's session, so two windows could claim over each other and a window's own folder claim was refused when its hook held a file inside it.
- **Shell commands and git are coordinated too**: the Claude Code and Codex hooks claim files a shell command writes or deletes (`>`, `>>`, `tee`, `sed -i`, `rm`) like edits, and refuse `git stash`, `reset --hard`, `checkout`, `restore`, `switch` and `clean -f` while another agent works in the same checkout, since they would throw away its uncommitted changes. Every wait, refusal and warning is logged to `~/.memro/hooks/log.jsonl`. Codex asks to trust the changed hook again in `/hooks`.
- **`memro connect` makes Claude Code and Codex claim each file before editing it** (ADR 0019 §7): a hook claims the file in Memro before every Edit or Write and refuses the edit when another agent or window holds it, so coordination no longer depends on the model remembering to call `claim_task`. Each Claude Code window is its own holder, files are named `<repo>/<path>` so every worktree agrees, and a window's claims are released when it closes. `--no-hooks` skips it. Codex gets the same hooks in `hooks.json` (every file in an `apply_patch` is claimed); trust them once with `/hooks` in Codex.
- **`memro agents` and `memro send`**: the same hooks report each Claude Code window's status (working, waiting for you, idle), and `agents` lists every agent with its status and the files it holds. `send <agent> <message>` messages an agent; Claude Code reads it at its next prompt, or is woken by it when it stops. An idle agent in a herdr pane on the same machine is prompted through herdr straight away. Files are held for one turn (released when the agent stops), an edit to a busy file waits up to 60 seconds for it, and an agent in another worktree gets a merge-conflict warning instead of a refusal.
- **MCP `recall` takes a `token_budget`**: it returns the best-ranked memories that fit in about that many tokens (estimated at ~4 characters per token), skips superseded and duplicate memories, and emits compact JSON. Without it, `recall` behaves as before.

### Fixed
- **MCP `recall` shows which memories are out of date**: it ranks a memory below a newer one that contradicts it and marks it with `superseded_by`. Before, the tool dropped the flag and did not ask for conflict resolution. The Python SDK's `SearchResult` now keeps `is_superseded` and `superseded_by`.
- **`min_score` now filters irrelevant results** (ADR 0033): search results include `similarity`, the cosine similarity to the query, and the Python, JS and Go SDKs apply `min_score` to it. Before, `min_score` filtered the rank-based fused score, which is above 0.4 for any of the top 16 candidates however unrelated.
- **Sentiment and negation words match whole words only** (ADR 0033): "badge" no longer counts as "bad", nor "likely" or "dislike" as "like", which had marked unrelated memories as contradicting.
- **Far fewer false contradictions in search**: the negation and sentiment rules now need the two statements to make the same claim (most content words shared), so ordinary chat messages that happen to share a word and contain "not" no longer demote each other (15 → 1 of 153 message pairs on one LongMemEval question). "Never use npm" still replaces "we use npm".
- **Fewer stale answers and duplicates in search**: contradiction detection now matches "X is Y" with "X: Y", understands "X moved/switched [from A] to B" (only against a where/when value of the same kind, so "Ana moved to the Berlin office" leaves "Ana is in the design team" alone), treats invoice/order/ticket numbers as identifiers rather than changed quantities, and marks near-duplicate restatements as superseded (except for counting questions). The save-time linker (`GRAPH_WRITE_EDGES`, still off by default) also finds candidates by shared words, not only entity tags, and links duplicates of non-episodic memories. On the new evolution benchmark (`TestEvolutionBench`): unresolved updates 4/9 → 0/9, duplicates left 2/2 → 0/2, false updates 0/7.
- **Memory evolution can be re-run safely**: `Evolve` (`POST /mee/evolve`, the `mee_evolve` tool) consolidates each episodic memory once, so repeated runs no longer add duplicate summaries of the same memories. One run folds in at most the 50 newest unconsolidated episodes and compares at most the 300 newest facts for contradictions, so its cost no longer grows with the agent's whole history.

### Changed
- **Deleting a memory archives it** (ADR 0032): `DELETE /memory/{id}` and the `delete_memory` tool move it out of search and listings, and `POST /memory/restore/{id}` brings it back. `?purge=true` removes it for good, together with the content kept in its history.

### Added
- **Agents can correct their memories** (ADR 0032): `PUT /memory/{id}` and the `update_memory` MCP tool save a corrected version and archive the old one, with the reason in both histories; `delete_memory` takes a `reason`. The Python, JS and Go SDKs add `update_memory`/`updateMemory`/`UpdateMemory`, plus restore and history.
- **Memory history and provenance** (ADR 0032): `GET /memory/history/{id}` lists every change to a memory (who, from where, content before and after, why). Memories record their `source` (`mcp`, `dashboard`, `api`, `import:pdf`…) and `author_type`; the MCP server identifies itself with `X-Memro-Source: mcp`, and batch writes are audited per memory.
- **Claude Code plugin** (`integrations/claude-code`, marketplace `memrohq/memro`): the Memro MCP server, with the API key asked at install and kept in the keychain, plus a SessionStart hook that recalls the project's memories and adds the Memro note at the start of every session.

### Added
- **memro-cli `doctor`**: for each tool, the Memro config it uses (project over global), where its key comes from, whether the server accepts it and lists its tools, and whether the tool has the Memro note. Exits 1 when a tool needs attention.
- **memro-cli `--env-key`**: Claude Code and Codex read the key from `$MEMRO_API_KEY` instead of storing it, so project config files can be committed.
- **memro-cli Cursor rule**: project setup adds `.cursor/rules/memro.mdc` (always applied).

### Added
- **memro-cli and GitHub Copilot**: `connect` adds the Memro note for VS Code's Copilot Chat (`~/.copilot/instructions/memro.instructions.md`, or `.github/instructions/memro.instructions.md` with `--project`), so Copilot recalls and remembers without being asked.

### Fixed
- **memro-cli and Codex project setups**: Codex reads a project's `.codex/config.toml` only in folders you've trusted in Codex. `doctor` now reports an untrusted folder (and that Codex falls back to the global config), and `connect --project` says to trust the folder.

### Changed
- Package author and the contact addresses in the docs are now subal@memro.co.

Packages: `memro-cli` 0.1.7, `memro` 0.2.1, `memro-mcp` 0.2.2, `@memro/sdk` 0.2.1.

## [v0.1.1] - 2026-11-14

Organisations and per-customer tenant keys, a faster 2D memory graph, batch
ingest, better handling of facts that change, and multi-tool coordination.
The first tagged release: Docker images, Go module tags and the GitHub
release are now produced from the tag.

Packages: `memro` 0.2.0, `memro-mcp` 0.2.1, `@memro/sdk` 0.2.0, `memro-cli`
0.1.6, Go SDK `sdk/go/v0.1.1`.

### Added
- **Organisations**: organisation accounts with verified work-email domains, join requests, plans, trials, limits, usage per member, account suspension and a staff console (ADRs 0026–0030). [#50](https://github.com/memrohq/memro/pull/50), [#51](https://github.com/memrohq/memro/pull/51)
- **Tenant keys**: one server key per business customer, with each of their end users isolated. The JS SDK can name and delete an end user. [#49](https://github.com/memrohq/memro/pull/49)
- **Batch ingest**: `POST /memories/batch` saves up to 500 memories in one transaction; `remember_batch` / `rememberBatch` / `RememberBatch` in the SDKs. [#44](https://github.com/memrohq/memro/pull/44)
- **Memory graph**: `GET /memory/graph` and a dashboard graph of memories and the people, companies and conversations linking them, now 2D WebGL with search, clusters and full screen, cached per agent. [#46](https://github.com/memrohq/memro/pull/46), [#47](https://github.com/memrohq/memro/pull/47), [#51](https://github.com/memrohq/memro/pull/51)
- **Coordination**: server-side leases, intents and handoffs so several AI tools on one account don't edit the same files (`claim_task`, `pre_action_check`, `handoff_task`, …); folder claims cover their files. [#35](https://github.com/memrohq/memro/pull/35), [#39](https://github.com/memrohq/memro/pull/39)
- **Self-hosting**: a dashboard image and `docker-compose.release.yml` for running prebuilt images without cloning. [#30](https://github.com/memrohq/memro/pull/30)
- **Benchmarks**: reproducible LongMemEval results with costs, and a 100-question measurement subset with a CI gate. [#27](https://github.com/memrohq/memro/pull/27), [#37](https://github.com/memrohq/memro/pull/37)

### Changed
- **Changing facts**: search recognises when a newer memory supersedes an older one and resolves contradictions by intent and time. [#45](https://github.com/memrohq/memro/pull/45)
- **Search**: keyword search matches parts of email addresses and domains; aggregate questions draw results from more conversations. [#42](https://github.com/memrohq/memro/pull/42), [#51](https://github.com/memrohq/memro/pull/51)
- **Auth (breaking for old clients)**: legacy v1 request signatures are disabled by default; replay protection and metrics added. [#33](https://github.com/memrohq/memro/pull/33)
- The sign-in code email was redesigned, and app.memro.co has a clear sign-in title. [#31](https://github.com/memrohq/memro/pull/31), [#48](https://github.com/memrohq/memro/pull/48)

### Fixed
- Migrations are serialised with an advisory lock, so several server replicas can start at once. [#36](https://github.com/memrohq/memro/pull/36)
- Signature hex is normalised for the replay cache and must be 128 hex characters. [#32](https://github.com/memrohq/memro/pull/32)

### Security
- `/metrics` and `/stats/cache` require a token. [#29](https://github.com/memrohq/memro/pull/29)
- Device approval verification and smaller hardening items. [#34](https://github.com/memrohq/memro/pull/34)

### Upgrading
- Self-hosted servers run migrations 0033–0038 on start. 0034 builds a GIN index on `memories` without `CONCURRENTLY`, which blocks writes while it builds: back up first and upgrade in a quiet window if the table is large.

## [v0.1.0] - 2026-10-22

### Initial Source-Available Release

Memro is persistent, verifiable memory infrastructure for AI agents. This release establishes the core server engine, official client SDKs (Python, JavaScript/TypeScript, Go), Model Context Protocol (MCP) server, and one-command CLI setup.

#### What Works

- **Core Server Engine (`server/`)**:
  - High-performance Go memory engine running on PostgreSQL with `pgvector`.
  - Hybrid search retrieval combining vector similarity with Reciprocal Rank Fusion (RRF) and tsvector full-text search.
  - Cryptographic sovereign identity: agents register Ed25519 public keys locally solved with proof-of-work (`/identity/pow`); private keys never leave the host.
  - Signed request authentication (v2 Ed25519 signatures) alongside standard Bearer API tokens (`memro_sk_...`).
  - Memory types: semantic, episodic, procedural, and profile memories with metadata scoping.
  - File and document ingestion (PDF, Word, text, images).
  - Health check endpoint at `/health` and Prometheus metrics at `/metrics` (authenticated).

- **Official SDKs**:
  - **Python SDK (`sdk/python`, PyPI: `memro`)**:
    - Support for Python 3.8+ with Ed25519 crypto (`pynacl`), `requests`, and `pydantic`.
    - LangChain memory integration (`memro[langchain]`).
  - **JavaScript / TypeScript SDK (`sdk/js`, npm: `@memro/sdk`)**:
    - Zero external runtime dependencies beyond `tweetnacl` and `@noble/hashes`.
    - Dual ESM and CommonJS exports with full TypeScript definitions for Node 18+, Deno, Bun.
  - **Go SDK (`sdk/go`, `github.com/memrohq/memro/sdk/go`)**:
    - Zero external dependencies outside the Go standard library.
    - Context-aware methods, retry logic with exponential backoff and `Retry-After` header handling.

- **Model Context Protocol Server (`mcp/`, PyPI: `memro-mcp`)**:
  - Standard stdio transport for local AI development tools.
  - Starlette/uvicorn HTTP SSE transport for cloud/remote agent environments.
  - Standard MCP tools: `remember`, `recall`, `get_recent_memories`, `delete_memory`, `export_memories`.

- **Developer CLI (`cli/`, npm: `memro-cli`)**:
  - One-command setup: `npx memro-cli connect`.
  - Automated MCP tool configuration and instruction guidance injection for:
    - Claude Code (`~/.claude.json` / `./.mcp.json` and `CLAUDE.md`)
    - Cursor (`~/.cursor/mcp.json` / `./.cursor/mcp.json`)
    - OpenAI Codex (`~/.codex/config.toml` and `AGENTS.md`)
    - VS Code, Antigravity, OpenCode, Gemini CLI, and Windsurf.

- **Deployment**:
  - Single-command self-hosting via root `docker compose up -d`.
  - Automated multi-arch Docker image packaging (`ghcr.io/memrohq/memro`) signed via Sigstore cosign.

#### Known Limitations

- **Temporal Reasoning & Memory Graph Evolution**:
  - Temporal reasoning queries and automated memory graph evolution are under active development and require tuning for domain-specific knowledge graphs.
- **Licensing**:
  - Distributed under the [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0) license (source available). Free for non-commercial personal, academic, and open research use. Commercial production use requires a managed Memro Cloud subscription or a commercial license agreement.
- **Local Embedded Models**:
  - The default self-hosted Docker configuration connects to embedding APIs or local proxy embedding services; local ONNX runtime embeddings are planned for a subsequent minor release.

[v0.1.0]: https://github.com/memrohq/memro/releases/tag/v0.1.0
