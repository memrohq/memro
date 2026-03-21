# Use a lightweight Python base image
FROM python:3.11-slim

# Set the working directory
WORKDIR /app

# 1. Install the vendored Memro SDK
# We copy it into a temporary location in the container
COPY vendor/memro-sdk-python ./sdk
RUN pip install --no-cache-dir ./sdk

# 2. Copy the MCP server source
COPY pyproject.toml .
COPY README.md .
COPY src ./src

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
