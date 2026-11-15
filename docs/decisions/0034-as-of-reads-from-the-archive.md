# 0034. "As of" reads from the archive
Date: 2026-11-15 · Status: Accepted

## Context
Reviewers of Memro ask to "reconstruct what the agent knew at a past time".
ADR 0032 plans as-of reads over structured facts (step 4), with valid time
(when a fact was true) and recorded time (when Memro believed it). Step 1
already keeps every deleted or replaced memory in `memory_archive` with its
`archived_at`, and the live row's `created_at` says when Memro learned it.
That is enough for recorded time on memories, without waiting for facts.

## Decision
`GET /memory/agent/{agent_id}?as_of=<RFC 3339>` lists what the agent knew at
that time: live memories created by then, plus archived memories created by
then and archived after it. Other filters and the limit apply as usual.

## Alternatives considered
- **Wait for step 4.** Rejected: recorded-time reads need only data we
  already keep, and step 4 adds valid time on top.
- **As-of search.** Not now: archived memories have no embeddings (they are
  removed with the live row), so a search could not rank them the same way.

## Consequences
- Purged memories never appear, as intended for privacy requests.
- A memory archived and later restored counts as known while it was archived
  (the restore keeps its original `created_at`).
- Valid time ("the user lived in A from 2010 to 2015") still needs step 4;
  until then `event_date` filters on `/search/temporal` are the closest tool.
