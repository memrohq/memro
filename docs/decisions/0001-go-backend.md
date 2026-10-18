# 0001. Rewrite the backend in Go
Date: 2026-10-18 · Status: Accepted

## Context
The API existed twice: a Rust (Axum) server with a separate Rust engine
crate, and a partial Go port. Both had diverged (e.g. different signing
payloads), and the Rust code had serious bugs found during the port:
unauthenticated identity takeover, missing ownership checks, a missing
`memories.metadata` column, a temporal search that always returned nothing.

## Decision
One Go server (`server/`) with full route parity, standard-library HTTP,
pgx, and the engine ported into `server/internal/engine`. All Rust code was
removed.

## Alternatives considered
- Keep Rust: slower iteration for the team, two codebases already drifting.
- Finish the partial Go port only: it covered ~a quarter of the routes.

## Consequences
Single binary, fast builds, simple deploys. Wire format kept compatible with
the SDKs (memory types are now lowercase, matching what clients expected).
