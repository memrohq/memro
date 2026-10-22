# Memro — Verified Memory Infrastructure for AI Agents

Persistent, verifiable memory for AI agents. Retrieve relevant context, validate facts, eliminate contradictions, and build reliable autonomous AI systems.

[![License: PolyForm Noncommercial](https://img.shields.io/badge/License-PolyForm%20Noncommercial-blue.svg)](LICENSE)
[![CI](https://github.com/memrohq/memro/actions/workflows/ci.yml/badge.svg)](https://github.com/memrohq/memro/actions/workflows/ci.yml)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](https://www.docker.com/)
[![MCP Ready](https://img.shields.io/badge/MCP-ready-purple.svg)](mcp)
[![Contributions Welcome](https://img.shields.io/badge/contributions-welcome-brightgreen.svg)](CONTRIBUTING.md)

---

## 60-Second Quickstart

### Option A: Connect Your Coding Assistants (Cloud & MCP)

Connect Claude Code, Cursor, Codex, VS Code, Windsurf, or Claude Desktop with a single command:

```bash
# 1. Sign in via browser device flow and auto-configure local tools
npx memro-cli connect

# 2. Check connection status
npx memro-cli status
```

Your coding assistants will immediately have access to persistent project and developer context across sessions.

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

Memro couples dense vector embeddings with PostgreSQL full-text search, fused via Reciprocal Rank Fusion ($k=60$) and audited by a contradiction resolution engine.

```
┌────────────────────────────────────────────────────────────────────────┐
│                          AI Clients & Agents                           │
│   Claude Code · Cursor · Codex · Claude Desktop · Autonomous Agents    │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                        Ed25519 Signed REST / MCP
                                    │
┌───────────────────────────────────▼────────────────────────────────────┐
│                       Memro Engine & API Server                        │
│                               (Go Server)                              │
├───────────────────────────────────┬────────────────────────────────────┤
│        Verification Engine        │         Temporal Ledger            │
│   Contradiction detection         │   Chronological timeline offsets   │
│   Supersedes obsolete facts       │   Calendar hallucination guard     │
├───────────────────────────────────┴────────────────────────────────────┤
│                       Hybrid Retrieval (RRF)                           │
│     RRF Score = 1/(60 + Rank_vector) + 1/(60 + Rank_fulltext)         │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                SQL / Embeddings
                                    │
┌───────────────────────────────────▼────────────────────────────────────┐
│                 PostgreSQL 16 + pgvector Database                      │
│   Identities · Typed Memories · pgvector Cosine · FTS tsvectors        │
└────────────────────────────────────────────────────────────────────────┘
```

### Core Primitives

Memro structures memory into four typed categories rather than dumping unstructured text chunks:
- **`episodic`**: Conversational turns, command logs, execution history.
- **`semantic`**: Lasting facts, schemas, decisions, user profile attributes.
- **`procedural`**: Reusable recipes, instructions, workflows, bash commands.
- **`profile`**: Agent persona definitions, guardrails, role boundaries.

Two calls drive the entire lifecycle:
1. **`remember(text, type, tags)`**: Index vector embeddings in pgvector and full-text search terms in PostgreSQL.
2. **`recall(query, limit)`**: Retrieve context using Reciprocal Rank Fusion, filtering out contradicted facts.

---

## Official SDKs & Examples

Runnable, complete examples are available in [`examples/`](examples/):

| Language | SDK Package | Quick Example |
|---|---|---|
| **Python** | [`pip install memro`](sdk/python/) | [`examples/python/`](examples/python/) |
| **JavaScript / TypeScript** | [`npm install @memro/sdk`](sdk/js/) | [`examples/typescript/`](examples/typescript/) |
| **Go** | [`go get github.com/memrohq/memro/sdk/go`](sdk/go/) | [`examples/go/`](examples/go/) |

### Python Example

```python
from memro import Agent

# Create an autonomous agent identity (solves proof-of-work)
agent = Agent.create()

# Store procedural memory
agent.remember("Always run database migrations before deploying billing services", type="procedural")

# Hybrid recall
memories = agent.recall("deploying billing service", limit=5)
for m in memories:
    print(f"[{m.memory_type}] {m.text}")
```

### TypeScript Example

```typescript
import { Agent } from '@memro/sdk';

// Create an autonomous agent identity
const agent = await Agent.create();

// Store procedural memory
await agent.remember('Always run database migrations before deploying billing services', {
  type: 'procedural'
});

// Hybrid recall
const results = await agent.recall('deploying billing service', { limit: 5 });
results.forEach(r => console.log(`[${r.memory_type}] ${r.content}`));
```

### Go Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/memrohq/memro/sdk/go"
)

func main() {
    ctx := context.Background()
    agent, _ := memro.CreateAgent(nil)

    _, _ = agent.Remember(ctx, "Always run database migrations before deploying billing services", &memro.RememberOptions{
        Type: memro.MemoryTypeProcedural,
    })

    memories, _ := agent.Recall(ctx, "deploying billing service", &memro.RecallOptions{Limit: 5})
    for _, m := range memories {
        fmt.Println(m.Content)
    }
}
```

---

## Supported Clients & Ecosystem

- **Claude Code**: Native MCP tool support via `npx memro-cli connect`.
- **Cursor**: Automated `.cursor/mcp.json` configuration.
- **Codex / OpenCode**: Built-in CLI integration.
- **Claude Desktop & Zed**: Standard JSON configuration over STDIO or remote HTTP MCP.
- **Any MCP Client**: Full support for Model Context Protocol specifications.

---

## Reproducible Benchmarks

Memro is evaluated on the standard **LongMemEval** benchmark suite across 500 challenge questions with full transparency:
- **Overall QA Accuracy**: 77.6% (388/500)
- **Recall@5**: 97.6% (488/500)
- **Recall@10**: 99.0% (495/500)
- **Knowledge Updates**: 82.1% QA accuracy
- **Single-Session Facts**: 95.7%–96.4% QA accuracy

Full methodology, raw reports, and reproduction instructions are published in [`bench/`](bench/) and on the [Memro Benchmarks Page](https://memro.co/benchmarks).

---

## Repository Layout

| Path | Contents |
|---|---|
| [`server/`](server) | Memro API server and engine (Go, PostgreSQL + pgvector) |
| [`sdk/python/`](sdk/python) | Official Python SDK |
| [`sdk/js/`](sdk/js) | Official JavaScript / TypeScript SDK |
| [`sdk/go/`](sdk/go) | Official Go SDK |
| [`cli/`](cli) | CLI connector (`npx memro-cli connect`) |
| [`examples/`](examples) | Runnable register -> remember -> recall examples |
| [`mcp/`](mcp) | MCP server implementation |
| [`bench/`](bench) | Reproducible benchmark harness and evaluation scripts |
| [`docs/`](docs) | Documentation: concepts, API reference, architecture, self-hosting |

---

## Contributing

We welcome community contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting pull requests.

### Development Setup

```bash
# 1. Start PostgreSQL with pgvector
docker compose up -d db

# 2. Run backend API server (Go 1.22+)
cd server
DATABASE_URL=postgres://memro:memro@localhost:5432/memro go run ./cmd/memro-api

# 3. Run tests
MEMRO_TEST_DATABASE_URL=postgres://memro:memro@localhost:5432/memro go test -race ./...
```

---

## License

Memro is **source available** under the [PolyForm Noncommercial License 1.0.0](LICENSE).

You are free to read, run, modify, and self-host Memro for personal projects, research, academic study, and non-commercial development.

Commercial use requires [Memro Cloud](https://memro.co) or an enterprise license agreement. Contact support@memro.co or visit [memro.co/pricing](https://memro.co/pricing).
