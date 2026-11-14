# 0025. Tenant Keys and End Users for B2B Customers
Date: 2026-11-13 · Status: Accepted

## Context
B2B customers (the first is Botnel, a hiring-intelligence platform) want Memro
as the memory for each of *their* users, e.g. "whom did this recruiter email
last month and who replied". They buy one account, but have hundreds to
millions of users whose memories must never mix, and they won't create a
Memro account per user. Before this, every credential acted as exactly one
agent. A tenant could exist (`POST /tenant`, platform admins only), but no
credential could act for the many users inside it.

## Decision
- **One tenant per customer.** A platform admin creates it (`POST /tenant`)
  and issues a **tenant key** (`POST /tenant/{id}/api-keys`): an `api_keys`
  row with no `agent_id`. It has the same `memro_sk_` prefix, so secret
  scanners still find it.
- **One agent per end user, created on first use.** Every request with a
  tenant key must send `X-Memro-User: <the customer's own user ID>`
  (1–128 chars of letters, digits and `. _ - : @`). Memro maps
  `(tenant_id, external_id)` to an agent identity and creates it the first time
  that ID appears, with a unique index so two first requests that race get
  the same agent. The agent ID is a fresh ed25519 public key whose private key
  is discarded, so the agent is reachable only through the tenant key.
- **Isolation mostly reuses existing scoping.** Memories, search, graph,
  files, hooks and rate limits are already scoped by `agent_id` + `tenant_id`
  (ADR 0018); coordination needed one change (below). A
  tenant key resolves to that end user's agent before any handler runs, so
  every existing endpoint is per user. Two customers may use the same user IDs.
- **A tenant key is an API key acting as the end user** (`Via: "api_key"`), so
  every API-key restriction applies unchanged: no minting keys, approving
  device logins, `DELETE /identity`, or admin endpoints.
- **End users never share coordination.** A dedicated tenant's agents share
  one scope for claims and intents (ADR 0019), but a tenant's end users get
  one each, so one user's claim notes never reach another.
- **Rate limits apply per end user, not per address.** A customer's backend
  sends every user's requests from a few IP addresses, so a request that a
  tenant key authenticates gives back its hit on the per-address limit. Failed
  key checks still count, so guessing keys stays limited.
- **Offboarding / right to erasure:** `DELETE /end-user` with the tenant key
  and `X-Memro-User` deletes that user's agent, memories and files (`404` if
  the user doesn't exist, without creating it). Only a tenant key may call it.
- Usage is recorded per tenant, which is the unit we bill.

## Alternatives considered
- **A tenant per end user.** Separates data no better (separation comes from
  the row scoping), but breaks billing and usage into thousands of tenants
  and blocks any later team-level sharing.
- **A Memro account per end user (signup or device login).** Customers won't
  run a signup per user, and this would need email addresses we shouldn't hold.
- **A `user_id` field in each request body.** Easy to forget on one endpoint.
  A header resolved once in authentication covers every endpoint, including
  future ones.

## Consequences
- The customer's backend is trusted to send the right `X-Memro-User`. A leaked
  tenant key exposes all of that customer's users. Keys stay server-side; the
  customer should revoke and reissue on any leak.
- Rate limits apply per end user. There is no per-tenant cap yet.
- Customers can't issue or rotate their own tenant keys yet. A platform admin
  does it (see memro-cloud `docs/operations.md`).
- Not built yet: a team level between tenant and user (e.g. "has anyone at
  Acme emailed this person?"), Postgres row-level security as a backstop, and
  per-tenant usage billing in memro-cloud.
