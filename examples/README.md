# Memro SDK Examples

Runnable, end-to-end examples demonstrating sovereign agent registration, typed memory storage, and hybrid recall across all official SDKs.

Each example executes a complete lifecycle in < 5 seconds:
1. **Register**: Creates a cryptographic Ed25519 identity (with CPU proof-of-work) or connects with an existing API key.
2. **Remember**: Stores a typed observation (`procedural`, `semantic`, `episodic`, or `profile`).
3. **Recall**: Performs hybrid retrieval (dense vector embeddings + PostgreSQL full-text search fused via Reciprocal Rank Fusion).

## Quick Start per Language

| Language | Directory | Command | Time to Run |
|---|---|---|---|
| **Python** | [`examples/python/`](python/) | `python register_remember_recall.py` | ~3 seconds |
| **JavaScript / TypeScript** | [`examples/typescript/`](typescript/) | `node index.js` | ~3 seconds |
| **Go** | [`examples/go/`](go/) | `go run .` | ~2 seconds |

## Connecting to Cloud vs. Self-Hosted

By default, all examples connect to the public Memro Cloud API at `https://api.memro.co`.

To run against a local self-hosted Memro instance:
```bash
# Start local PostgreSQL + Memro instance
docker compose up -d

# Point examples to local backend
export MEMRO_BASE_URL="http://localhost:8081"
```

To run with an authenticated developer API key (from `npx memro-cli connect` or the [Memro Dashboard](https://app.memro.co)):
```bash
export MEMRO_API_KEY="memro_sk_..."
```
