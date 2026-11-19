# 0037. A repo's rules live in the repo, in `.memro/rules.md`
Date: 2026-11-19 · Status: Accepted

## Context
ADR 0035's hooks save the rules a user states ("never…", "always…") to their
Memro account and show them at every prompt. The 2026-11-19 depth grid showed
that pushing rules is what works (150/150, against 139/150 for a notes file
and 58/150 for search alone), but the user couldn't see what the hook had
saved or fix a wrong rule, and the rules needed an account. Hacker News
threads on agent memory (2026-11) said the same: people delete most of what
auto-memory saves and trust markdown they can read. Syncing one rules file
to every tool's format is already free (rulesync and others).

## Decision
- The hook appends each rule it catches to `.memro/rules.md` at the repo's
  root, one `- rule` line each, in the user's words. The file is the source
  of truth: the user sees new rules in `git diff`, edits or deletes lines by
  hand, and commits it; clones and teammates get it through git.
- At every prompt the hook shows the file's rules, plus rules saved in the
  account before the file existed (`GET /rules`), without duplicates, so no
  existing rule stops reaching agents.
- `memro rules` lists the file's rules and those account rules;
  `rules edit <n> <text>` and `rules delete <n>` change a line;
  `rules import` moves the account's rules into the file and deletes them
  from the account.
- The server no longer stores new rules. `GET /rules` stays for the old ones.

## Consequences
- Agents' working trees gain `.memro/rules.md` changes; an agent committing
  its work may commit a new rule with it, which puts the rule in review.
- A rule caught in a worktree lands in that worktree's file and reaches the
  others when it's merged, not at once.
- Without an account (`memro init`), the hooks still save, show and check
  the file's rules; claims, statuses and memory need `memro connect`.
- Enforcement, warn only: a "never…" rule naming something in backticks is
  checked against what each edit or command adds (Claude Code edits, Codex
  and OpenCode patches, shell commands), whole-term, and a match adds a note
  with the rule. Every warning is logged to `~/.memro/hooks/log.jsonl`
  (`rule-warn`), so the false-positive rate can be measured before a block
  mode. Gemini CLI and Cursor pass only the file path and can't show a note
  on an edit, so they get no warnings yet.
- The relay benchmark's Memro arms seed rules in the account, which the hook
  still shows; a rerun should seed the file instead.
- Two hook processes adding the same rule at once may write it twice; reading
  removes duplicates.
