# Connectors: Slack, GitHub, Linear, Sentry, and Notion

Connectors sync content from the tools your team uses into an agent's
memory, and keep it current. Each connection belongs to the dashboard user
who made it; their memories are private to their agent, like any other.

| | Slack | GitHub | Linear | Sentry | Notion |
|---|---|---|---|---|---|
| What becomes memory | Messages in the channels you turn on, one item per channel per day, threads kept together | Issues and pull requests with their comments | Issues, projects, priorities, and discussion from teams | Production issues, exceptions, stack traces, and culprit files | Specs, architectural decisions, RFCs, and database pages |
| Kept current by | Events API (new, edited, deleted messages), plus a sync every 30 minutes | App webhooks, plus a sync every 30 minutes | Webhooks, plus reconciliation sync | Webhooks, plus reconciliation sync | Webhooks, plus reconciliation sync |
| History on connect | Last `CONNECTOR_BACKFILL_DAYS` (30) | Items updated in the last `CONNECTOR_BACKFILL_DAYS` | Active issues updated in last 30 days | Unresolved issues in the last 14 days | Active pages in shared databases |
| You choose | Channels, in the Memro dashboard | Repositories, on GitHub | Teams, in the Memro dashboard | Projects, in the Memro dashboard | Databases and pages shared with integration |
| Left out | Bot messages, joins/leaves and other system messages | Bot comments, template comments (`<!-- -->`) | Automated bot noise | Archive events | Archived/trashed pages |
| Memory type | `episodic` | `semantic` | `semantic` | `episodic` | `semantic` |

Memories made by a connector have `metadata.source` (`slack`, `github`, `linear`, `sentry`, or `notion`),
`metadata.connector: true`, `metadata.url` and source details (channel,
repo, team, issue, page). Every memory starts with a structured header (e.g.
`[Slack #eng · 2026-10-20 · Acme]`, `[GitHub acme/api#42 · Title]`, `[Linear · ENG · ENG-123 · Title]`, `[Sentry · backend · Issue #9412 · Error]`, or `[Notion · Workspace · RFC Title]`).

### Cross-Source Context Mesh
Memro automatically parses ticket keys (`ENG-123`), PR/Issue shorthand (`#42`), GitHub/Linear URLs, and Sentry references (`Sentry #1234`). When a discussion in Slack or Linear references a GitHub PR or Sentry bug, or a Notion spec outlines architecture for a ticket, Memro forms bidirectional semantic relationships (`derives`, `cross_source`) in `memory_relations`, allowing agents to retrieve cross-tool causal chains seamlessly.

**Behaviour you can rely on**

- Long items are split into chunks like uploaded documents. When an item
  changes, only chunks whose text changed are replaced; the rest keep their
  memory and embedding.
- A memory you delete is not recreated unless its text changes.
- Deleted at the source → deleted in Memro (deleted Slack messages, deleted
  GitHub issues).
- Content counts against the agent's memory limit. When the limit is reached
  the connection pauses with a message; free up space and press **Resume**.
- Disconnecting releases Memro's access at the provider. Memories already
  made stay unless you tick *Also delete the memories it created*.
- Synced text is written by other people. Treat it as untrusted input in
  your agents, as with any memory (see [ADR 0010](decisions/0010-connector-hardening.md)).

## Using them (dashboard)

**Integrations → Connect Slack**: approve the Memro app for your workspace,
then pick channels under **Channels**. Nothing is read until you pick one.
Public channels are joined automatically. A private channel is listed only
if you are a member and someone has invited the Memro app to it
(`/invite @Memro`).

**Integrations → Connect GitHub**: install the Memro GitHub App on your
account or organization and choose repositories. Memro syncs the chosen
repositories that your GitHub account can read. Change the selection later
with **Repositories** (it opens GitHub). An organization may need an owner to
approve the app first.

## API

Connector endpoints need a dashboard session; API keys and signed agent
requests get `403`. See the [API reference](api-reference.md#connectors).

## Setting up the connectors (self-hosting)

Connectors are off until their credentials are set. All of them need an
encryption key for provider tokens at rest:

```bash
CONNECTOR_ENCRYPTION_KEY=$(openssl rand -base64 32)
```

Keep it secret and stable; without it stored tokens can't be read. To
rotate, move the old value to `CONNECTOR_ENCRYPTION_KEY_PREVIOUS` and set a
new one; tokens are re-encrypted with the new key as they are used.

Provider redirects go to the **dashboard's origin**, which must proxy
`/api/*` to the API (the bundled dashboard images do). Memro sets a cookie
when you press *Connect* and checks it on the way back, so a link someone
else started can't attach their account to yours. Override the callback base
with `CONNECTOR_CALLBACK_BASE_URL` (default `$APP_URL/api`).

Below, `APP` is your dashboard URL (e.g. `https://app.memro.co`) and `API`
your API URL (e.g. `https://api.memro.co`). Webhooks go to the API directly.

### Slack

1. <https://api.slack.com/apps> → **Create New App** → **From a manifest**,
   and paste (replace the two URLs):

   ```yaml
   display_information:
     name: Memro
     description: Gives your AI agents memory of the channels you choose.
   features:
     bot_user:
       display_name: Memro
       always_online: false
   oauth_config:
     redirect_urls:
       - APP/api/connectors/slack/callback
     scopes:
       bot:
         - channels:history
         - channels:read
         - channels:join
         - groups:history
         - groups:read
         - users:read
         - team:read
   settings:
     event_subscriptions:
       request_url: API/connectors/slack/events
       bot_events:
         - message.channels
         - message.groups
         - app_uninstalled
         - tokens_revoked
         - channel_rename
         - group_rename
     org_deploy_enabled: false
     socket_mode_enabled: false
     token_rotation_enabled: false
   ```

   Start the server with the Slack settings (step 3) before saving, so Slack
   can verify the events URL.
2. **Manage Distribution → Activate Public Distribution** if workspaces other
   than yours will connect.
3. From **Basic Information** set:

   | Variable | From |
   |---|---|
   | `SLACK_CLIENT_ID` | App Credentials → Client ID |
   | `SLACK_CLIENT_SECRET` | App Credentials → Client Secret |
   | `SLACK_SIGNING_SECRET` | App Credentials → Signing Secret |

Memro never posts; the scopes are read-only apart from `channels:join`,
used to join public channels a user picks. Token rotation is supported if
you turn it on, but it's not needed.

Slack limits how often apps that are not in the Slack Marketplace may read
channel history. Live messages arrive through events and are unaffected;
the initial backfill of a busy channel can take a while. Memro waits out
Slack's `Retry-After` and resumes where it stopped.

### GitHub

1. GitHub → **Settings → Developer settings → GitHub Apps → New GitHub App**
   (under the organization's settings to have it owned by the org):

   | Setting | Value |
   |---|---|
   | Homepage URL | your site |
   | Callback URL | `APP/api/connectors/github/callback` |
   | Expire user authorization tokens | ✓ |
   | Request user authorization (OAuth) during installation | ✓ (required) |
   | Redirect on update | ✓ |
   | Webhook → Active | ✓ |
   | Webhook URL | `API/connectors/github/webhook` |
   | Webhook secret | a random string (`openssl rand -hex 32`) |
   | Repository permissions | **Metadata: Read**, **Issues: Read**, **Pull requests: Read** — nothing else |
   | Subscribe to events | Issues, Issue comment, Pull request, Pull request review, Repository |
   | Where can this app be installed | Any account (or only this account for internal use) |

2. After creating it, **Generate a private key** and note the App ID, the
   app's slug (its URL is `github.com/apps/<slug>`) and the Client ID, and
   **Generate a new client secret**.
3. Set:

   | Variable | |
   |---|---|
   | `GITHUB_APP_ID` | App ID |
   | `GITHUB_APP_SLUG` | `<slug>` |
   | `GITHUB_APP_PRIVATE_KEY` | Contents of the `.pem` file (newlines may be written as `\n`) |
   | `GITHUB_CLIENT_ID` | Client ID (`Iv1…`) |
   | `GITHUB_CLIENT_SECRET` | Client secret |
   | `GITHUB_WEBHOOK_SECRET` | The webhook secret |

When someone connects, Memro checks with their own GitHub token that they
can see the installation, and syncs only repositories they can read. It
revokes that token on disconnect; uninstalling the app from GitHub stops all
syncing for that account.

### Other settings

| Variable | Default | |
|---|---|---|
| `CONNECTOR_BACKFILL_DAYS` | `30` | History synced when a channel or repository is added (`0`: only new content) |
| `CONNECTOR_SYNC_INTERVAL_MINUTES` | `30` | Reconciliation sync in addition to webhooks |
| `CONNECTOR_WORKERS` | `4` | Parallel connector jobs per server instance |

## Operating them

- Work runs as jobs in the `connector_jobs` table. Every API instance runs
  workers; rows are claimed with `FOR UPDATE SKIP LOCKED`, so several
  instances (blue-green deploys, replicas) share the queue safely.
- Jobs retry with backoff (30 s doubling to 1 h, 8 attempts). Rate limits
  wait for the provider's `Retry-After` without using an attempt. If the
  provider revokes access the connection becomes *Disconnected*; if a sync
  keeps failing it shows *Needs attention* with the error.
- Metrics: `memro_connector_jobs_total{kind,result}`,
  `memro_connector_webhooks_total{provider,result}` (watch
  `bad_signature`), `memro_connector_memories_total{provider,action}`,
  `memro_connector_jobs_due`.
- Webhook endpoints are exempt from the per-IP rate limit (they are
  authenticated by signature and come from a few provider addresses).
