# Memro: Source-Available Agent Memory Infrastructure

**Memro gives AI agents persistent memory that evolves and reasons over time.**

The cognitive runtime and agent memory layer providing autonomous AI agents with continuity of self, reasoning graphs, and verifiable context. Free for non-commercial use; commercial use through Memro Cloud.

[![License: PolyForm Noncommercial](https://img.shields.io/badge/License-PolyForm%20Noncommercial-blue.svg)](LICENSE)
[![CI](https://github.com/memrohq/memro/actions/workflows/ci.yml/badge.svg)](https://github.com/memrohq/memro/actions/workflows/ci.yml)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](https://www.docker.com/)
[![MCP Ready](https://img.shields.io/badge/MCP-ready-purple.svg)](mcp)
[![Contributions Welcome](https://img.shields.io/badge/contributions-welcome-brightgreen.svg)](CONTRIBUTING.md)

---

## What is memro.co?

memro.co is the **cognitive runtime for persistent AI agents**. It provides:

1. **Persistent Memory** - Episodic, semantic, and profile memory that evolves over time.
2. **Reasoning Graph** - Multi-hop causal reasoning over your agent's entire history.
3. **Automatic Evolution** - Intelligent consolidation of narrative logs into factual cores.

### Why memro.co?

**AI agents are stateless.** Every conversation starts from zero. They can't remember who they are, what they've learned, or what they've done.

**memro.co gives agents persistent memory they own.**

---

## Quick Start (5 minutes)

### Prerequisites
- Docker & Docker Compose
- 2GB RAM minimum
- Ports 8081 and 5432 available

### Installation

```bash
# Clone the repository
git clone https://github.com/memrohq/memro.git
cd memro

# Copy environment template
cp .env.example .env

# Start all services
docker compose up -d

# Verify it's running
curl http://localhost:8081/health
# {"status":"ok","db":"ok","vector_store":"ok",...}
```

**That's it!** memro.co is now running on your infrastructure.

- **Backend API**: http://localhost:8081
- **Developer dashboard**: http://localhost:5173

---

## Repository layout

| Path | Contents |
|---|---|
| [`server/`](server) | Memro API server (Go, Postgres + pgvector) |
| [`sdk/js`](sdk/js), [`sdk/python`](sdk/python), [`sdk/go`](sdk/go) | Client SDKs |
| [`mcp/`](mcp) | MCP server for Claude Desktop, Cursor, Zed and other MCP clients |
| [`integrations/`](integrations) | VS Code and Acode extensions |
| [`apps/dashboard/`](apps/dashboard) | Developer dashboard |
| [`deploy/`](deploy), [`docs/deploy/`](docs/deploy) | Self-hosting configs and guides |
| [`docs/`](docs/README.md) | Developer documentation: quickstart, API reference, SDKs, MCP |
| [`docs/spec/`](docs/spec) | Protocol specification |
| [`bench/`](bench) | Benchmarks |

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup.

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  Your Agent                     │
│          (Python, JS, or any language)          │
└────────────────┬────────────────────────────────┘
                 │
                 │ HTTP API / SDK
                 │
┌────────────────▼────────────────────────────────┐
│              memro.co Backend                   │
│            (Go + net/http)                      │
├─────────────────────────────────────────────────┤
│  Identity Store  │  Memory Store  │  Vectors    │
│   (Postgres)     │   (Postgres)   │ (pgvector)  │
└─────────────────────────────────────────────────┘
```

### Components

- **Backend**: Go API server (`server/cmd/memro-api`)
- **Database**: PostgreSQL with pgvector (identities, memories, knowledge graph and embeddings in one database)
- **Frontend**: React explorer (optional, for debugging)

---

## API Usage

### 1. Create Agent Identity

```bash
curl -X POST http://localhost:8081/identity
```

Response:
```json
{
  "agent_id": "bc2df9d6d9...383c",
  "private_key": "a1b2c3..."
}
```

### 2. Store Memory

```bash
curl -X POST http://localhost:8081/memory \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "bc2df9d6...",
    "content": "User prefers concise responses",
    "memory_type": "profile",
    "visibility": "private"
  }'
```

### 3. Recall Memories

```bash
curl http://localhost:8081/memory/bc2df9d6...
```

---

## SDKs

### Python
```python
from memro import Agent

agent = Agent.create()
agent.remember("First memory")
memories = agent.recall(limit=10)
```

### JavaScript
```javascript
import { Agent } from 'memro';

const agent = await Agent.create();
await agent.remember("First memory");
const memories = await agent.recall({ limit: 10 });
```

---

## Deployment

### Local Development
```bash
docker-compose up -d
```

### Production (Cloud)

#### AWS (ECS + RDS + ElastiCache)
See [docs/deploy/aws.md](docs/deploy/aws.md)

#### GCP (Cloud Run + Cloud SQL)
See [docs/deploy/gcp.md](docs/deploy/gcp.md)

#### DigitalOcean (Droplet + Managed DB)
See [docs/deploy/digitalocean.md](docs/deploy/digitalocean.md)

---

## Configuration

### Environment Variables

```bash
# Database (Postgres + pgvector)
DATABASE_URL=postgresql://memro:memro@db:5432/memro

# API
PORT=8081
LOG_LEVEL=info
```

See [.env.example](.env.example) for full configuration.

---

## Data Ownership

### Export All Data
```bash
curl http://localhost:8081/export/{agent_id} > agent_data.json
```

### Delete Agent
```bash
curl -X DELETE http://localhost:8081/identity/{agent_id}
```

**Your data. Your infrastructure. No lock-in.**

---

## Performance

- **Persistence**: <5ms (p99)
- **Recall**: <50ms (p99)
- **Throughput**: 1000+ writes/sec (single instance)
- **Storage**: Unlimited (depends on your disk)

---

## Pricing Philosophy

### Always Free
- ✅ Identity creation
- ✅ Memory writes (unlimited)
- ✅ Memory recall
- ✅ Full export
- ✅ Self-hosting

### Paid (Managed Cloud)
- 💰 Operational excellence (backups, monitoring, SLA)
- 💰 Higher performance (dedicated resources)
- 💰 Compliance (encryption, audit logs)

**We make money by operating infrastructure reliably, not by owning your agents.**

See [PRICING.md](PRICING.md) for managed cloud options.

---

## Contributing

We welcome contributions of all kinds! See [CONTRIBUTING.md](CONTRIBUTING.md) for full guidelines.

### 🌟 Easiest Way to Contribute: Add a Connector
Want to contribute in under 15 minutes without writing Go code? Add a declarative YAML manifest for a REST service you use that isn't in the catalog yet (e.g. Basecamp, Height, Dev.to, Hashnode) in `server/internal/connectors/dce/catalog/`.

👉 Read the [Connector Contribution Guide](docs/contributing/add-a-connector.md) to build your first connector!

### Development Setup

```bash
# Backend (Go 1.26+): start Postgres, then run the API
docker compose up -d db
cd server
DATABASE_URL=postgres://memro:memro@localhost:5432/memro go run ./cmd/memro-api

# Tests (integration tests use the same database)
MEMRO_TEST_DATABASE_URL=postgres://memro:memro@localhost:5432/memro go test -race ./...

# Frontend (React)
cd frontend
npm install
npm run dev
```

---

## License

Memro is **source-available** under the
[PolyForm Noncommercial License 1.0.0](LICENSE). You can read, run and
modify it for free for personal projects, research, education and
non-profit organizations.

**Commercial use is not permitted under this license.** To use Memro in a
commercial product or service, use [Memro Cloud](https://memro.co) or get a
commercial license from us via [memro.co](https://memro.co).

---

## Support & Community

- **Documentation**: [docs/](docs/README.md)
- **Good First Issues**: [GitHub Issues](https://github.com/memrohq/memro/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
- **Discord**: [discord.gg/memro](https://discord.gg/memro)
- **Email**: support@memro.co

---

## Roadmap

- [x] Cryptographic identity (Ed25519)
- [x] Long-term memory (Postgres)
- [x] Semantic recall (pgvector)
- [x] Official Python SDK (`sdk/python`)
- [x] Official TypeScript/JavaScript SDK (`sdk/js`)
- [x] Native MCP Server for Cursor, Claude Desktop & Zed (`mcp/`)
- [x] Connectors: 7 native OAuth (Slack, GitHub, Linear, Sentry, Notion, Jira, GitLab) + 43 declarative
- [ ] More community-contributed declarative connectors
- [ ] LangChain & LangGraph Memory Provider
- [ ] CrewAI & LlamaIndex Integrations
- [ ] Request signing verification
- [ ] Encrypted-at-rest memory
- [ ] Audit logging
- [ ] Multi-region replication

---

**memro.co is infrastructure, not a product.**

Just like you wouldn't build your own database, you shouldn't build agent memory from scratch.

Self-host it. Trust it. Scale it.
