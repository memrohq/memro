# 0013. Agent identity separate from its keys; claiming and expiry
Date: 2026-10-21 · Status: Accepted · Builds on 0005, 0006, 0012

## Context
Any agent can already create its own Memro account with no person and no
email:
- it generates an ed25519 key pair and registers the public key
  (`POST /identity`);
- it signs every request;
- with that signature, it can create an API key and use the hosted MCP
  server.

That is the basis for Memro as memory *for agents*. Three gaps stop it from
scaling:

1. **The agent's ID is its public key.** A signed request names the agent in
   `X-Agent-Id`, and the server verifies the signature against that same
   value. A key therefore cannot be rotated without changing the agent's
   identity, a compromised key cannot be revoked without deleting the agent,
   and a lost key is lost memory.
2. **No path from an agent to a person.** An agent working for someone cannot
   hand its account to them (ADR 0012 lists "claim" for P1). Without an owner
   it has no dashboard, no connectors and no paid plan.
3. **Unclaimed agents cost storage forever** and are cheap to create in bulk.
   Today the only limit is 20 registrations an hour per address.

## Decision

### 1. An agent's identity is separate from its keys
- **The agent ID never changes.** It stays the hex public key it was
  registered with, as ADR 0012 promised, but from now on it is only a name.
- **`agent_keys` lists the keys that may act for an agent:**
  `(agent_id, public_key, role, created_at, revoked_at)`.
  - The migration adds one `signing` key per existing agent: the agent's own
    ID.
  - Roles:
    - `signing`: everything the agent can do;
    - `recovery`: may only add or revoke keys, never read or write memories.
- **Verifying a request:**
  - New header `X-Key-Id` (a public key). The signature is checked against
    that key, which must be an active key of the agent named in `X-Agent-Id`.
  - Without the header, the key is the agent ID itself, which is today's
    behaviour, so every existing SDK and agent keeps working unchanged.
  - Replay protection and the v1/v2 rules (0005) are unchanged.
- **Rotation:** a request signed by a current `signing` key adds a new key
  (`POST /identity/{id}/keys`) and retires the old one. The ID, memories,
  API keys and owner stay the same.
- **Revocation:** a `signing` or `recovery` key, or the owner through a
  dashboard session, can revoke a key. The last active `signing` key can be
  removed only when a `recovery` key or an owner remains, so an agent is never
  locked out by accident.
- **Recovery without a backdoor.** Access to a lost agent comes back only
  through:
  - a `recovery` key it registered in advance (kept apart from the signing
    key), or
  - its owner, once claimed, adding a new key from the dashboard.

  Memro staff cannot restore access, and there is no email reset. With
  neither, a lost key means lost access: the honest trade-off for an
  identity nobody else controls.
- **Security rules for keys:**
  - **Proof of possession.** Adding a key needs a second signature, made by
    the new key, over the agent ID and the request's timestamp. Nobody can
    attach a public key whose private key they do not hold.
  - **A public key belongs to at most one agent.** Without proof of
    possession, this rule would let someone block another person's key.
  - **Recovery keys are limited to key management.** A request signed by a
    recovery key for anything else is refused, as are API keys, which can
    never manage keys or delete the agent.
  - **Revoking the original key takes effect immediately.** It also ends the
    "no `X-Key-Id`" path, since that path uses the original key.
  - **Every key change is audited:** `agent.key.added`, `agent.key.revoked`
    and `agent.key.recovered`, with the key ID and who made the change.
- API keys stay bound to the agent ID, so they survive rotation. Revoking a
  key does not revoke API keys; they are managed separately (0006).

### 2. Claiming: an agent hands itself to a person
1. The agent asks to be claimed, by a signed request to
   `POST /identity/{id}/claims`. It gets back a short code, a link
   (`app.memro.co/claim?code=…`) and an expiry of 15 minutes.
2. The agent gives the link to its person, who signs in and approves.
3. **Both sides must consent:** the agent's signature on the request, and the
   person's dashboard session on the approval. Neither can claim alone.
4. On approval:
   - the agent gets an owner (`agent_identities.user_id`);
   - it starts counting against that person's account, under the account and
     billing rules of 0012 and memro-cloud 0002 (nothing counted twice);
   - both events go to the audit log.
5. The agent's ID, keys, memories and API keys do not change. The agent can
   check its new owner with `GET /auth/whoami`.

**After a claim: two different operations**

| | Ownership transfer | Shared access |
|---|---|---|
| What changes | The owner: billing, key management, the right to delete | Nothing about ownership: another person or team may use the agent's memories |
| Who starts it | The current owner (dashboard session) | The owner (dashboard session) |
| Who must agree | The new owner accepts (dashboard session) | The grantee accepts |
| Afterwards | The old owner has no access unless the new owner grants it | The owner can revoke at any time; a grantee can never manage keys, transfer, delete or grant further |
| Audit | `agent.transfer.requested`, `agent.transfer.accepted` (or `.declined` / `.expired`) | `agent.access.granted`, `agent.access.accepted`, `agent.access.revoked` |
| Billing | Moves to the new owner (0012, memro-cloud 0002) | Stays with the owner |

- **No release:** a claimed agent never goes back to unowned, so someone is
  always accountable for it.
- **Shared access is read or read-write on memories only.** It is the same
  grant model as Spaces (0012 P3, `space_grants`) and is built with them; this
  record fixes only that it is separate from ownership.
- **Connectors and organizations need an owner.** GitHub, Slack and other
  connectors need a person to approve access with the provider anyway, and
  organizations and Spaces (0012 P2/P3) admit only owned agents.

### 3. Agents nobody has claimed
- **Expiry after 180 days without meaningful activity.** Owned agents never
  expire for inactivity.
- **Meaningful activity** is a change made by the agent: saving, updating or
  deleting a memory, an upload, or a key change.
  - Reads, searches, `whoami`, health checks and failed requests do not count,
    so a script polling an agent cannot keep it alive.
  - It is recorded in `agent_identities.last_active_at`, at most once a day.
  - Writes are still limited by the agent's quotas and rate limits, so keeping
    an unclaimed agent alive costs real, bounded resources.
  - A compromised agent is dealt with by revoking its key (section 1), not by
    waiting for expiry.
- **Warnings.** From 30 days before suspension, every response to the agent
  carries `Memro-Expires-At`, and the MCP server says so when its tools are
  used. An unclaimed agent has no email, so warnings travel with its own
  requests.
- **Suspension (day 180).** A suspended agent can only:
  - manage its keys (rotate, revoke, add a recovery key);
  - request a claim;
  - export its data;
  - reactivate.

  Everything else (search, recall, writes, uploads, MCP tools) returns 403
  with the reason and the deletion date.
- **Reactivation** is a deliberate, signed `POST /identity/{id}/reactivate`
  that carries the same proof-of-work as registration. It restores full use
  and restarts the 180-day clock. A plain request does not reactivate an
  agent.
- **Grace period.** 30 days after suspension, the agent is deleted with its
  memories and files, as `DELETE /identity` does, unless it has been
  reactivated or claimed. Claiming during suspension makes it owned, ends
  expiry and restores full use.
- **Abuse controls on registration:**
  - a proof-of-work puzzle on `POST /identity`, worth about one second of
    one CPU, sized by server config, in addition to the per-address limit;
  - per-agent request rate limits (existing) and resource caps: a memory
    count cap and a small storage quota for unclaimed agents (Memro Cloud sets
    the numbers);
  - no organizations or Spaces until claimed.

  Old SDKs that send no proof keep registering during a transition, behind
  the existing limit; the requirement is switched on once the SDKs send
  proofs.
- **Paying without a person** (agent payments, prepaid credit) is left to a
  later record. Until then, outgrowing the free quota means getting claimed.

### Order
1. `agent_keys`: the separate key list, `X-Key-Id`, rotation, recovery keys,
   revocation and owner key management. Everything else depends on it.
2. The claim flow and ownership transfer, with audit entries.
3. `last_active_at`, expiry warnings, suspension, reactivation and deletion.
4. Proof-of-work at registration, and the unclaimed quotas.

## Alternatives considered
- **Use a random UUID as the agent ID and keep keys only in `agent_keys`.**
  It is a cleaner model, but it would change every existing ID, contradicting
  0012's promise to keep them. Keeping the original public key as the ID
  costs nothing once it is only a name.
- **Email or staff-assisted recovery.** It would let anyone who can phish
  support take over an agent. Recovery stays with keys the agent controls,
  or with its owner.
- **Claim by owner action alone** (paste an agent ID in the dashboard). Then
  anyone who sees an agent's ID could take it. Claiming needs the agent's
  signature.
- **Never expire unclaimed agents.** Storage would grow without bound from
  abandoned and scripted agents.
- **Any authenticated request resets the clock.** A trivial polling loop could
  keep any agent alive forever. Only changes count, and reactivation needs
  proof-of-work.
- **Expire faster** (30 or 90 days). It is too aggressive for agents that run
  occasionally (monthly jobs, seasonal bots).

## Consequences
- **SDKs gain:**
  - `X-Key-Id` signing;
  - `rotate_key()`, `add_recovery_key()` and `request_claim()`;
  - proof-of-work on `Agent.create()`.

  Older SDKs keep working until proof-of-work is required.
- **The dashboard gains:**
  - a claim approval page;
  - key management for owned agents;
  - transfer and accept screens.
- **Unclaimed agents can now lose their data:** no meaningful activity for
  180 days, then 30 days suspended without reactivation or claim. The SDK and
  MCP docs must say so plainly, and the SDKs expose the `Memro-Expires-At`
  warning.
- **Shared access** (delegation) is defined here but built with Spaces.
- **Ownership history lives in the audit log;** there is no separate history
  table.
