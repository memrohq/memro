# Connectors

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

## Token connectors (the catalog)

Besides the sign-in connectors above (Slack, GitHub, Linear, Sentry, Notion,
Jira, GitLab), 43 services connect with an API token the user pastes in the
dashboard. Each is one manifest in
[`server/internal/connectors/dce/catalog`](../server/internal/connectors/dce/catalog)
run by the declarative connector engine ([ADR 0014](decisions/0014-declarative-connector-engine.md)).

- **Connecting checks the token** with a cheap request to the service and
  names the account; a wrong token or field is refused with a message and
  nothing is saved.
- **The token is encrypted** (AES-256-GCM with `CONNECTOR_ENCRYPTION_KEY`,
  bound to the connection) and stored in Postgres with the connection. It is
  never returned by the API, logged, or put in Secret Manager. Secret Manager
  holds only the server's own keys, so the number of users does not matter.
- **Syncs are incremental where the service allows it** (a checkpoint sent as
  a filter), page through in jobs of 5 pages, and resume where they stopped
  after a restart. Services without a filter are re-read every 30 minutes;
  unchanged items cost nothing (chunk hashes).
- **A refused token (401) marks the connection disconnected**; the user
  reconnects with a new one.
- Requests go only to the service's own host: private, loopback and metadata
  addresses are refused when connecting, and redirects to another host are
  not followed.

| Area | Connector | What becomes memory | You enter | Sources |
|---|---|---|---|---|
| Engineering | Bitbucket | Pull requests from the Bitbucket repositories you choose. | Atlassian account email, API token, Workspace ID | you choose |
| Engineering | Buildkite | Builds across your organization's pipelines, with state, branch and commit. | API access token, Organization slug | everything the token reads |
| Engineering | CircleCI | Pipelines with their branch, commit and trigger. | Personal API token, Organization slug | everything the token reads |
| Engineering | Datadog | Monitors with their queries, messages and current state. | Datadog site, API key, Application key | everything the token reads |
| Engineering | Grafana | Dashboards in your Grafana instance, with folders and tags. | Grafana host, Service account token | everything the token reads |
| Engineering | Netlify | Deploys of the sites you choose, with branch, commit and errors. | Personal access token | you choose |
| Engineering | New Relic | Alert issues with their priority, state, entities and condition. | User key, Account ID, API host | everything the token reads |
| Engineering | Opsgenie | Alerts with their priority, status, owner and tags. | API key, Region (EU accounts: eu.) | everything the token reads |
| Engineering | PagerDuty | Incidents with their service, urgency, status and responders. | API key | everything the token reads |
| Engineering | Postman | Your Postman collections and who owns them. | API key | everything the token reads |
| Engineering | Render | Deploys of the services you choose, with status and commit. | API key | you choose |
| Engineering | Snyk | Open security issues in your Snyk organization, with severity and status. | API token, Organization ID, API host | everything the token reads |
| Engineering | SonarQube Cloud | Code issues across your organization's projects, with severity and rule. | User token, Organization key | everything the token reads |
| Engineering | Supabase | Your Supabase projects with their region, status and database version. | Personal access token | everything the token reads |
| Engineering | Vercel | Deployments with their target, state, branch and commit. | Access token, Team ID | everything the token reads |
| Knowledge | Airtable | Records from the tables of one Airtable base, every field included. | Personal access token, Base ID | you choose |
| Knowledge | Coda | The page outline of the Coda docs you choose. | API token | you choose |
| Knowledge | Confluence | Pages from the Confluence spaces you choose, as plain text. | Site name, Atlassian account email, API token | you choose |
| Knowledge | Figma | Design files in the Figma team projects you choose. | Personal access token, Team ID | you choose |
| Knowledge | Outline | Every document in your Outline workspace, in Markdown. | Outline host, API key | everything the token reads |
| Knowledge | Raindrop.io | Your bookmarks with excerpts, notes, highlights and tags. | Test token | everything the token reads |
| Knowledge | Readwise | Your books and articles with every highlight and note. | Access token | everything the token reads |
| Knowledge | Zotero | Papers and sources in your Zotero library, with authors and abstracts. | API key, User ID | everything the token reads |
| Work | Asana | Tasks assigned to you in the Asana workspaces you choose. | Personal access token | you choose |
| Work | ClickUp | Tasks, including closed ones and subtasks, from the workspaces you choose. | Personal API token | you choose |
| Work | Harvest | Time entries with project, task, client, hours and notes. | Personal access token, Account ID | everything the token reads |
| Work | monday.com | Items from the monday.com boards you choose, with every column. | Personal API token | you choose |
| Work | Plane | Work items from the Plane projects you choose. | API key, Workspace slug, API host | you choose |
| Work | Shortcut | Stories with their type, state, labels and description. | API token | everything the token reads |
| Work | Todoist | Your open Todoist tasks with due dates, priorities and labels. | API token | everything the token reads |
| Work | Trello | Cards from the Trello boards you choose, with lists, labels and due dates. | API key, Token | you choose |
| Work | Wrike | Tasks with their status, importance, dates and description. | Permanent access token, Data center host | everything the token reads |
| Work | YouTrack | Issues with their project, fields and description. | YouTrack host, Permanent token | everything the token reads |
| Communication | Close | Leads with their status, contacts and description. | API key | everything the token reads |
| Communication | Discord | Messages from the text and announcement channels you choose in one server. | Bot token, Server ID | you choose |
| Communication | Freshdesk | Support tickets with their status, priority, tags and description. | Freshdesk domain, API key | everything the token reads |
| Communication | Front | Conversations with their status, assignee, tags and latest message. | API token | everything the token reads |
| Communication | Gong | Recorded calls with their title, time, duration and scope. | API base host, Access key, Access key secret | everything the token reads |
| Communication | Gorgias | Support tickets with their status, channel, customer and summary. | Gorgias domain, Your email, API key | everything the token reads |
| Communication | HubSpot | Contacts, companies and deals from your HubSpot CRM. | Private app access token | everything the token reads |
| Communication | Intercom | Customer conversations with their state, assignee and first message. | Access token, Region | everything the token reads |
| Communication | Pipedrive | Deals with their value, stage, status, organization and contact. | API token | everything the token reads |
| Communication | Zendesk | Support tickets with their status, priority, tags and description. | Subdomain, Agent email, API token | everything the token reads |

Dropped from the earlier list because they cannot work with a token today:
Pocket and Height (shut down), Obsidian Publish (no API), Basecamp and Help
Scout (OAuth apps only), Railway and GitBook (no stable list API for the
content), Slite (undocumented response format).

## Using them (dashboard)

**Integrations → Connect Slack**: approve the Memro app for your workspace,
then pick channels under **Channels**. Nothing is read until you pick one.
Public channels are joined automatically. A private channel is listed only
if you are a member and someone has invited the Memro app to it
(`/invite @Memro`).

**Linear, Sentry, Notion, Jira, GitLab**: sign in with the provider, then
choose teams, projects or databases with the button on the connection. As
with Slack, nothing is read until you choose.

**Token connectors**: fill in the form (the token and, for some, a site or
account ID; each field says where to find it). Connectors that list sources
(projects, boards, spaces) show a button to choose them; the others sync
everything the token can read, so prefer a read-only token scoped to what
you want in memory.

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

### Linear

1. Linear → **Settings → API → My applications → Create new application**:
   - Callback URL: `APP/api/connectors/linear/callback`
   - Webhook URL: `API/connectors/linear/webhook`
2. Set:

   | Variable | From |
   |---|---|
   | `LINEAR_CLIENT_ID` | Application Client ID |
   | `LINEAR_CLIENT_SECRET` | Application Client Secret |
   | `LINEAR_WEBHOOK_SECRET` | Webhook Secret for signature verification |

### Sentry

1. Sentry → **Settings → Developer Settings → Custom Integrations → Create New Integration**:
   - Authorize URL: `APP/api/connectors/sentry/callback`
   - Webhook URL: `API/connectors/sentry/webhook`
   - Permissions: Project: Read, Issue & Event: Read, Organization: Read
   - Alert Rule Action: Enable issue webhooks
2. Set:

   | Variable | From |
   |---|---|
   | `SENTRY_CLIENT_ID` | Client ID |
   | `SENTRY_CLIENT_SECRET` | Client Secret |
   | `SENTRY_WEBHOOK_SECRET` | Webhook verification secret |

### Notion

1. Notion → **[notion.so/my-integrations](https://www.notion.so/my-integrations) → New Integration**:
   - Type: Public Integration
   - Redirect URI: `APP/api/connectors/notion/callback`
   - Capabilities: Read content, Read user information including email
2. Set:

   | Variable | From |
   |---|---|
   | `NOTION_CLIENT_ID` | OAuth Client ID |
   | `NOTION_CLIENT_SECRET` | OAuth Client Secret |
   | `NOTION_WEBHOOK_SECRET` | Webhook verification secret |

### Jira

1. [developer.atlassian.com/console/myapps](https://developer.atlassian.com/console/myapps) → **Create → OAuth 2.0 integration**:
   - Permissions → Jira API: `read:jira-work`, `read:jira-user`; User identity API: `read:me`
   - Authorization → Callback URL: `APP/api/connectors/jira/callback`
2. Set `JIRA_CLIENT_ID` and `JIRA_CLIENT_SECRET`. Access tokens last an
   hour; Memro refreshes them (Atlassian rotates the refresh token each time).
3. Webhooks are optional. To use them, register a webhook to
   `API/connectors/jira/webhook` with a secret and set `JIRA_WEBHOOK_SECRET`;
   only HMAC-signed deliveries are accepted. Without it the 30-minute sync
   keeps issues current.

### GitLab

1. GitLab → **Preferences → Applications** (or the group's or instance's):
   - Redirect URI: `APP/api/connectors/gitlab/callback`
   - Scopes: `read_api`, `read_user`, `openid`
2. Set `GITLAB_CLIENT_ID` and `GITLAB_CLIENT_SECRET`. Tokens last two hours
   and are refreshed.
3. Webhooks are optional: add a project or group webhook to
   `API/connectors/gitlab/webhook` with secret token `GITLAB_WEBHOOK_SECRET`
   (merge request and pipeline events).

### Webhooks and token refresh (all sign-in connectors)

- A webhook endpoint answers `503` until its secret is set, `401` to
  anything not signed with it, and acts only for connections whose user
  chose that team, project or database. The item is then re-read with that
  connection's own token, so a webhook never decides what is stored.
- Notion sends a one-time verification token when a webhook subscription is
  created. With `NOTION_WEBHOOK_SECRET` unset, the server logs it once
  (`notion webhook verification request`); set the secret to it, redeploy,
  and confirm the subscription in Notion.
- Expiring tokens (Jira, GitLab, Sentry, newer Linear apps) are refreshed
  five minutes before they expire and once more after a `401`. A refresh
  holds a lock on the connection so concurrent workers never spend a
  rotated refresh token twice. Only a refused refresh disconnects.

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
