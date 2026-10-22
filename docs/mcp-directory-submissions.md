# MCP Directory Audit and Submission Guide

This guide details where Memro appears across major Model Context Protocol (MCP) registries, what each listing states, and the exact metadata, text, and commands needed to submit or update listings pointing to the canonical `memrohq/memro` repository and published packages (`memro-mcp` / `memro-cli`).

---

## 1. GitHub Repository Metadata

Run this command from your terminal to configure repo description and discovery topics:

```bash
gh repo edit memrohq/memro \
  --description "Memro — verified memory infrastructure for AI agents. Server, SDKs (Python/JS/Go), MCP server, CLI. Source available." \
  --homepage "https://memro.co" \
  --add-topic ai-agents \
  --add-topic agent-memory \
  --add-topic ai-memory \
  --add-topic mcp \
  --add-topic llm \
  --add-topic rag \
  --add-topic pgvector \
  --add-topic golang
```

---

## 2. Smithery (`smithery.ai`)

- **Current Status**: Unindexed / missing or confusing with Mem0.
- **Action**: Register Memro on Smithery.
- **Submission Link**: [https://smithery.ai/new](https://smithery.ai/new) or run:
  ```bash
  npx -y @smithery/cli publish
  ```
- **Repository URL**: `https://github.com/memrohq/memro`
- **Root Manifest**: [smithery.yaml](smithery.yaml) (pre-configured in repo root).
- **Listing Name**: `memro`
- **Display Title**: `Memro — Verified Memory Infrastructure for AI Agents`
- **Short Description**:
  > Give AI agents persistent, verifiable memory across sessions. Hybrid search retrieval (PostgreSQL + pgvector), sovereign Ed25519 cryptographic identity, and cross-session recall for Claude Code, Cursor, Codex, and all MCP clients.
- **Installation Command**:
  ```bash
  npx -y @smithery/cli install memro --client claude
  ```

---

## 3. Glama (`glama.ai/mcp/servers`)

- **Current Status**: Old community search results point to legacy repositories or external memory servers.
- **Action**: Submit the canonical Memro server entry.
- **Submission Link**: [https://glama.ai/mcp/servers](https://glama.ai/mcp/servers) (or open PR at [glama-ai/mcp-servers](https://github.com/glama-ai/mcp-servers)).
- **Repository URL**: `https://github.com/memrohq/memro`
- **Package Name**: `memro-mcp` (PyPI) / `memro-cli` (npm)
- **JSON Entry**:
  ```json
  {
    "name": "memro",
    "title": "Memro Memory Server",
    "description": "Verified memory infrastructure for AI agents with hybrid pgvector search and sovereign identity.",
    "homepage": "https://memro.co",
    "repository": "https://github.com/memrohq/memro",
    "license": "LicenseRef-PolyForm-Noncommercial-1.0.0",
    "command": "npx",
    "args": ["-y", "memro-cli", "mcp"],
    "env": {
      "MEMRO_API_KEY": {
        "description": "Memro API Key from https://memro.co",
        "required": true
      }
    }
  }
  ```

---

## 4. MCP.so (`mcp.so`)

- **Current Status**: Pending canonical registration.
- **Action**: Submit new listing.
- **Submission Link**: [https://mcp.so/submit](https://mcp.so/submit)
- **Title**: Memro
- **Tagline**: Verified Memory Infrastructure for AI Agents
- **Category**: Memory / Agent Utilities
- **Tags**: `memory`, `ai-agents`, `mcp`, `pgvector`, `hybrid-search`, `sovereign-identity`
- **Repository**: `https://github.com/memrohq/memro`
- **Documentation**: `https://memro.co`

---

## 5. Official Model Context Protocol Registry (`modelcontextprotocol/servers`)

- **Repository**: [github.com/modelcontextprotocol/servers](https://github.com/modelcontextprotocol/servers)
- **Action**: Add entry under community / external servers list.
- **PR Description**: Add Memro persistent memory server for AI agents.
- **Configuration Spec**:
  ```json
  {
    "name": "memro",
    "description": "Verified memory infrastructure for AI agents. Persists cross-session context, decisions, and operational runbooks using hybrid search and sovereign agent identity.",
    "homepage": "https://memro.co",
    "repository": "https://github.com/memrohq/memro",
    "docker": "ghcr.io/memrohq/memro",
    "pypi": "memro-mcp",
    "npm": "memro-cli"
  }
  ```

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
