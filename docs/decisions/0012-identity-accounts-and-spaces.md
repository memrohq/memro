# 0012. Users, agents, accounts and Spaces
Date: 2026-10-20 · Status: Accepted

## Context
Memro grew one identity at a time: ed25519 agents (0005), email sign-in that
creates an agent, API keys bound to an agent (0006), and a `tenants` table
that nothing manages. A review on 2026-10-20 found the result conflates four
different things:

- Every self-signup user lands in one shared default tenant, and limits and
  usage were keyed by tenant, so strangers shared one account.
- `POST /tenant` and `GET /tenant/{id}` were open to anyone signed in, because
  everyone is in the default tenant, which the checks treated as privileged.
- An API key, the credential that sits in tool config files, could delete its
  whole agent.
- A session always acts as the user's oldest agent, so one user cannot have
  several agents.
- `memories.visibility = 'shared'` is stored but no read ever honours it.

Companies need many people, teams, a reporting hierarchy, shared memory and
administration. That needs these concepts kept apart.

## Decision

### Four concepts
| Concept | What it is | Lives in |
|---|---|---|
| **User** | A human, signed in with an email code | `users`, `sessions` |
| **Agent** | One memory context (personal, coding, research…) | `agent_identities` |
| **Organization** | A company: members, roles, teams | `organizations`, `org_members`, `teams` (P2) |
| **Access** | Who may read or write which memories | agent ownership, `spaces`, `space_grants` (P3) |

### Users and credentials
- People sign in with an email code. Passwords are not planned. Adding them
  would need its own record.
- Each surface has one credential (0007):

  | Credential | Acts as | May also |
  |---|---|---|
  | Dashboard session | The user, on a selected agent they own | Manage keys, approve device logins, delete agents, admin actions |
  | `memro_sk_` API key | One agent (and later only its granted scopes) | Nothing else |
  | ed25519 signature | The agent whose private key signed | Manage that agent, including deleting it |

- **An agent ID is never a credential by itself.** It is a public key and an
  address. Only a signature made with its private key proves anything.
- **API keys never perform destructive or account-level operations**:
  deleting an agent, managing keys, approving logins and admin actions.
  A key leaked from a tool's config can read and write that agent's memories,
  but cannot destroy the agent or escalate.
- **What an API key may do** is everything on its own agent's memory:
  - Remember, recall, delete memories and export. Export returns only that
    agent's memories, which the key can already read through search. The MCP
    `export_memories` tool relies on it.
  - Webhooks, with one limit: a webhook registered with a key is deleted
    when that key is revoked (`hooks.api_key_id`, migration 0026).
    Otherwise a leaked key could leave a hook behind that keeps receiving
    new memories after the owner cleaned up.
- **Unauthenticated agent registration** (`POST /identity`) stays, for SDK
  self-owned agents (0009). A public key without its private key grants
  nothing. Each client address may register 20 agents an hour.
- **The public identity lookup** (`GET /identity/{id}`) says only whether an
  agent exists (`id`, `created_at`). Its owner and tenant are visible to the
  agent itself through `/auth/whoami`, not to everyone.
- **Platform admins** (Memro staff) are listed in `platform_admins` and act
  only through a dashboard session. Admin power never comes from an API key,
  a signature or an email domain. Hiding UI is not access control; every check
  is on the server.

### Agents
- An agent ID stays the hex ed25519 public key, generated once and never
  regenerated. Existing IDs are kept as they are.
- `agent_identities.user_id` is the owner. Every agent created through the
  dashboard or device login has exactly one owning user. There is no
  `user_agents` join table: co-ownership is not a goal, and sharing happens
  through Spaces.
- A user may own many agents (P1): the dashboard gets an agent picker, and
  the session names the selected agent per request. The server verifies the
  user owns it.
- Self-registered SDK agents (`POST /identity` with a public key) have no
  owner. They remain supported as standalone single-agent accounts,
  authenticated only by signature. P1 adds a signed claim so a user can adopt
  one.
- Agents do not share private memories with each other, even under one
  owner.

### Organizations (P2)
- A company is an organization, not a user or an agent. Members keep their
  own user accounts and personal emails; a membership links them.
- Roles: **owner** (settings, admins, subscription), **admin** (members,
  teams, invites, Spaces), **manager** (their team's membership and team
  resources), **member** (personal memory plus granted Spaces).
- Teams form a tree (`parent_team_id`). The reporting line (`manager_id`) is
  recorded separately and **grants nothing by itself**. A manager cannot read
  a report's private memories.
- Joining needs an invite or approval by an owner or admin. A verified email
  domain makes invites and approval easier but never grants access by itself.

### Spaces: shared memory (P3)
- A Space is a shared memory store owned by a user, team or organization.
  `space_grants` give **users or teams** read or read-write access. An agent
  has its owner's access, so grants are not managed per agent.
- An API key can be narrowed (P3 scopes), e.g. a coding agent's key may only
  write to the Engineering Space.
- A memory has an owning agent, and optionally a Space. Without a Space it is
  private to the agent. Every read, search, update and delete checks access
  on the server, in the same query that selects the rows.
- `visibility` is deprecated: still accepted and stored, never used to grant
  access. It is removed once clients stop sending it.

### Accounts and quotas
An **account** is what limits and billing apply to. Each agent counts
against exactly one account at a time:

| Agent | Account |
|---|---|
| Owned by a user, personal | That user's personal account |
| Assigned to an organization that covers it (P2) | The organization's account |
| In a dedicated tenant created by a platform admin | That tenant |
| Self-registered SDK agent with no owner | The agent alone |

The shared default tenant is **never** an account.

- **Primary quota: stored text (tokens) summed over every agent that counts
  against the account.** Creating more agents never multiplies an
  allowance.
- **Safety cap:** the per-agent memory-count cap (`MEMORY_SOFT_CAP`) stays as
  protection against runaway writes. It is not the paid quota.
- **Effective limit**, applied the same way to API, MCP, SDK and dashboard
  requests: (1) an active override for the account, (2) otherwise the
  organization's policy for covered members, (3) otherwise the plan.
  Overrides (P1) belong to accounts, never to single agents. Each override
  records its scope, value, unit, expiry, reason and the admin who set it.
- **Moving between accounts**:
  - Usage counts against an agent's account at the moment it happens.
  - Stored text counts against the agent's current account.
  - Assigning an agent to or from an organization therefore moves its whole
    stored total at once and leaves past monthly counters where they were.
  - Nothing is counted twice, and nothing stored is deleted.
  - If the new account is over its limit, new writes are refused until there
    is room.
- **Leaving an organization**: memories in organization Spaces stay with the
  organization. The organization's settings decide whether an
  organization-assigned agent returns to the user's personal account or is
  held for an admin to export or delete.
- Plans, prices, payment providers and subscription handling are not part of
  the community server. Memro Cloud implements them behind
  `extension.QuotaPolicy`; see memro-cloud's ADR 0002.

### Tenants
`tenant_id` remains a data-partition column. Personal and organization data
stay in the default tenant, and organizations are not tenants. Creating a
tenant, and reading the default tenant's details or usage, is platform-admin
only. Members of a dedicated tenant may read their own tenant.

## Phases
- **P0** (this change):
  - API keys cannot delete agents.
  - Webhooks registered with a key are deleted when the key is revoked.
  - Tenant endpoints require a platform admin or membership of a dedicated
    tenant (`platform_admins`, migrations 0025 and 0026).
  - Agent registration is limited per address, and the public identity
    lookup hides the owner.
  - Memro Cloud keys accounts by user, not tenant, and sums stored text
    across a user's agents.
- **P1**: multiple agents per user with a dashboard picker; claiming
  self-registered agents; account quota overrides; platform-admin dashboard.
- **P2**: organizations, invites, roles, verified domains, teams, reporting
  lines, organization accounts and agent assignment.
- **P3**: Spaces and grants, permission-aware search, API-key scopes,
  coupons, organization usage reporting.

## Alternatives considered
- **A `user_agents` join table** allows co-owned agents, but makes "whose
  account pays" ambiguous. Spaces cover sharing.
- **Grants to agents** are precise, but unmanageable for hundreds of
  employees with several agents each. Grants go to users and teams, and API
  key scopes narrow them when needed.
- **Organization = tenant** would split one person's personal and work
  memories across partitions and complicate moving between them.
- **Keep the default tenant as an account and patch the symptoms.** This was
  rejected: it is the root of the shared-allowance and cross-user billing
  bugs.
- **Admins listed in an environment variable.** It is simpler to deploy, but
  not auditable and not manageable from an admin dashboard.

## Consequences
- SDK users who delete an agent with an API key now get 403. They must sign
  the request with the agent's private key or use the dashboard. The SDKs'
  own signed deletes are unaffected.
- `POST /tenant`, `GET /tenant/{id}` and `GET /tenant/{id}/usage` return 403
  for ordinary users. No client of ours calls them.
- `GET /identity/{id}` no longer returns `user_id` and `tenant_id`. No client
  of ours read them; the JS SDK's type is narrowed.
- Webhooks registered before migration 0026 have no recorded key and are not
  removed by revoking one. Review them on upgrade.
- The first platform admin is granted with SQL; see the ops runbook.
- Organization and Space tables, quota overrides and the agent picker do not
  exist yet. Until P1 and P2 land, each user has one agent and one personal
  account.
