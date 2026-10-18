# 0007. No private keys in the browser; one credential per surface
Date: 2026-10-19 · Status: Accepted (amends 0005)

## Context
Email sign-in generated the agent's ed25519 key pair on the server and
returned the private key to the dashboard, which kept it in localStorage and
showed it on the Identity page. That contradicts the model in 0005 (a private
key never leaves the machine that created it), exposes the key to any XSS, and
left users copying raw keys into tool configs.

## Decision
| Surface | Credential |
|---|---|
| Dashboard (browser) | Session token from email sign-in. Never a private key. |
| Tools, MCP clients, CI | `memro_sk_` API keys (0006), revocable, from `memro connect` or the dashboard. |
| SDK apps that want signatures | Their own ed25519 key, generated locally; only the public key goes to the server (`POST /identity`). |

- Sign-in creates the account's agent with a server-side key pair and
  discards the private half; the account acts through sessions and API keys.
- The dashboard drops any `memro_pk` left in localStorage.
- Public repo (memrohq/memro) holds everything self-hosters run; the private
  memro-cloud repo holds only what is specific to running Memro Cloud
  (billing, quotas, infrastructure, website). See 0003.

## Consequences
Agents created by earlier dashboards still have keys in old browsers'
storage until they next open the dashboard; those keys remain valid for
signed requests. Rotating them would need a key-rotation endpoint (later).
