# Limits and errors

## Errors

Failed requests return a 4xx/5xx status with a short plain-text message:

```http
HTTP/1.1 403 Forbidden
Content-Type: text/plain; charset=utf-8

agent_id does not match authenticated identity
```

| Status | Meaning | What to do |
|---|---|---|
| `400` | Invalid input (bad JSON, empty content, unknown `memory_type`, query too long) | Fix the request |
| `401` | Missing, invalid, revoked or expired credential; bad or reused signature; timestamp more than 5 minutes off | Check the key, your clock, and the signing payload |
| `403` | Acting on another agent; an API key managing keys; memory limit reached | Use your own agent ID; use the dashboard; delete old memories |
| `404` | No such memory, file, webhook or code (or it belongs to another agent) | |
| `413` | Body larger than 10 MB | |
| `429` | Rate limited | Back off and retry |
| `500`, `502`–`504` | Server or gateway error | Retry reads with backoff |

The SDKs raise `MemroError` (status and message) and `RateLimitError` for
429. They retry 429 automatically, retry gateway errors only for `GET` and
`DELETE`, and re-sign a signed request refused as a duplicate.

## Rate limits

| Scope | Limit |
|---|---|
| Per client IP, all endpoints | 200 requests per minute |
| Per agent, per endpoint | 200 requests per minute |
| `POST /mee/reason` | 20 per minute per agent |
| `POST /mee/resolve` | 10 per minute per agent |
| `POST /mee/evolve` | 5 per minute per agent |
| Sign-in codes (`POST /auth/login`) | 5 per email per hour |

Limits use fixed one-minute windows. Calls through the hosted MCP server
count against your own IP and agent, not the MCP server's. Self-hosters can
change the first two with `RATE_LIMIT_MAX` and `RATE_LIMIT_WINDOW_SECONDS`.

## Sizes

| | Limit |
|---|---|
| Memory content | 10,000 characters |
| Search query | 500 characters |
| `limit` on `/search`, `/search/temporal` | 50 |
| `limit` on `/memory/agent/{id}` | 100 |
| Request body (uploads included) | 10 MB (`MAX_BODY_BYTES`) |
| API key name | 60 characters |
| Webhook `event_type` | 50 characters |

## Memory limit

Each agent can hold up to 1,000 memories on Memro Cloud. Past 80%,
`GET /usage` returns a `warning`; at the limit, writes return `403` with a
message saying so. Delete or export old memories to make room. Self-hosters
set the limit with `MEMORY_SOFT_CAP`.
