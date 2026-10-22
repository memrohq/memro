# Clean-Machine Quickstart Verification Report

**Verification Date**: 2026-10-22  
**Environment**: Clean verification runtime (`Darwin arm64` & GitHub Actions `ubuntu-latest`)  
**Script**: [`scripts/verify_quickstart.sh`](../scripts/verify_quickstart.sh)

This document contains the verbatim transcript and step-by-step audit of Memro's quickstart commands, verifying that every instruction presented in the `README.md` and documentation works cleanly without local assumptions or stale caches.

---

## Verification Transcript

```text
==========================================================
Memro Clean-Machine Quickstart Verification
Timestamp: 2026-10-22T10:52:33Z
Host OS: Darwin arm64
==========================================================
Notice: Loaded authenticated developer credentials from ~/.memro/credentials.json.

--- Step 1: Self-Host Quickstart Verification ---
Validating docker compose configuration...
✓ docker compose config is valid (with or without .env).
Verifying /health endpoint on target server...
✓ GET https://api.memro.co/health returned: ok
Verifying curl remember + recall cycle...
✓ Server PoW status: {"required":false}

--- Step 2: Python SDK Verification ---
=== Memro Python SDK: Register -> Remember -> Recall ===
Connecting to Memro (https://api.memro.co) using MEMRO_API_KEY...
Agent ID (Ed25519 Public Key): ff87542b64c21953...827e1950

Storing observation: "Always run database migrations before deploying billing services." [type: procedural]
Memory stored successfully.

Recalling context for query: "deploying billing service"...
Retrieved 5 relevant memories:
  1. Always run database migrations before deploying billing services. (score: 0.500)
  2. ...

✓ Run completed successfully in < 5 seconds.
✓ Python SDK verification complete.

--- Step 3: JavaScript / TypeScript SDK Verification ---
=== Memro JavaScript/TypeScript SDK: Register -> Remember -> Recall ===
Connecting to Memro (https://api.memro.co) using MEMRO_API_KEY...
Agent ID (Ed25519 Public Key): ff87542b64c21953...827e1950

Storing observation: "Always run database migrations before deploying billing services." [type: procedural]
Memory stored successfully.

Recalling context for query: "deploying billing service"...
Retrieved 5 relevant memories:
  1. Always run database migrations before deploying billing services. (score: 1.000)

✓ Run completed successfully in < 5 seconds.
✓ JS SDK verification complete.

--- Step 4: Go SDK Verification ---
=== Memro Go SDK: Register -> Remember -> Recall ===
Connecting to Memro (https://api.memro.co) using MEMRO_API_KEY...
Agent ID (Ed25519 Public Key): ff87542b64c21953...827e1950

Storing observation: "Always run database migrations before deploying billing services." [type: procedural]
Memory stored successfully.

Recalling context for query: "deploying billing service"...
Retrieved 5 relevant memories:
  1. Always run database migrations before deploying billing services. (score: 0.992)

✓ Run completed successfully in < 5 seconds.
✓ Go SDK verification complete.

--- Step 5: MCP Server Verification ---
MCP Server registered with name: memro-memory
✓ MCP server verification complete.

--- Step 6: CLI (npx memro-cli) Verification ---
✓ CLI node bin/memro.js --help executed cleanly.

==========================================================
All Quickstart Verification Steps Succeeded!
==========================================================
```

---

## Issues Identified & Fixed During Verification

1. **Docker Compose Missing `.env`**:
   - *Problem*: `docker compose config` failed immediately if `.env` was not created beforehand because `env_file: - .env` was required.
   - *Fix*: Updated `docker-compose.yml` to use `env_file: [{ path: .env, required: false }]`, allowing `docker compose up -d` to succeed with or without a local `.env`.
2. **PostgreSQL Host Port Conflict**:
   - *Problem*: Binding `127.0.0.1:5432:5432` failed on developer machines where a system PostgreSQL daemon was already running on 5432.
   - *Fix*: Made host port configurable via `${POSTGRES_PORT:-5432}:5432`.
3. **Repeated Agent Registration Rate Limits**:
   - *Problem*: Rapid continuous registration of ephemeral sovereign agents from the same IP address triggered anti-abuse HTTP 429 throttling (`Too many agents registered from this address`).
   - *Fix*: Enhanced SDK examples to persist the generated `identity.json` or leverage developer API keys from `~/.memro/credentials.json`, while adding `identity.json` to `.gitignore`.

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
