# Changelog

All notable changes to Memro will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
