# Memro: rules your AI coding agents actually follow

**Tell one agent a rule once; every agent follows it.** Say "never use `Intl` in cart code" to Claude Code, and Codex, Cursor, Gemini CLI and OpenCode see it at every prompt, on every clone, and get a warning when an edit breaks it.

```bash
npx memro-cli init      # no account: rules in .memro/rules.md, hooks in every agent tool
npx memro-cli connect   # optional: shared memory and coordination across machines and teammates
```

- **Caught as you say it:** "never…", "always…", "from now on…", "don't use…" in a prompt is saved to `.memro/rules.md` in your words. See it in `git diff`, fix it by editing a line, share it by committing.
- **Shown to every agent, every prompt:** no search to miss it. In our [benchmark](docs/benchmarks/journal/2026-11-19-relay-rule-depth.md), with the rule buried among 3,000 facts, agents shown the rules this way avoided the trap 150/150 times; a notes file they were told to read, 139/150 (not a significant difference at our pre-set bar); search alone, 58/150.
- **Checked on every edit:** name the thing to avoid ("never use Intl in cart code", or in backticks for anything longer than a word) and an edit or command that uses it gets a warning with the rule (Claude Code and Codex; warn only, never blocks).
- **Memory and coordination too:** with an account, agents share long-term memory across tools and machines, and claim files so two agents don't edit one at once. Sync Slack, GitHub, Linear, Notion and [40+ more](https://memro.co/docs/connectors.html), hosted or self-hosted with Docker.

<p align="center">
  <img src="docs/assets/memro-aha.gif" width="720" alt="Cursor forgets the deploy day decided in Claude Code; with Memro, a new Cursor session recalls it: Tuesdays, decided with Claude Code on Monday.">
</p>

```bash
npx memro-cli rules     # list this repo's rules; rules edit <n> <text>, rules delete <n>
```

⭐ **If Memro saves you from re-explaining your project, star the repo (top right): it helps other developers find it.**

<p align="center">
  <a href="https://memro.co">Website</a> ·
  <a href="https://memro.co/docs/">Docs</a> ·
  <a href="https://discord.gg/ykTeRFRpQP">Discord</a> ·
  <a href="https://memro.co/demo/voice/">Voice demo</a> ·
  <a href="https://dev.to/subal_bain_059153c5cbc1cc/i-gave-claude-code-codex-cursor-and-copilot-one-shared-memory-heres-what-broke-3g56">What broke wiring one memory into 4 AI tools</a>
</p>

[![npm](https://img.shields.io/npm/v/memro-cli?label=memro-cli)](https://www.npmjs.com/package/memro-cli)
[![PyPI](https://img.shields.io/pypi/v/memro?label=pypi%20memro)](https://pypi.org/project/memro/)
[![CI](https://github.com/memrohq/memro/actions/workflows/ci.yml/badge.svg)](https://github.com/memrohq/memro/actions/workflows/ci.yml)
[![Discord](https://img.shields.io/badge/Discord-join-5865F2?logo=discord&logoColor=white)](https://discord.gg/ykTeRFRpQP)
[![License: PolyForm Noncommercial](https://img.shields.io/badge/License-PolyForm%20Noncommercial-blue.svg)](LICENSE)

**Source-available:** free for personal, research and other non-commercial use, including self-hosting; commercial use runs on [Memro Cloud](https://memro.co/pricing/) or a commercial license.

<details>
<summary>License details</summary>

Memro is licensed under the [PolyForm Noncommercial License 1.0.0](LICENSE).
- **Free for non-commercial use:** personal study, experiments, hobby projects, research, and testing without commercial application.
- **Commercial use:** using Memro for commercial products or production services requires a managed subscription on [Memro Cloud](https://memro.co) or an enterprise commercial license.
- **Getting a commercial license:** see [memro.co/pricing](https://memro.co/pricing) or contact `subal@memro.co`.

`github.com/memrohq/memro` is the single public repository for Memro: the Go server, official SDKs (Python, JavaScript/TypeScript, Go), the MCP server and the CLI.
</details>

---

## Claude Code plugin

```
/plugin install memro --marketplace memrohq/memro
```

Adds Memro's MCP server and recalls the project's memory at the start of every session. See [integrations/claude-code](integrations/claude-code/).

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

## See your memory

<p align="center">
  <a href="https://memro.co/#graph"><img src="docs/assets/memro-graph.gif" width="720" alt="The Memro dashboard's memory graph: a million memories, linked by the people, companies and conversations in them"></a>
  <br/>
  <sub>The dashboard's memory graph on an account with a million memories; it draws the newest 5,000 at most at a time. <a href="https://memro.co/#graph">Watch the tour with sound</a>.</sub>
</p>

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

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co) · Community: [Discord](https://discord.gg/ykTeRFRpQP)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use requires Memro Cloud or a commercial license.
