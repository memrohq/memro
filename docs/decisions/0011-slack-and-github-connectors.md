# 0011. Slack and GitHub connectors
Date: 2026-10-20 · Status: Accepted

## Context
Teams want their agents to remember what is already written down in Slack
and GitHub without copying it in by hand. Doing this for real means holding
long-lived provider credentials, receiving webhooks from the internet,
syncing large histories under provider rate limits, and keeping memories
correct as content is edited and deleted. It must run on our blue-green
deploys (two API instances for a short time) and stay safe when several
Memro users connect the same Slack workspace or GitHub organization.

## Decision
- **A connection belongs to the dashboard user's agent**, and its content
  becomes that agent's private memories (`memories.connection_id`,
  `source_item`). Connections are managed with a dashboard session only;
  API keys cannot connect accounts.
- **Slack: OAuth v2 bot token + Events API.** Read-only scopes plus
  `channels:join`. Channels are opt-in. Private channels are only listed and
  synced when the connecting Slack user is a member (checked on selection
  and every sync), because the bot token is shared by everyone who connected
  the workspace. Events are applied to a stored per-channel-day document
  (versioned by edit/delete time, so out-of-order and duplicate events
  converge) instead of re-reading history, which Slack rate-limits heavily
  for apps outside its Marketplace.
- **GitHub: a GitHub App** (not an OAuth App) with Metadata, Issues and Pull
  requests read-only, one signed app webhook and short-lived installation
  tokens. "Request user authorization during installation" is required: the
  callback's `installation_id` is only accepted if the user's own token can
  see that installation, and only repositories that user can read are
  synced. Items are re-fetched from the API on each webhook rather than
  trusting payloads.
- **OAuth state is single-use, expires in 10 minutes, is bound to the agent
  (hashed in `connector_oauth_states`) and to the browser** (an HttpOnly,
  SameSite=Lax cookie compared with the returned `state`). Callbacks
  therefore go through the dashboard origin (`APP_URL/api/...`).
- **Credentials are sealed with AES-256-GCM** (`CONNECTOR_ENCRYPTION_KEY`),
  with the connection ID as associated data and a key ID prefix so a
  previous key still decrypts during rotation.
- **Webhooks**: Slack v0 signatures with a 5-minute window; GitHub
  `X-Hub-Signature-256`. Both only enqueue work and answer immediately, and
  are exempt from the per-IP rate limit.
- **All provider work is small jobs in Postgres** (`connector_jobs`), claimed
  with `FOR UPDATE SKIP LOCKED` under a lease, coalesced by key, retried
  with backoff, paused on `Retry-After`. One history page per job, with the
  cursor saved, so long backfills resume after restarts and rate limits.
- **Memories are diffed per chunk**: unchanged chunks keep their memory and
  embedding; chunk hashes per item remember which chunks the user deleted so
  they are not recreated. Quota is enforced in the same transaction; hitting
  it pauses the connection with a message instead of failing silently.

## Alternatives considered
- **GitHub OAuth App with per-repo webhooks.** Needs broad `repo` scope and
  admin rights to install hooks; a GitHub App is least-privilege and
  user-selectable.
- **Slack user tokens instead of a bot token.** Would let a connection read
  everything the user can, including DMs, which is more than we want, and
  needs each user to authorize. The bot sees only channels it is in.
- **Re-reading Slack history on every event.** Simple, but
  `conversations.replies`/`history` are tightly rate-limited for
  non-Marketplace apps; storing the channel-day document avoids API calls
  on the realtime path.
- **An external queue (Pub/Sub, Redis).** Postgres already gives durable,
  transactional jobs at our volume without a new dependency.
- **One memory per Slack message.** Produces thousands of tiny memories with
  no context; a channel-day chunked like a document recalls better.

## Consequences
- New secrets per environment: encryption key, Slack client ID/secret and
  signing secret, GitHub App ID/slug/private key, client ID/secret and
  webhook secret. Losing the encryption key means every connection must be
  reconnected.
- The Slack app and GitHub App must be created by us, with callback URLs on
  the dashboard origin and webhooks on the API (docs/connectors.md).
  Distributing the Slack app publicly needs Slack's review for Marketplace
  listing to lift the history rate limits.
- Synced content is written by other people and is untrusted input to
  agents (ADR 0010 applies).
- When an agent is at its memory limit, queued work for that connection is
  dropped; Resume re-syncs from the saved cursors, but Slack edits made
  while paused are only picked up if they fall inside a later sync window.
- Still open: Slack Enterprise Grid org-wide installs (rejected for now);
  GitHub repositories with issues disabled (their PRs are not synced yet);
  discussions, wikis and code are not synced; connector management via API
  keys for automation.
