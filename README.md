# memro.co

**The High-Performance Memory Layer Built for AI Agents**

Open-source memory infrastructure that gives AI agents continuity of self. Install with Docker, scale with managed cloud.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](https://www.docker.com/)

---

## What is memro.co?

memro.co is **infrastructure for persistent AI agents**. It provides three core primitives:

1. **Cryptographic Identity** - Ed25519 identity generation (public key = Agent ID)
2. **Long-Term Memory** - Append-only storage with episodic, semantic, and profile types
3. **Semantic Recall** - Vector-based similarity search (<50ms latency)

### Why memro.co?

**AI agents are stateless.** Every conversation starts from zero. They can't remember who they are, what they've learned, or what they've done.

**memro.co gives agents persistent memory they own.**

---

## Quick Start (5 minutes)

### Prerequisites
- Docker & Docker Compose
- 2GB RAM minimum
- Ports 8081, 5432, 6333 available

### Installation

```bash
# Clone the repository
git clone https://github.com/memro-co/memro.git
cd memro

# Copy environment template
cp .env.example .env

# Start all services
docker-compose up -d

# Verify it's running
curl http://localhost:8081/health
# {"status":"ok"}
```

**That's it!** memro.co is now running on your infrastructure.

- **Backend API**: http://localhost:8081
- **Infrastructure Explorer**: http://localhost:5174

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
│         (Rust + Axum + Hexagonal)               │
├─────────────────────────────────────────────────┤
│  Identity Store  │  Memory Store  │  Vector DB  │
│   (Postgres)     │   (Postgres)   │  (Qdrant)   │
└─────────────────────────────────────────────────┘
```

### Components

- **Backend**: Rust API server (Axum framework)
- **Database**: PostgreSQL (identity + memory metadata)
- **Vector Store**: Qdrant (semantic search)
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
# Database
DATABASE_URL=postgresql://memro:memro@postgres:5432/memro

# Vector Store
QDRANT_URL=http://qdrant:6333

# API
PORT=8081
RUST_LOG=info
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
# Backend (Rust)
cd backend
cargo run

# Frontend (React)
cd frontend
npm install
npm run dev
```

---

## License

MIT License - see [LICENSE](LICENSE)

---

## Support

- **Documentation**: [docs.memro.co](https://docs.memro.co)
- **GitHub Issues**: [github.com/memro-co/memro/issues](https://github.com/memro-co/memro/issues)
- **Discord**: [discord.gg/memro](https://discord.gg/memro)
- **Email**: support@memro.co

---

## Roadmap

- [x] Cryptographic identity (Ed25519)
- [x] Long-term memory (Postgres)
- [x] Semantic recall (Qdrant)
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
# memro-core
