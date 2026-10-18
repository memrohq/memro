# Contributing to Memro

Thanks for helping build Memro! This repository holds everything that is
open source: the server, SDKs, MCP server, integrations, deployment configs
and docs.

## Repository layout

| Path | What it is | Language |
|---|---|---|
| `server/` | Memro API server (memory engine, Postgres + pgvector) | Go |
| `sdk/js`, `sdk/python`, `sdk/go` | Client SDKs | TypeScript, Python, Go |
| `mcp/` | MCP server for Claude Desktop, Cursor, Zed and other MCP clients | Python |
| `integrations/vscode`, `integrations/acode` | Editor integrations | TypeScript, JavaScript |
| `apps/dashboard/` | Developer dashboard | React |
| `deploy/`, `docker-compose.yml` | Self-hosting configs | |
| `docs/` | Guides and the protocol spec (`docs/spec`) | Markdown |
| `bench/` | Benchmarks | Python |

## Development setup

### Server

Requires Go 1.26+ and Docker.

```bash
docker compose up -d db                       # Postgres with pgvector
make dev                                      # runs the API on :8081
make test                                     # unit + integration tests
```

Integration tests talk to a real database through
`MEMRO_TEST_DATABASE_URL`; without it they are skipped.

### SDKs

```bash
cd sdk/js && npm install && npm test
cd sdk/python && pip install -e ".[dev]" && pytest
cd sdk/go && go test ./...
```

### MCP server

```bash
pip install -e sdk/python -e mcp
MEMRO_BASE_URL=http://localhost:8081 memro-mcp
```

Docker images are built from the repository root, e.g.
`docker build -f mcp/Dockerfile .`.

## Making changes

1. Open an issue first for anything larger than a small fix, so we can agree
   on the approach.
2. Keep pull requests focused on one change and include tests.
3. Run the tests for every component you touched.
4. Write commit messages that explain *why*, not just *what*.

API changes must stay backward compatible with released SDKs. New database
migrations go in `server/migrations/` as the next `NNNN_description.sql`;
never edit a migration that has been released.

## Contributor License Agreement

Before we can merge your first pull request, you need to sign the
[Contributor License Agreement](CLA.md). A bot will ask you to sign it on
the pull request; it only takes a moment and covers all future
contributions.

## Reporting security issues

Please don't open public issues for vulnerabilities; see
[SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions are licensed under the
[Apache License 2.0](LICENSE).
