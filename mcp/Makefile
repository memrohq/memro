# memro-mcp - Build & Execution Makefile

.PHONY: build run-sse run-stdio local-sse clean help

# Default: Build the docker image
build:
	@echo "🐳 Building portable memro-mcp docker image..."
	docker build -t memro-mcp .

# Run in SSE (Cloud) mode via Docker
run-sse:
	@echo "🌐 Starting MCP SSE server on port 8080 (Docker)..."
	docker run -it -p 8080:8080 --env-file .env -e MCP_TRANSPORT=sse memro-mcp

# Run in STDIO (Local) mode via Docker
run-stdio:
	@echo "💻 Starting MCP STDIO server (Docker)..."
	docker run -it --env-file .env -e MCP_TRANSPORT=stdio memro-mcp

# Run locally on your Mac (SSE Mode)
local-sse:
	@echo "🍎 Starting MCP SSE server locally..."
	@MCP_TRANSPORT=sse MCP_PORT=8080 ../.venv/bin/python src/memro_mcp/server.py

# Clean up build artifacts
clean:
	@echo "🧹 Cleaning up..."
	rm -rf vendor/ memro_mcp.egg-info/ build/ dist/

help:
	@echo "Usage:"
	@echo "  make build       - Build the portable Docker image"
	@echo "  make run-sse     - Run containerized in SSE mode (Port 8080)"
	@echo "  make run-stdio   - Run containerized in STDIO mode"
	@echo "  make local-sse   - Run locally on Mac in SSE mode"
