# Use a lightweight Python base image
FROM python:3.11-slim

# Set the working directory
WORKDIR /app

# Build from the repository root: docker build -f mcp/Dockerfile .

# 1. Install the Memro Python SDK from this monorepo
COPY sdk/python ./sdk
RUN pip install --no-cache-dir ./sdk

# 2. Copy the MCP server source
COPY mcp/pyproject.toml .
COPY mcp/README.md .
COPY mcp/src ./src

# Install the MCP server
RUN pip install --no-cache-dir .

# Create a non-privileged user and switch to it
RUN addgroup --system memro && adduser --system --group memro
USER memro

# Expose port 8080 for SSE (Cloud Mode)
EXPOSE 8080

# Environment variables will be injected at runtime
ENV PYTHONUNBUFFERED=1
ENV MCP_TRANSPORT=stdio
ENV MCP_PORT=8080

# Run the MCP server
ENTRYPOINT ["memro-mcp"]
