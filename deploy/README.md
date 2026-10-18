# Deploying Memro

Memro is one Go service plus PostgreSQL with the pgvector extension.
Memories, the knowledge graph and embeddings all live in that database.

| Option | Files |
|---|---|
| Docker Compose (single machine) | [`../docker-compose.yml`](../docker-compose.yml) |
| Render (one-click blueprint) | [`render.yaml`](render.yaml) |
| Railway | [`railway.toml`](railway.toml) |
| AWS, GCP, DigitalOcean guides | [`../docs/deploy/`](../docs/deploy) |

## Requirements

- PostgreSQL 15+ with `pgvector` (the `pgvector/pgvector` image, Cloud SQL, RDS and most managed Postgres offer it)
- `OPENAI_API_KEY` or `VOYAGE_API_KEY` for semantic search (without one, Memro falls back to keyword search)

## Configuration

All configuration is through environment variables; see [`../.env.example`](../.env.example).
Behind a reverse proxy or load balancer, set `TRUST_PROXY_HEADERS=true` so rate limits use the real client IP.

## Backups

Embeddings are stored in Postgres, so a regular `pg_dump` (or your provider's snapshots) backs up everything.
Uploaded files live in `FILE_STORAGE_PATH`; back that volume up too.
