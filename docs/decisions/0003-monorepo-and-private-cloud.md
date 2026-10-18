# 0003. Public monorepo + private memro-cloud
Date: 2026-10-18 · Status: Accepted

## Context
Fifteen repositories (core, SDKs, MCP, infra, spec, plugins…) drifted apart.
We also need cloud-only code (billing, quotas, website, infrastructure).

## Decision
- `memrohq/memro` (public): server, SDKs, MCP, integrations, dashboard,
  deploy configs, docs. Histories were merged with `git filter-repo`.
- `memrohq/memro-cloud` (private): imports `github.com/memrohq/memro/server`
  as a Go module and extends it through `server/app` options (usage meter,
  quotas, extra routes, startup hooks). Never fork server code into it.

## Alternatives considered
- `ee/` folder in one repo (PostHog/GitLab style): mixes licenses in one tree.
- Separate repos per component: the drift we were escaping.

## Consequences
Self-hosted and cloud run the same engine. Cloud pins a server commit in
go.mod; the dashboard image is built from the same pinned commit.
