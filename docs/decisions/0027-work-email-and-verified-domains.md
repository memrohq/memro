# 0027. Work Email, Self-Service Organisations and Verified Domains
Date: 2026-11-14 · Status: Accepted · Builds on 0026

## Context
Until now only Memro staff could create an organisation (ADR 0026), which
doesn't scale past a few sales-led customers. Letting anyone create one
raises two questions: is this person at a real company, and do they speak
for it? An email domain answers neither on its own. Anyone at a company can
sign up first, and a free mailbox says nothing about a company.

## Decision
Keep three steps apart. Each needs its own proof and grants only what that
proof supports.

- **Work email: what the dashboard offers, never rights.** Sign-in already
  proves the person owns the address. `internal/workemail` rejects free
  mailbox providers (Kikobeats/free-email-domains, MIT) and throwaway
  services (disposable-email-domains, CC0), including their subdomains. Both
  lists are embedded, so nothing is looked up per request; the refresh
  commands are in the package comment.
- **Anyone with a work email may create an organisation** (`POST /orgs`) and
  owns it, up to 5 per person. It starts unverified, holding no domain, and
  on the trial (ADR 0028).
- **A DNS TXT record proves the company.** An owner or admin claims a domain
  and adds `_memro-challenge.<domain>  TXT  "memro-verify=<token>"`.
  `POST …/verify` checks it. This is the same proof Vercel, GitHub, Google
  Workspace and Okta use.
  - Any number of organisations may claim a domain, but a unique index lets
    only one hold it verified. The first to prove it wins; a dispute goes to
    support, and the valid record decides it.
  - Verified domains are re-checked weekly (an hourly loop picks up those
    due). A record that is gone loses its verification, since companies
    change hands and staff leave. A DNS failure changes nothing.
  - Domains Memro staff set when creating a tenant count as verified and
    aren't re-checked (`token` NULL). Existing tenants' domains were
    backfilled this way, Vaultnel's included.
- ~~Server keys need a vouched-for organisation.~~ Superseded by ADR 0028:
  self-service organisations start on a trial with a shared memory pool,
  which bounds their cost, and a key only reaches its own organisation's
  users.
- **Joining through a domain never grants more than an owner chooses.** With
  `join_policy = request` (only possible with a verified domain), people
  signed in at that domain see the organisation (`GET /orgs/discover`) and
  may ask to join. An owner or admin approves with a role (default viewer;
  only owners grant owner) or declines. Organisations stay invisible to
  others; invite-only ones aren't listed at all.
- Each step is audit-logged: `org.create`, `tenant_domain.claim/verify/delete`,
  `org.join_policy`, `join_request.create/approve/decline`.

## Not done (yet)
- Auto-join (members at a verified domain joining as viewer without
  approval), and enforcing that every account at a domain belongs to the
  organisation. Enforcement needs SSO and belongs in a paid tier.
- Retrying unverified claims in the background. Owners press Verify, which
  is how most products do it.
- An admin UI for disputes; staff use SQL as for platform admins.

## Consequences
- Customers can onboard themselves and test on the trial; joining through a
  domain needs DNS proof or staff.
- Each verify press costs one DNS lookup (5 s timeout) and is covered by the
  usual per-IP rate limit.
