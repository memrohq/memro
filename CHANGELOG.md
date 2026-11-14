# Changelog

All notable changes to Memro will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.1.1] - 2026-11-14

Organisations and per-customer tenant keys, a faster 2D memory graph, batch
ingest, better handling of facts that change, and multi-tool coordination.
The first tagged release: Docker images, Go module tags and the GitHub
release are now produced from the tag.

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
