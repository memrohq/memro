# Self-hosting

Memro is one Go service plus PostgreSQL with pgvector. The dashboard and the
MCP server are optional. Self-hosting is free for non-commercial use under the
[PolyForm Noncommercial License](../LICENSE); commercial products use Memro
Cloud.

## Run it

### Option 1: Clone the repository (works today)

```bash
git clone https://github.com/memrohq/memro && cd memro
cp .env.example .env              # add OPENAI_API_KEY for hybrid search; set METRICS_TOKEN
docker compose up -d              # API on :8081, dashboard on :5173, Postgres on 127.0.0.1:55432
curl localhost:8081/health
```

The database host port is published on `127.0.0.1:55432` to avoid clashes with existing local Postgres instances on port 5432.

### Option 2: Prebuilt images (no clone required, after v0.1.0 release)

Run Memro in any empty directory without cloning the source tree:

```bash
curl -fsSL https://raw.githubusercontent.com/memrohq/memro/v0.1.0/docker-compose.release.yml -o docker-compose.yml && docker compose up -d
curl localhost:8081/health
```

> **Note:** The prebuilt one-liner downloads `docker-compose.release.yml` with pinned `ghcr.io/memrohq/memro:0.1.0` and `ghcr.io/memrohq/memro-dashboard:0.1.0` images, which become available once tag `v0.1.0` is published to GitHub Container Registry. All environment variables have sensible defaults and `.env` is optional. If `OPENAI_API_KEY` is not provided, the server starts normally and falls back to PostgreSQL keyword full-text search.

### Verifying Container Signatures

Memro releases sign both server and dashboard container images keylessly using Sigstore Cosign during GitHub Actions. Verify image integrity and provenance:

```bash
# Verify the Memro API server image
cosign verify ghcr.io/memrohq/memro:0.1.0 \
  --certificate-identity-regexp 'https://github.com/memrohq/memro/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

# Verify the Memro Dashboard image
cosign verify ghcr.io/memrohq/memro-dashboard:0.1.0 \
  --certificate-identity-regexp 'https://github.com/memrohq/memro/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Configuration

All settings are environment variables. The common ones:

| Variable | Default | |
|---|---|---|
| `DATABASE_URL` | | Postgres with pgvector, required |
| `PORT` | `8081` | |
| `BASE_URL` | `http://localhost:$PORT` | Public URL of the API, used in asset links |
| `APP_URL` | `http://localhost:5173` | Dashboard URL, used by device login |
| `MCP_URL` | `http://localhost:8080/mcp` | Returned to `memro connect` |
| `OPENAI_API_KEY` / `VOYAGE_API_KEY` | | Embeddings. OpenAI also enables image description and audio transcription |
| `EMBEDDING_PROVIDER`, `EMBEDDING_MODEL`, `EMBEDDING_BASE_URL` | `openai` | Any OpenAI-compatible endpoint works |
| `ALLOWED_ORIGINS` | all | Comma-separated CORS origins for browsers |
| `TRUST_PROXY_HEADERS` | `false` | Set behind a proxy that sets `X-Real-IP` |
| `ALLOW_LEGACY_SIGNATURES` | `false` | Accept legacy v1 signatures (disabled by default; set `true` only if older clients require it) |
| `RATE_LIMIT_MAX`, `RATE_LIMIT_WINDOW_SECONDS` | `200`, `60` | |
| `MEMORY_SOFT_CAP` | `1000` | Memories per agent |
| `IDENTITY_POW_BITS` | `20` | Proof-of-work difficulty for agent registration (about a second of CPU) |
| `IDENTITY_POW_REQUIRED` | `false` | Reject registrations without a proof; turn on once your clients send one |
| `UNCLAIMED_MAX_MEMORIES`, `UNCLAIMED_MAX_FILE_BYTES` | `0`, `0` (no cap) | Caps for agents with no owner. A quota policy that implements `extension.UnclaimedCapExemption` can lift them per agent |
| `UNCLAIMED_CAP_HINT` | | Added to the cap message; `{agent_id}` is replaced |
| `METRICS_TOKEN` | | Protects `GET /metrics` and `GET /stats/cache` with `Bearer <token>` (min 32 chars; scheme matched case-sensitively as Prometheus sends it); open when unset (logs a warning) |
| `MAX_BODY_BYTES` | `10000000` | |
| `FILE_STORAGE_PATH` | `./data/files` | Uploaded files; back this up |
| `EMAIL_HOST`, `EMAIL_PORT`, `EMAIL_HOST_USER`, `EMAIL_HOST_PASSWORD`, `EMAIL_FROM` | | SMTP for dashboard sign-in codes |
| `DEV_LOG_OTP` | `false` | Also log sign-in codes; development only |
| `MIGRATE_ON_STARTUP` | `true` | Apply database migrations at start |
| `CONNECTOR_ENCRYPTION_KEY`, `SLACK_*`, `GITHUB_*` | | Slack and GitHub connectors; see [Connectors](connectors.md#setting-up-the-connectors-self-hosting) |

The full list with comments is in [.env.example](../.env.example).
