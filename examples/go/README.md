# Memro Go SDK Example

Complete runnable example demonstrating:
1. Sovereign Ed25519 identity creation (with proof-of-work registration) or API key connection
2. Storing a typed procedural memory
3. Hybrid context recall (vector + full-text search fused via RRF)

## Quick Run (< 2 minutes)

```bash
# 1. Run with Go
go run .

# Optional: Run against local self-hosted instance (docker compose up -d)
MEMRO_BASE_URL=http://localhost:8081 go run .

# Optional: Use an API key from npx memro-cli connect or dashboard
MEMRO_API_KEY=memro_sk_... go run .
```
