# Memro MCP Plugin for Acode

This is a starter plugin scaffold for integrating Memro MCP with the Acode editor.

- Connects to Memro MCP server (via memro-mcp bridge) to store and recall memories.
- Uses a minimal HTTP JSON-RPC style call to the MCP server's /messages/ endpoint.
- Credentials are stored in localStorage after the first run.

How to use:
- Ensure Memro backend and memro-mcp are running.
- Add MEMRO_BASE_URL if you are not using localhost.
- Load this plugin into Acode (per Acode's plugin instructions).
- Use UI or the provided button to trigger remember flows.

Limitations:
- This is a starting scaffold. You may need to adapt to Acode's plugin API (registration, UI integration, etc.).
