# 0029. A Staff Console for Running Memro
Date: 2026-11-14 · Status: Accepted · Builds on 0012, 0028

## Context
ADR 0028 put an Admin page in the dashboard's sidebar. That page lived inside
whatever account the staff member was signed into (locally, a customer's), and
it only listed organisations. Running Memro needs a view of every account,
kept apart from any customer's.

## Decision
- **A separate console at `/admin`**, with its own header and no account
  sidebar. Staff sign in with their Memro staff address. Anyone else who opens
  it sees "This console is for Memro staff". The dashboard's sidebar item for
  staff only links there.
- **Overview** (`GET /admin/overview`):
  - people, plus those who signed in within 30 days, and staff
  - organisations by trial, contract and trials ending within 7 days or
    ended
  - agents and end users
  - memories, stored tokens, and queries this month
- **Organisations** (`GET /admin/tenants`): everything from ADR 0028 (plans,
  limits, trials, creating organisations).
- **Users** (`GET /admin/users?q=`): every person, newest first or by email,
  with:
  - when they joined and last signed in
  - their personal agents and storage
  - their organisations and roles
  - whether they are staff
- **Staff rights** (`PUT`/`DELETE /admin/staff/{user_id}`): only staff grant
  and revoke them. Nobody removes their own, the last staff member can't be
  removed, and each change is audit-logged (`staff.grant`, `staff.revoke`).
- As before, every endpoint requires a platform admin's dashboard session;
  API keys and signed requests get `403`.

- **Detail pages** (`GET /admin/users/{id}`, `GET /admin/tenants/{id}`):
  click a person or organisation for everything about it.
  - A person: their agents, API keys, recent sign-ins, organisations, usage
    by month and audit log.
  - An organisation: its members, invitations, domains, keys, plan, usage by
    month and audit log.
- **Account actions:** each one is audit-logged as `admin.user.*` or
  `admin.tenant.*`.
  - **Suspend / unsuspend** (`POST …/{id}/suspend` with a reason,
    `…/unsuspend`):
    - Every request the person or organisation makes gets `403` with
      `Memro-Account: suspended` and the reason: sessions, API keys, server
      keys, signed requests.
    - Sign-in is refused too, but only after a correct code, so it doesn't
      reveal who has an account.
    - Nothing is deleted.
    - Checked per request, cached for 30 seconds. Suspending clears that
      instance's cache at once. A database error fails open, so a hiccup
      can't lock everyone out.
  - **Sign out everywhere** (people) and **revoke all keys** (both).
  - **Delete** (`DELETE …/{id}` with the email or name typed back):
    - A person goes with their own agents' memories and files, but not
      while they are the only owner of an organisation.
    - An organisation goes with every agent's memories and files, its
      members, keys, domains, limits and audit log.
  - Staff can't act on themselves, and can't suspend or delete staff until
    their rights are removed.
- **Plans** (memro-cloud ADR 0013): the drawer shows the person's or
  organisation's tier from Memro Cloud. Staff can give one free for 1–12
  months with a reason, extend it, or end it. Organisations' contracts and
  trials stay under Manage.
- **Activity** (`GET /admin/activity?action=`):
  - sign-ups and new organisations per day for 30 days
  - queries per month
  - the newest 100 audit events with who did them, filtered by kind (staff
    actions, plans given, contracts and trials, staff rights,
    organisations)
- **Revenue** on the overview (memro-cloud): Dodo MRR by tier, contracts at
  list price, and accounts that aren't paying (complimentary, x402-prepaid,
  payment failing).

## Not done (yet)
- Changing a paying Dodo subscription from the console; staff do that in
  Dodo.
- A read-only look inside an end user's memories for support. That needs
  its own privacy decision.
- Pagination beyond 500 rows, and precomputed figures. The counts are marked
  `shortcut:` for when Memro is large enough to need it.
