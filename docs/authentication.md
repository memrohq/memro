# Authentication

Every request acts as one agent. There are three credentials; send one.

| Credential | Header | Used by |
|---|---|---|
| API key | `Authorization: Bearer memro_sk_…` | Tools, MCP clients, servers, CI |
| Signed request | `X-Agent-Id`, `X-Timestamp`, `X-Signature`, `X-Signature-Version: 2` | Self-owned agents (SDK `Agent.create()`) |
| Dashboard session | `Authorization: Bearer <session token>` | The dashboard only |

Public endpoints need none: `GET /health`, `POST /identity`,
`GET /identity/{id}`, `GET /assets/{id}` and the sign-in and device-login
endpoints.

## API keys

- Create: `npx memro-cli connect`, or dashboard → **API keys** → **New key**.
  The key is shown once; Memro stores only its SHA-256 hash.
- Use: `Authorization: Bearer memro_sk_…`. The SDKs read `MEMRO_API_KEY`.
- Revoke: dashboard → **API keys** → **Revoke**. It stops working immediately.
- `GET /auth/whoami` tells you which agent a key acts as.
- A key can do everything its agent can, except manage API keys or approve
  device logins. Those need the dashboard.

Treat keys like passwords: keep them out of source control and client-side
code, use one per tool or machine, and revoke any that leak. Keys start with
`memro_sk_` so secret scanners can find them.

## Signed requests

A self-owned agent signs each request with its ed25519 private key; the
server checks it against the agent ID (the public key). Nothing secret is
sent, and a captured request can't be replayed.

```
payload   = "<timestamp>\n<METHOD>\n<path?query>\n" + <raw body bytes>
signature = ed25519_sign(private_key, payload)          # hex in X-Signature
```

- `timestamp`: Unix seconds, also sent as `X-Timestamp`. It must be within
  5 minutes of the server's clock.
- `METHOD`: uppercase (`GET`, `POST`, …).
- `path?query`: exactly as sent, without the host, e.g.
  `/search?agent_id=ab12…&query=deploy+day&limit=5`. If you call through a
  proxy prefix (e.g. `app.memro.co/api`), sign the path the API receives
  (`/search?…`).
- body: the exact bytes sent; empty for `GET` and `DELETE`. For multipart
  uploads, the full encoded multipart body.

Headers: `X-Agent-Id: <agent id>`, `X-Timestamp`, `X-Signature`,
`X-Signature-Version: 2`.

Each signature is accepted once. Because ed25519 signatures are
deterministic, sending the identical request twice within the same second
gets `401 Request already used`; the SDKs wait for the next second and
re-sign automatically.

Register a self-owned agent with `POST /identity`: its public key plus a
small proof-of-work, about a second of CPU, so agents stay free to create
but costly to create in bulk ([API reference](api-reference.md#post-identity)).
No person or email is involved. The SDKs do this for you:

```python
agent = Agent.create()        # key pair generated locally
agent.save("identity.json")   # the private key can't be recovered; keep this file secret
```

A self-owned agent can mint API keys for its own tools with a signed
`POST /api-keys`. See [Agents that sign themselves up](agents.md) for the
whole flow and the limits of an agent nobody has claimed.

**Legacy v1 signatures** (timestamp + body only, no `X-Signature-Version`)
don't cover the method or URL. Servers reject them by default; set
`ALLOW_LEGACY_SIGNATURES=true` only if older clients require it. New code must use v2.

## Device login (how `npx memro-cli connect` works)

The CLI never sees your email or session. It follows the OAuth device flow
([RFC 8628](https://www.rfc-editor.org/rfc/rfc8628)):

1. `POST /auth/device/code {"client_name": "…"}` →
   `{device_code, user_code, verification_uri_complete, expires_in, interval}`.
2. The CLI shows `user_code` (e.g. `KQZT-HMRW`) and opens
   `verification_uri_complete` in the browser.
3. You sign in to the dashboard, check that the code matches, and press
   **Connect** (`POST /auth/device/approve`, dashboard session only).
4. The CLI polls `POST /auth/device/token {"device_code": "…"}` every
   `interval` seconds. It gets `authorization_pending` until you approve,
   then `{api_key, agent_id, api_url, mcp_url}` exactly once.
   `expired_token` after 10 minutes.

Use the same flow to connect your own CLI or desktop app.

## Dashboard sign-in

`POST /auth/login {"email"}` emails a 6-digit code (valid 15 minutes; at
most 5 codes per email per hour). `POST /auth/verify {"email", "code"}`
returns a session token valid for 7 days. Each code allows 5 attempts and
works once. Sessions are meant for the dashboard; use API keys everywhere
else.
