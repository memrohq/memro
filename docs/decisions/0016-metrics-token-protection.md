# 0016. Metrics and cache stats endpoint token protection
Date: 2026-10-22 · Status: Accepted

## Context
Prometheus metrics exposed at `GET /metrics` and embedding cache statistics at `GET /stats/cache` leak internal operational details (such as request counts, cache sizes, hit rates, memory counts, and connector webhook metrics) to the public internet when exposed without an authentication layer or reverse-proxy filter.

## Decision
- Protect both `GET /metrics` and `GET /stats/cache` in core with an optional bearer token configured via `METRICS_TOKEN`.
- When `METRICS_TOKEN` is set, requests must supply `Authorization: Bearer <METRICS_TOKEN>`. The `"Bearer "` scheme prefix is matched case-sensitively (as sent by Prometheus) and compared in constant time (`subtle.ConstantTimeCompare`). Unauthorized requests are rejected with HTTP 401 and `WWW-Authenticate: Bearer realm="metrics"`.
- Minimum token length: When `METRICS_TOKEN` is set, it must be at least 32 characters long. Configurations with shorter tokens fail validation and refuse to start.
- Rate limiting: `/metrics` and `/stats/cache` are exempt from the per-IP rate limiter only when authenticated with a valid metrics token. Failed authentication attempts count against the IP rate limiter and eventually receive HTTP 429 (`Too Many Requests`), protecting the auth check against brute force.
- When `METRICS_TOKEN` is empty (the self-hosting default), both endpoints remain accessible without authentication and the server logs a warning at startup.
- In multi-tenant cloud environments (`memro-cloud`), defense-in-depth is enforced: the edge reverse proxy (Caddy) returns 404 for `/metrics*` and `/stats/cache*` on public hosts (`api.*` and `app.*/api/*`), while internal scraping through the private network authenticates with `METRICS_TOKEN`.

## Alternatives considered
- Moving metrics to a separate internal listener port: complicates single-container and serverless deployments for self-hosters.
- Leaving metrics unconditionally open in core: relies entirely on external reverse proxies and risks exposure if a proxy is misconfigured or bypassed.

## Consequences
- Scraping systems (Prometheus, Grafana Agent) and cache monitoring tools must provide `Authorization: Bearer <METRICS_TOKEN>` when `METRICS_TOKEN` is configured.
- Edge proxy configurations in `memro-cloud` block public ingress with HTTP 404, preventing external discovery or brute force attempts.
- Existing self-hosted deployments continue working without changes, with an explicit startup warning advising token configuration.
