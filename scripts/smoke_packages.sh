#!/usr/bin/env bash
set -euo pipefail

# Memro Package Smoke Verification Test Suite
# Tests: Python SDK, JS SDK, Go SDK, MCP Server, and CLI
# Verifies full register -> remember -> recall flow against a live Memro server.

MEMRO_URL="${MEMRO_BASE_URL:-http://localhost:8081}"

echo "=========================================================="
echo "Memro Smoke Test: Register -> Remember -> Recall"
echo "Target Base URL: $MEMRO_URL"
echo "=========================================================="

# Check if target server is reachable; if local fails, fall back to api.memro.co
if ! curl -sf "${MEMRO_URL}/health" >/dev/null 2>&1; then
  echo "Notice: Local server at ${MEMRO_URL} not responding. Testing against https://api.memro.co..."
  MEMRO_URL="https://api.memro.co"
  if ! curl -sf "${MEMRO_URL}/health" >/dev/null 2>&1; then
    echo "Error: Neither local server nor https://api.memro.co is reachable."
    exit 1
  fi
fi

export MEMRO_BASE_URL="$MEMRO_URL"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

echo ""
echo "1. Smoke testing Python SDK..."
if [ -f "sdk/python/.venv/bin/python" ]; then
  PYTHON_BIN="sdk/python/.venv/bin/python"
else
  PYTHON_BIN="python3"
fi
"$PYTHON_BIN" examples/python/register_remember_recall.py
echo "✓ Python SDK smoke test passed."

echo ""
echo "2. Smoke testing JavaScript / TypeScript SDK..."
node examples/typescript/index.js
echo "✓ JS SDK smoke test passed."

echo ""
echo "3. Smoke testing Go SDK..."
go run examples/go/main.go
echo "✓ Go SDK smoke test passed."

echo ""
echo "4. Smoke testing MCP Server..."
"$PYTHON_BIN" -c "
import sys
from memro_mcp.server import server
assert server.name == 'memro-memory', f'Unexpected server name: {server.name}'
print(f'MCP server instance {server.name} initialized successfully.')
"
echo "✓ MCP server smoke test passed."

echo ""
echo "5. Smoke testing Memro CLI..."
node cli/bin/memro.js --help >/dev/null
echo "✓ CLI smoke test passed."

echo ""
echo "=========================================================="
echo "All package smoke tests passed successfully!"
echo "=========================================================="
