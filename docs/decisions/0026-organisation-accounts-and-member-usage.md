# 0026. Organisation Accounts and Usage per End User
Date: 2026-11-14 · Status: Accepted · Builds on 0025

## Context
B2B customers (first: Vaultnel) need three things ADR 0025 left out:

- What each of their users costs, plus the total, from one account.
- Several of their own people managing that account, with the right access.
- A way to invite colleagues without Memro staff.

Before this, usage was recorded per tenant only, with no model token counts.
A dashboard login belonged to one person, only platform admins could act on
a tenant, and nobody could invite anyone.

## Decision
- **Usage per end user, per month.** Every usage event also adds to
  `usage_monthly (tenant, agent, month, metric)` running totals. This is the
  pattern memro-cloud's billing table already uses, so it stays small. The
  rows aren't tied to `agent_identities`, so a deleted user's usage still
  counts for that month.
- **Model usage is recorded.**
  - Search queries' estimated tokens: `query_tokens`.
  - Image-analysis tokens as GPT-4o reports them: `vision_input_tokens`,
    `vision_output_tokens`.
  - Connector imports now also record `stored_tokens`.
  - Embedding tokens = `stored_tokens` + `query_tokens`.
- **`GET /tenant/{id}/usage/members?month=2026-11`** returns each end user's
  totals (named by the customer's `X-Memro-User`), the number of active users
  (anyone with usage that month), the totals, and an estimated model cost.
  - The cost comes from hard-coded list prices, worked out at read time.
    Prices change; counts don't.
  - Who can read it: the organisation's members, platform admins, and the
    tenant's own key (for a customer's billing job).
  - It contains usage only, never memory content.
- **Organisations are dedicated tenants with members.**
  - `tenant_members (tenant, user, role)`. A person keeps one sign-in (their
    `users` row) and can belong to several organisations. We don't move users
    into tenants: emails are unique per tenant, so that would mean one account
    per organisation for the same person.
  - Roles: `owner` (everything, including owners), `admin` (members,
    invitations, tenant keys, usage), `billing` (usage, billing), `viewer`
    (read-only usage).
  - Only owners make or change owners. A tenant always keeps one owner: the
    change locks the owner rows, so two owners can't demote each other at once.
  - Anyone can leave.
- **Invitations by email.**
  - The link carries a one-time secret, stored only as a hash, valid for 7
    days. It can only be accepted by someone signed in with the invited
    address.
  - The response also returns the link, so it can be shared by hand where
    email isn't set up. That's safe because of the address check.
  - `POST /tenant` takes `owner_email` to invite the first owner, so staff
    never have to add people by SQL.
- **Members never read end users' memories.** Membership grants
  management and usage, not an agent. Content stays reachable only through
  the tenant's key, from the customer's own backend. This keeps the customer's
  users' data private and keeps GDPR simple.
- **Customers manage their own tenant keys.** Owners and admins create and
  revoke them. This replaces "staff issue keys" from ADR 0025 and memro-cloud
  ADR 0009.
- **Dashboard.** A new Organization page has an organisation switcher, usage
  per user (by month, with CSV export), members and roles, invitations, and
  server keys. `/invite?token=…` accepts an invitation after sign-in.

## Alternatives considered
- **Users inside each tenant:** one account per person per organisation (see
  above).
- **Real provider token counts for embeddings:** the indexer embeds in batches
  across agents, so per-user attribution would need a split by text length,
  which is an estimate anyway. The 4-bytes-per-token estimate is what billing
  already uses.
- **Admins reading members' memories ("support access"):** deferred. It's a
  privacy decision per customer, and nobody needs it yet.

## Consequences
- Each usage event is now two writes: the existing `tenant_usage` log and the
  monthly total.
- Not measured:
  - audio transcription (Whisper bills per minute; only upload bytes count)
  - cached query embeddings (counted as if they were used)
- Not built: SSO, domain auto-join, and an audit-log page for members.
