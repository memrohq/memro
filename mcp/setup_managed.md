# Setup Guide: 100x Managed Mode (SaaS/Multi-tenant)

Memro is architected to be 100x more scalable and future-proof than existing memory protocols.

## 1. Start the Server in Production Mode

Run the server with `MCP_TRANSPORT=sse`. In production, Memro uses **URL-based Tenancy** to provide isolated streams for every agent.

```bash
docker run -p 8080:8080 \
  -e MCP_TRANSPORT=sse \
  -e MEMRO_BASE_URL=http://your-backend-url \
  memro-mcp
```

## 2. Dynamic Identity Resolution (100x Pattern)

Instead of a single endpoint, you can now route agents to their own dedicated streams:

- **SSE URL**: `http://localhost:8080/sse/{agent_id}`
- **Universal Webhook**: `POST http://localhost:8080/webhook/{agent_id}`

### Authenticating your Agent

Memro supports two methods for authentication:

1.  **Bearer Token (Primary)**: Provide the `Authorization: Bearer <token>` header.
2.  **Query Parameter (Fallback)**: If your client (e.g., Claude Code, MCP Inspector) has issues with custom headers, pass the token in the URL:
    - `http://localhost:8080/sse/{agent_id}?token=your_token_here`

## 3. Agent Configuration (e.g., Claude Desktop)

In 100x Transparent Mode, the agent **never sees its credentials**. It is "logged in" by the underlying infrastructure.

Simply add this to the agent's system prompt:

> "You are an AI with proactive long-term memory. 
> 1. Check your status using the `get_system_status` tool.
> 2. Use `remember` and `recall` for persistent memory.
> 3. You are 'webhook-aware'—external data may arrive in your memory at any time."

## 4. Proactive Webhooks

You can push memories into an agent's stream from *external* apps (GitHub, Slack, etc.) by hitting the /webhook endpoint. Memro will automatically parse and store the event for the agent to recall later.

## 5. Verification
Use the `test_saas_multi_tenant.py` script to verify that agents on different URL paths (`/sse/alice` vs `/sse/bob`) are completely isolated.
