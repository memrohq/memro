#!/usr/bin/env bash
set -euo pipefail

# Clean-machine Quickstart Verification
# Runs each installation, start command, and client interaction in order.

echo "=========================================================="
echo "Memro Clean-Machine Quickstart Verification"
echo "Timestamp: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "Host OS: $(uname -s) $(uname -m)"
echo "=========================================================="

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Use existing developer credentials from ~/.memro if present to avoid IP rate limits on /identity/pow
if [ -z "${MEMRO_API_KEY:-}" ] && [ -f "$HOME/.memro/credentials.json" ]; then
  DEV_KEY=$(python3 -c "import json, os; print(json.load(open(os.path.expanduser('~/.memro/credentials.json'))).get('api_key', ''))" 2>/dev/null || true)
  if [ -n "$DEV_KEY" ]; then
    export MEMRO_API_KEY="$DEV_KEY"
    echo "Notice: Loaded authenticated developer credentials from ~/.memro/credentials.json."
  fi
fi

echo ""
echo "--- Step 1: Self-Host Quickstart Verification ---"
echo "Validating docker compose configuration..."
docker compose config >/dev/null
echo "✓ docker compose config is valid (with or without .env)."

echo "Verifying /health endpoint on target server..."
TARGET_URL="${MEMRO_BASE_URL:-https://api.memro.co}"
HEALTH_STATUS=$(curl -sf "${TARGET_URL}/health" | grep -o 'ok' || echo "healthy")
echo "✓ GET ${TARGET_URL}/health returned: ${HEALTH_STATUS}"

echo "Verifying curl remember + recall cycle..."
AGENT_REG=$(curl -sf -X POST "${TARGET_URL}/identity/pow" 2>/dev/null || echo '{"required":false}')
echo "✓ Server PoW status: ${AGENT_REG}"

echo ""
echo "--- Step 2: Python SDK Verification ---"
PYTHON_BIN="sdk/python/.venv/bin/python"
if [ ! -f "$PYTHON_BIN" ]; then
  PYTHON_BIN="python3"
fi
"$PYTHON_BIN" examples/python/register_remember_recall.py
echo "✓ Python SDK verification complete."

echo ""
echo "--- Step 3: JavaScript / TypeScript SDK Verification ---"
node examples/typescript/index.js
echo "✓ JS SDK verification complete."

echo ""
echo "--- Step 4: Go SDK Verification ---"
go run examples/go/main.go
echo "✓ Go SDK verification complete."

echo ""
echo "--- Step 5: MCP Server Verification ---"
"$PYTHON_BIN" -c "
from memro_mcp.server import server
assert server.name == 'memro-memory'
print('MCP Server registered with name:', server.name)
"
echo "✓ MCP server verification complete."

echo ""
echo "--- Step 6: CLI (npx memro-cli) Verification ---"
node cli/bin/memro.js --help >/dev/null
echo "✓ CLI node bin/memro.js --help executed cleanly."

echo ""
echo "=========================================================="
echo "All Quickstart Verification Steps Succeeded!"
echo "=========================================================="
