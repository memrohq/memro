# Self-hosting

Memro is one Go service plus PostgreSQL with pgvector. The dashboard and the
MCP server are optional. Self-hosting is free for non-commercial use under the
[PolyForm Noncommercial License](../LICENSE); commercial products use Memro
Cloud.

## Run it

```bash
git clone https://github.com/memrohq/memro && cd memro
cp .env.example .env              # add OPENAI_API_KEY or VOYAGE_API_KEY for semantic search
docker compose up -d              # API on :8081, dashboard on :5173, Postgres on 127.0.0.1:5432
curl localhost:8081/health
```

The MCP server isn't in the compose file. Run it next to the API:

```bash
pip install -e sdk/python -e mcp
MEMRO_BASE_URL=http://localhost:8081 MCP_TRANSPORT=sse memro-mcp     # :8080/mcp
```

Connect your tools to it:

```bash
node cli/bin/memro.js connect --api http://localhost:8081
```

For production hosting (Render, Railway, AWS, GCP, DigitalOcean), backups and
reverse-proxy settings, see [deploy/README.md](../deploy/README.md).

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
| `ALLOW_LEGACY_SIGNATURES` | `true` | Accept v1 signatures; set `false` once your clients use v2 |
| `RATE_LIMIT_MAX`, `RATE_LIMIT_WINDOW_SECONDS` | `200`, `60` | |
| `MEMORY_SOFT_CAP` | `1000` | Memories per agent |
| `IDENTITY_POW_BITS` | `20` | Proof-of-work difficulty for agent registration (about a second of CPU) |
| `IDENTITY_POW_REQUIRED` | `false` | Reject registrations without a proof; turn on once your clients send one |
| `UNCLAIMED_MAX_MEMORIES`, `UNCLAIMED_MAX_FILE_BYTES` | `0`, `0` (no cap) | Caps for agents with no owner. A quota policy that implements `extension.UnclaimedCapExemption` can lift them per agent |
| `UNCLAIMED_CAP_HINT` | | Added to the cap message; `{agent_id}` is replaced |
| `METRICS_TOKEN` | | Protects `GET /metrics` with Bearer auth; open when unset (logs a warning) |
| `MAX_BODY_BYTES` | `10000000` | |
| `FILE_STORAGE_PATH` | `./data/files` | Uploaded files; back this up |
| `EMAIL_HOST`, `EMAIL_PORT`, `EMAIL_HOST_USER`, `EMAIL_HOST_PASSWORD`, `EMAIL_FROM` | | SMTP for dashboard sign-in codes |
| `DEV_LOG_OTP` | `false` | Also log sign-in codes; development only |
| `MIGRATE_ON_STARTUP` | `true` | Apply database migrations at start |
| `CONNECTOR_ENCRYPTION_KEY`, `SLACK_*`, `GITHUB_*` | | Slack and GitHub connectors; see [Connectors](connectors.md#setting-up-the-connectors-self-hosting) |

The full list with comments is in [.env.example](../.env.example).
