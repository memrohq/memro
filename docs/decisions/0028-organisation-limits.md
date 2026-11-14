# 0028. Organisation Plans, Trials and the Admin Portal
Date: 2026-11-14 · Status: Accepted · Builds on 0025, 0026, 0027 · Pricing: memro-cloud ADR 0012

## Context
Every agent had the same memory cap: `MEMORY_SOFT_CAP`, or whatever quota
policy a distribution plugs in. A B2B customer may have 10,000 users behind
one server key, and an active one would reach that cap within months. Their
backend treats refusals as non-fatal, so their product would quietly stop
learning.

We also want:
- a free trial that companies can't farm
- plans in the same units as Memro Cloud's public tiers: stored tokens and
  queries
- a place where Memro staff run all of this, not SQL

## Decision
- **One server key per organisation, not one per user.** `X-Memro-User`
  already gives each user separate memory (ADR 0025).
- **Organisations buy the public tiers** (Free, Builder, Pro, Scale) through
  Memro Cloud's checkout, as the organisation. All its end users share the
  tier, which Memro Cloud's quota policy enforces (memro-cloud ADR 0012).
  Nothing about those tiers lives here.
- **`tenant_limits` holds what isn't sold by checkout:** the free trial, and
  Enterprise or custom contracts. Its fields are hard limits (`NULL` =
  none), all shared by the organisation's users:
  - stored tokens per user and in total (bytes of memory text / 4, the
    public tiers' unit)
  - queries per calendar month (UTC)
  - seats (members plus pending invitations)
  - whether image analysis is allowed
  - when a trial ends

  | Plan | Hard limits in the server | Billed by memro-cloud |
  |---|---|---|
  | trial | the Free tier: 100k stored tokens, 5k queries a month, plus 3 seats, no images, 30 days | nothing |
  | enterprise | none | from $1,500 with 100M tokens and 5M queries included, then overage |
  | custom | as staff set | as Enterprise |

  A contract's usage is billed, never refused. Staff can change any field
  per organisation. When an organisation's Dodo subscription becomes active,
  Memro Cloud removes its trial row, so the bought tier applies.
- **A plan replaces the unclaimed-agent cap and the quota policy** for the
  organisation's agents. Tenant-key users are never "claimed". Organisations
  without a plan behave exactly as before.
- **Self-service organisations start on the trial, one per company.** The
  creator's email domain is recorded in `trial_domains`. A second
  organisation from the same domain starts with its trial already ended
  (`trial_started: false`). This replaces ADR 0027's rule that keys need a
  verified domain, because the trial bounds cost and a key only reaches its
  own organisation.
- **After a trial ends:**
  - The organisation is read-only: searching and deleting work, saving
    doesn't.
  - Owners are emailed when it ends, then 14, 7 and 1 days before deletion.
  - 60 days after the end, an hourly job deletes the organisation's agents
    with their memories and files. The organisation, members and keys stay,
    so it can still be upgraded.
  - Each stage is recorded (`trial_notice`) before its email goes out, so
    none is sent twice. Changing the plan or the date restarts the stages,
    and an organisation no longer on the trial is never deleted.
- **Warnings at 80%:** the response carries a `Memro-Limit-Warning` header
  (e.g. `stored_tokens_total=85%`), and the owners get one email (until the
  limits change).
- **Refusals say why:** `403` with a message and a `Memro-Limit` header:
  - `stored_tokens_per_user`, `stored_tokens_total` or `queries_monthly`
  - `members`, `images` or `trial_ended`
  Not `429`, because waiting doesn't lift these limits and the SDKs retry
  429s.
- **Billing samples storage hourly:** each agent's stored tokens are
  recorded as this month's `stored_tokens_held`, keeping the peak. So the
  statement charges for what was held, and members' usage shows it.
- **Admin portal** (dashboard → Admin, platform admins only; `whoami`
  reports `platform_admin`):
  - Every organisation with its plan, trial state, stored tokens against the
    pool, queries this month, users, members, domains and whether it is
    self-service.
  - Change the plan or any limit, extend a trial (+14 or +30 days, or a
    date), reset to the defaults, and create organisations with an owner
    invitation.
  - API: `GET /admin/tenants`, `PUT`/`DELETE /tenant/{id}/limits`. Every
    change is audit-logged with the values set.
  - Members see their plan and usage on the Organization page
    (`GET /tenant/{id}/limits`).

## Not done (yet)
- An email before a trial ends (only after).
- Ingest limits for organisations, and self-service upgrades and payment.
  Plans are sold, then set by staff.
- Scale. The trial's shared pool is a `SUM` per write, the storage sample is
  one `GROUP BY` an hour, and the admin list sums the newest 500
  organisations. All are fine now and marked `shortcut:` with when to change
  them.

## Consequences
- Vaultnel is on `enterprise`: no hard limits, billed per memro-cloud ADR
  0012. Smaller customers buy Builder, Pro or Scale themselves.
- Each write or search in a dedicated tenant costs one primary-key lookup,
  plus a sum when the trial's shared limits apply.
