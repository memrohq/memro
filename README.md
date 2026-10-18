# memro.co

**Memro gives AI agents memory that evolves and reasons over time.**

Open-source memory infrastructure that gives AI agents continuity of self. Install with Docker, scale with managed cloud.

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![CI](https://github.com/memrohq/memro/actions/workflows/ci.yml/badge.svg)](https://github.com/memrohq/memro/actions/workflows/ci.yml)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](https://www.docker.com/)

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

## SDKs (Coming Soon)

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

#### Kubernetes
See [docs/deploy/kubernetes.md](docs/deploy/kubernetes.md)

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

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md).

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

Apache License 2.0 - see [LICENSE](LICENSE)

---

## Support

- **Documentation**: [docs.memro.co](https://docs.memro.co)
- **GitHub Issues**: [github.com/memrohq/memro/issues](https://github.com/memrohq/memro/issues)
- **Discord**: [discord.gg/memro](https://discord.gg/memro)
- **Email**: support@memro.co

---

## Roadmap

- [x] Cryptographic identity (Ed25519)
- [x] Long-term memory (Postgres)
- [x] Semantic recall (pgvector)
- [ ] Python SDK
- [ ] JavaScript SDK
- [ ] Request signing verification
- [ ] Encrypted-at-rest memory
- [ ] Audit logging
- [ ] Multi-region replication

---

**memro.co is infrastructure, not a product.**

Just like you wouldn't build your own database, you shouldn't build agent memory from scratch.

Self-host it. Trust it. Scale it.
