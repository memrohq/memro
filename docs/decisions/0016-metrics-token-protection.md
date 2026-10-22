# 0016. Metrics endpoint token protection
Date: 2026-10-22 · Status: Accepted

## Context
Prometheus metrics exposed at `GET /metrics` leak internal operational details (such as request counts, cache sizes, memory counts, and webhook statistics) to the public internet when exposed without an authentication layer or reverse-proxy filter.

## Decision
- Protect `GET /metrics` in core with an optional bearer token configured via `METRICS_TOKEN`.
- When `METRICS_TOKEN` is set, requests must supply `Authorization: Bearer <METRICS_TOKEN>`. Unauthorized requests are rejected with HTTP 401 and `WWW-Authenticate: Bearer realm="metrics"`.
- When `METRICS_TOKEN` is empty (the self-hosting default), `/metrics` remains accessible without authentication and the server logs a warning at startup.
- In multi-tenant cloud environments (`memro-cloud`), the edge reverse proxy (Caddy) returns 404 for `/metrics` requests on public hosts.

## Alternatives considered
- Moving metrics to a separate internal listener port: complicates single-container and serverless deployments for self-hosters.
- Leaving metrics unconditionally open in core: relies entirely on external reverse proxies and risks exposure if a proxy is misconfigured or bypassed.

## Consequences
- Scraping systems (Prometheus, Grafana Agent) must provide `Authorization: Bearer <METRICS_TOKEN>` when `METRICS_TOKEN` is configured.
- Existing self-hosted deployments continue working without changes, with an explicit startup warning advising token configuration.
