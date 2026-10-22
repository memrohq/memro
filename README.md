# Memro — Verified Memory Infrastructure for AI Agents

Give your AI agents persistent memory across sessions: agent identity, memory storage and hybrid retrieval through an API, SDKs and MCP.

[![License: PolyForm Noncommercial](https://img.shields.io/badge/License-PolyForm%20Noncommercial-blue.svg)](LICENSE)
[![CI](https://github.com/memrohq/memro/actions/workflows/ci.yml/badge.svg)](https://github.com/memrohq/memro/actions/workflows/ci.yml)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](https://www.docker.com/)
[![MCP Ready](https://img.shields.io/badge/MCP-ready-purple.svg)](mcp)

> **License (Source Available)**
> 
> Memro is licensed under the [PolyForm Noncommercial License 1.0.0](LICENSE).
> - **Free for Non-Commercial Use**: Personal study, experiments, hobby projects, research, and testing without commercial application.
> - **Commercial Use**: Using Memro for commercial products or production services requires a managed subscription on [Memro Cloud](https://memro.co) or an enterprise commercial license.
> - **How to Get a Commercial License**: Visit [memro.co/pricing](https://memro.co/pricing) or contact `licensing@memro.co`.

> **Repository Notice**: `github.com/memrohq/memro` is the single public repository for Memro. It contains the Go server, official SDKs (Python, JavaScript/TypeScript, Go), Model Context Protocol (MCP) server, and the CLI.

---

## Quickstart

### Option A: Memro Cloud with Claude Code, Cursor, or Codex (< 10 lines)

```bash
# 1. Connect installed agent tools via device login:
npx memro-cli connect

# 2. Check connection status:
npx memro-cli status
```

In your assistant, say: *"Remember that our primary database is PostgreSQL 16 with pgvector."*  
Restart your session and ask: *"What database and extensions do we use?"*

### Option B: Self-Host with Docker Compose (Source Available)

Run the full Memro API server and PostgreSQL with pgvector on your own infrastructure:

#### Option 1: Clone the repository (works today)
```bash
git clone https://github.com/memrohq/memro && cd memro
cp .env.example .env              # add OPENAI_API_KEY for hybrid search; set METRICS_TOKEN
docker compose up -d              # API on :8081, dashboard on :5173, Postgres on 127.0.0.1:55432
curl http://localhost:8081/health
```

#### Option 2: Prebuilt images (no clone required, after v0.1.0 release)
```bash
curl -fsSL https://raw.githubusercontent.com/memrohq/memro/v0.1.0/docker-compose.release.yml -o docker-compose.yml && docker compose up -d
curl http://localhost:8081/health
```
*(Note: The prebuilt image one-liner downloads `docker-compose.release.yml` with pinned `ghcr.io/memrohq/memro:0.1.0` and `ghcr.io/memrohq/memro-dashboard:0.1.0` images, available once `v0.1.0` is published.)*

- **Backend API**: `http://localhost:8081`
- **Dashboard**: `http://localhost:5173`
- **Postgres Database**: Port `55432` on localhost (Option 1 clone) to avoid port 5432 collisions with existing local Postgres setups.
- **Metrics Token**: Set `METRICS_TOKEN` (min 32 characters) in `.env` to protect `GET /metrics` and `GET /stats/cache`; the server logs a warning if unset.
- **Verify Signatures**: Verify published container images keylessly using Cosign:
  ```bash
  cosign verify ghcr.io/memrohq/memro:0.1.0 \
    --certificate-identity-regexp 'https://github.com/memrohq/memro/' \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com
  ```

For complete deployment options, production topologies, and configuration tables, see [docs/self-hosting.md](docs/self-hosting.md).

---

## Architecture

```
┌──────────────────────────────────────┐
│         Clients & MCP Tools          │
│   Claude Code · Cursor · Codex · SDKs│
└──────────────────┬───────────────────┘
                   │  HTTP REST / Stdio MCP
┌──────────────────▼───────────────────┐
│            Memro API (Go)            │
│  Sovereign Ed25519 Identity · RRF    │
└──────────────────┬───────────────────┘
                   │  SQL & Embeddings
┌──────────────────▼───────────────────┐
│       PostgreSQL 16 + pgvector       │
│  pgvector Dense Search + tsvector FTS│
└──────────────────────────────────────┘
```

Memro runs a Go HTTP API backed by PostgreSQL and pgvector. Search queries combine vector cosine similarity and full-text keyword search using Reciprocal Rank Fusion (RRF, $k=60$). Agents authenticate using sovereign Ed25519 keypairs solved via local proof-of-work (private keys never leave the host) or standard Bearer API tokens.

---

## Repository Layout

`memrohq/memro` holds all components in one mono-repository:

| Path | Description | Documentation |
|---|---|---|
| [`server/`](server) | Go API server, PostgreSQL + pgvector engine | [Self-Hosting Guide](docs/self-hosting.md) |
| [`cli/`](cli) | One-command connector (`npx memro-cli connect`) | [CLI Documentation](cli/README.md) |
| [`mcp/`](mcp) | Model Context Protocol server (stdio & SSE HTTP) | [MCP Guide](docs/mcp.md) |
| [`sdk/python/`](sdk/python) | Official Python SDK (PyPI: `memro`) | [Python SDK Guide](sdk/python/README.md) |
| [`sdk/js/`](sdk/js) | Official JavaScript / TypeScript SDK (npm: `@memro/sdk`) | [JavaScript SDK Guide](sdk/js/README.md) |
| [`sdk/go/`](sdk/go) | Official Go SDK (`github.com/memrohq/memro/sdk/go@v0.1.0`) | [Go SDK Guide](sdk/go/README.md) |
| [`examples/`](examples) | Runnable integrations (Claude Code, Cursor, Codex) | [Examples Index](examples/README.md) |
| [`bench/`](bench) | Reproducible benchmark harness and test suites | [Benchmarks Index](docs/benchmarks/README.md) |
| [`docs/`](docs) | Architecture, concepts, API reference, decisions | [Documentation Index](docs/README.md) |

---

## Official SDKs & Runnable Examples

### Python (`sdk/python`)
```bash
# PyPI (once published) or install from source: pip install ./sdk/python
pip install memro
```
```python
from memro import Agent

agent = Agent.create()  # local sovereign Ed25519 identity
mem = agent.remember("The billing service was rewritten in Go", type="semantic")
results = agent.recall("billing service language")
print(results[0].content)
```
See runnable script at [`examples/python/register_remember_recall.py`](examples/python/register_remember_recall.py).

### JavaScript / TypeScript (`sdk/js`)
```bash
# npm (once published) or install from local build: npm install ./sdk/js
npm install @memro/sdk
```
```typescript
import { Agent } from '@memro/sdk';

const agent = await Agent.create(); // local sovereign Ed25519 identity
const mem = await agent.remember('The billing service was rewritten in Go', { type: 'semantic' });
const results = await agent.recall('billing service language');
console.log(results[0].content);
```
See runnable script at [`examples/typescript/index.js`](examples/typescript/index.js).

### Go (`sdk/go`)
```bash
go get github.com/memrohq/memro/sdk/go@v0.1.0
```
```go
package main

import (
	"context"
	"fmt"
	memro "github.com/memrohq/memro/sdk/go"
)

func main() {
	ctx := context.Background()
	agent, _ := memro.CreateAgent(nil)
	mem, _ := agent.Remember(ctx, "The billing service was rewritten in Go", nil)
	results, _ := agent.Recall(ctx, "billing service language", nil)
	fmt.Println(results[0].Content)
}
```
See runnable script at [`examples/go/main.go`](examples/go/main.go).

---

## Integration Guides

Step-by-step guides for connecting coding tools with full session-restart memory verification:
- [Claude Code Integration](docs/integrations/claude-code.md) ([`examples/claude-code/`](examples/claude-code/))
- [Cursor Integration](docs/integrations/cursor.md) ([`examples/cursor/`](examples/cursor/))
- [OpenAI Codex Integration](docs/integrations/codex.md) ([`examples/codex/`](examples/codex/))

---

## Advanced Memory Features

Memro includes structured primitives and cognitive graphs beyond basic vector search:
- **Typed Memories**: Categorize observations as `semantic` (facts), `episodic` (events/conversations), `procedural` (runbooks/recipes), or `profile` (agent guardrails). See [Memory Concepts](docs/concepts.md).
- **Temporal Search & Evolution**: Query memories with timestamp weighting and trace how beliefs evolve over time. See [API Reference](docs/api-reference.md).
- **Conflict Analysis & Resolution**: Identify contradictory facts across sessions and mark superseded memories using `resolve_conflict()`. See [Conflict Resolution](docs/concepts.md#conflict-resolution).
- **Reasoning Graphs**: Trace memory provenance, causal chains, and dependencies between stored observations. See [Opinionated Guide](docs/Opinionated_Guide.md).

---

## Measurable Verification & Benchmarks

Memro publishes empirical evaluations and methodology:
- **LongMemEval Benchmark**: Evaluated across 500 questions testing multi-session recall and knowledge updates. See [LongMemEval Report](docs/benchmarks/longmemeval.md).
- **Empirical Failure & Boundary Evaluations**: Runnable test suite for identity boundaries, contradictory facts, stale facts, and distractor rejection. See [Measurable Evaluations](docs/benchmarks/evaluations.md).
- **Clean-Machine Verification**: Full transcript verifying every quickstart command in a container without local caches. See [Verification Transcript](docs/verify-quickstart.md).

---

## Contributing & Development

```bash
# 1. Start PostgreSQL with pgvector
docker compose up -d db

# 2. Run backend API server
cd server
DATABASE_URL=postgres://memro:memro@localhost:55432/memro go run ./cmd/memro-api

# 3. Run tests
MEMRO_TEST_DATABASE_URL=postgres://memro:memro@localhost:55432/memro go test -race ./...
```

Please review [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md) before submitting pull requests.

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use requires Memro Cloud or a commercial license.
