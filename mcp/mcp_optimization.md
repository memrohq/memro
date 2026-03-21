# Memro MCP: Production Optimization Guide

To move beyond the developer preview and ensure a high-performance, production-ready memory connection, follow these optimization strategies.

## 1. Eliminate Bridge Latency
Currently, we use `npx supergateway`. This periodically checks the npm registry, adding ~200ms of overhead to the initial connection.

**Optimized Fix:** Install `supergateway` once and use the direct binary.
```bash
npm install -g supergateway
# In claude_desktop_config.json, use:
"command": "supergateway",
"args": ["--sse", "http://localhost:8084/sse/..."]
```

## 2. High-Throughput ASGI
The current `uvicorn` setup is single-worker. For multi-agent support or high-frequency memory updates, use `uvloop` (C-based event loop) and multiple workers.

**In Dockerfile/Compose:**
```yaml
command: >
  uvicorn memro_mcp.server:starlette_app 
  --host 0.0.0.0 
  --port 8080 
  --workers 4 
  --loop uvloop
```

## 3. Cognitive Optimization: Atomic Facts
Memro's reasoning engine is powerful but has overhead. For simple data (names, preferences), always use `is_atomic=True`.

**LLM Prompting Strategy:**
"If you are storing a simple fact (e.g., 'The user is Subal'), always set `is_atomic: true` in the `remember` tool call. This skips the expensive graph-synthesis and stores it as a direct primitive."

## 4. Connection Stability: Heartbeats
We've already implemented the 15s heartbeat. For production, ensure your reverse proxy (Nginx/Cloudflare) has a `proxy_read_timeout` greater than 30s to avoid dropping the SSE stream.

## 5. Security: Secret Management
Never put tokens directly in `claude_desktop_config.json` for production. Use environment variable injection:
```json
"env": {
  "MEMRO_TOKEN": "..."
}
```
Then use `&token=$MEMRO_TOKEN` in the URL (depending on your shell's expansion capabilities).
