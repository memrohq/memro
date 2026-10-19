Error: non-hexadecimal number found in fromhex() arg at position 0Error: non-hexadecimal number found in fromhex() arg at position 0Error: non-hexadecimal number found in fromhex() arg at position 0Error: non-hexadecimal number found in fromhex() arg at position 0# Memro VS Code Extension

A VS Code extension that provides persistent memory capabilities via the Memro MCP server.

## Features
- Remember
- Recall

- Get Recent Memories
- Delete Memory
- Export Memories
- Query Knowledge Graph
ssssss
## Setup

1. Get an API key: run `npx memro connect`, or create one in the dashboard
   (app.memro.co → API keys).
2. Open this folder in VS Code and run the extension (F5).
3. Run **Memro: Set API Key** and paste the `memro_sk_…` key. It is stored in
   VS Code's secret storage (your OS keychain), not in settings.
4. Self-hosting? Set `memro.mcpUrl` to your MCP server, e.g.
   `http://localhost:8080/mcp`. Memro Cloud (`https://mcp.memro.co/mcp`) is the default.

**Memro: Sign Out** removes the key from this machine.

## Settings

| Setting | Default | |
|---|---|---|
| `memro.mcpUrl` | `https://mcp.memro.co/mcp` | MCP server (Streamable HTTP) |
| `memro.sessionId` | `default` | Coordination session; claims are only visible within it |
| `memro.checkOnOpen` | off | Check whether a file is claimed when you open it (sends its workspace-relative path) |
| `memro.autoClaimOnEdit` | off | Claim a file when you start editing it (each claim is stored as a memory) |

Background work (the active-work poll and the two options above) never asks
for a key; it stays idle until you set one. Memory content is treated as
untrusted: the Memory Explorer renders it as text, **Insert** asks before
writing into your file, and file links only open paths inside the workspace.

## Build & Run

- Install: npm install
- Build: npm run compile
- Run from VS Code: F5 (Extension Development Host)

## Troubleshooting
- Ensure memro-mcp is available on PATH
- Ensure backend is running



