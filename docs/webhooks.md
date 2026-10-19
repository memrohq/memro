# Webhooks

Register a URL and Memro POSTs to it each time the agent stores a memory.

```bash
curl -s https://api.memro.co/hooks -H "Authorization: Bearer $MEMRO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"event_type": "memory_created", "url": "https://example.com/memro-hook"}'
```

A webhook belongs to the agent that registered it and fires only for that
agent's memories. List them with `GET /hooks/memory_created`.

## Delivery

```http
POST /memro-hook HTTP/1.1
Content-Type: application/json
User-Agent: memro-webhooks/1

{ "event": "memory_created", "memory_id": "8d0e…", "agent_id": "3f9a…",
  "content": "Ana leads the platform team", "memory_type": "profile" }
```

- Sent in the background after the memory is stored, once, with a 10-second
  timeout. Failures are logged on the server and not retried.
- Redirects are not followed.
- The URL must be public `http(s)`. Private, loopback, link-local and
  carrier-grade NAT addresses are refused, both when you register the hook
  and when it is delivered (so DNS rebinding can't reach internal hosts).
- Fires for `POST /memory`, `/memory/from-file` and `/upload/image`; not for
  document chunks from `/upload/document` or memories the engine creates.

Requests are not signed yet. Use an unguessable URL (for example with a
random path segment) and treat the payload as untrusted input.

Remove one with `DELETE /hooks` and the same JSON body (`204`, or `404` if
it isn't registered). Deleting the agent removes all of its webhooks.
