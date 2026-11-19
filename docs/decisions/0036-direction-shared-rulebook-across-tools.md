# 0036. Direction: capture, delivery and enforcement, the shared rulebook across agent tools
Date: 2026-11-19 · Status: Proposed

## Context
Four benchmarks so far show where Memro adds value and where it doesn't:

| Benchmark | Result | Source |
|---|---|---|
| LongMemEval-100 | Memro 81%, plain semantic RAG 81% (McNemar p = 1.0) | journal 2026-11-15 |
| Coordination, one folder | Memro ties an uncoordinated shared folder, 98/100 tasks landed each: Claude Code's Edit refuses stale writes | journal 2026-11-17 |
| Relay, 300 facts | Memro + hooks 10/10, NOTES.md 9/10, Memro tools alone 8/10 | journal 2026-11-17 §8, ADR 0035 |
| Relay, rule at 25/50/75% of 3,000 facts, 150 trials per arm | Memro + hooks 150/150, NOTES.md 139/150 (p = 0.056 at the closest depth, short of the p < 0.01 bar: a tie), Memro search alone 58/150, Claude Code auto-memory 0/150 | journal 2026-11-19 |

The pattern: Memro's search is no better than plain RAG, and alone it finds a
buried rule about half the time. Memro wins when the hooks capture what the
user decided and put it in front of every agent, without anyone keeping a file.
The notes file keeps up mainly because agents grep it for the very thing they
are about to use. A win there would show "rules kept apart and pushed beat
rules mixed in and pulled", which a rules section in CLAUDE.md could also do.

Every agent vendor now ships its own memory (Claude Code auto-memory and
CLAUDE.md, Codex, Cursor and Gemini equivalents), and it will improve. Each
works only inside its own tool and mostly on one machine. Teams use several
tools, several machines and several people.

## Decision
Memro positions itself as the tool-neutral layer a team's agents share, not a
better memory inside one agent:

1. **One rulebook across tools.** Decisions and rules a user states are saved
   by the hooks and shown to every agent in that repo, in Claude Code, Codex,
   Cursor, Gemini CLI and OpenCode alike, keyed by the repo's origin (ADR 0035).
2. **Team memory.** A teammate's fresh clone, a new hire or a CI agent starts
   with what earlier sessions learned (organisation accounts, ADRs 0025–0028).
3. **Coordination where it matters.** Mixed tools, work across machines and
   destructive git commands, not the same-folder Claude Code case, which ties.
4. **Governance.** Who taught the agents what and when, what changed, and how
   to undo it (provenance, audit and as-of reads, ADRs 0032 and 0034).

The product is three capabilities, each measured separately:

| Capability | Promise | State on 2026-11-19 |
|---|---|---|
| **Capture** | Remember what the user actually said, in their words | The prompt hook saves "never / always / from now on" sentences (ADR 0035), keyword detection only |
| **Delivery** | The right memory reaches the agent at the right moment | Repo rules at every prompt (`GET /rules`), a file's memories on its first edit (ADR 0035 §3); measured by the relay's `b_saw_rule`. Search alone finds a buried plain fact about 39% of the time (58/150) |
| **Enforcement** | The agent doesn't break a rule the team set | Not built as such. The relay's 150/150 is compliance (hidden tests confirm the trap was avoided) in a two-trap, one-model, pre-seeded setting, not proof of enforcement in general |

Delivery isn't compliance; write-ups report them as separate numbers.

Order of work (detail in `docs/roadmap.md`):
- **P0. Historical recall.** Search excludes memories older than 90 days and
  has no default recency weight. Drop the exclusion, add a configurable
  recency weight chosen by benchmark, base both on `document_date` with
  `created_at` as fallback, and measure recall by memory age before and after.
- **P1. Offline replay benchmark** of recorded queries: Recall@5, MRR,
  age-adjusted recall, P95 latency, cost per successful recall. Every later
  retrieval or enforcement change is judged on it.
- **P2. Pre-edit enforcement, warnings first.** Extend ADR 0035's first-edit
  step to every edit and command, matching deterministically on the
  identifiers, paths and commands a rule names, and warn with the rule and
  reason. Deny mode only once false positives are measured; model judgment
  only for rules that can't be matched deterministically. Validate beyond the
  relay: more rules, several repos, Claude Code, Codex and Gemini CLI, two
  models, real edits, with precision, recall, false positives and negatives,
  latency per edit and violations prevented.
- **P3. Retrieval experiments** behind flags: query expansion and a hosted
  cross-encoder reranker. No rewrite in another language and no new model
  server.
- **P4. ADR 0032 decision** with evidence from P1 on supersession (a job
  change, a contradicted preference) and entity resolution (two people with
  similar names).

Alongside: the npm release of memro-cli with the hooks and the claim flow
(ADR 0013 §2), a cross-tool benchmark a notes file can't follow, an outside
rerun of the benchmarks, then team features (board, rule review).

## Alternatives considered
- **Compete on memory benchmarks** (LongMemEval, LoCoMo). Plain RAG ties
  Memro there, and model vendors can outspend us. Kept only as regression
  checks, not as the pitch.
- **A memory plugin for one tool** (Claude Code first). Simplest to build,
  but it competes head-on with that vendor's own memory and gets absorbed.
- **Coordination alone.** In one folder with one tool it ties a shared folder;
  without memory there's little a team keeps coming back for.

## Consequences
- Benchmarks must include the baselines a skeptic would pick: a rules section
  in CLAUDE.md, end-to-end relays where the hook has to catch the rule itself,
  more traps with keywords the agent wouldn't guess, and a second model.
- Public claims are about delivering rules across tools and people, and about
  beating built-in auto-memory, not about recall quality.
- Work on Memro's search is judged on the P1 replay and the relay's
  plain-fact arm, not by leaderboard scores.
- The 100% delivery and 150/150 compliance figures are not a claim of 100%
  enforcement; that needs its own violations-prevented rate (P2).
- The hooks become the core of the product, so their failure modes (keyword
  rule detection, the 200-rule listing, failing open) need tests and limits
  stated up front.
- Open: what the rulebook does past 200 rules per repo, and how users review
  rules the hook saved wrongly.
