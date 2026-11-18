# 0036. Direction: the shared rulebook and coordination layer across agent tools
Date: 2026-11-19 · Status: Proposed

## Context
Four benchmarks so far show where Memro adds value and where it doesn't:

| Benchmark | Result | Source |
|---|---|---|
| LongMemEval-100 | Memro 81%, plain semantic RAG 81% (McNemar p = 1.0) | journal 2026-11-15 |
| Coordination, one folder | Memro ties an uncoordinated shared folder, 98/100 tasks landed each: Claude Code's Edit refuses stale writes | journal 2026-11-17 |
| Relay, 300 facts | Memro + hooks 10/10, NOTES.md 9/10, Memro tools alone 8/10 | journal 2026-11-17 §8, ADR 0035 |
| Relay, rule at 25% of 3,000 facts (interim) | Memro + hooks 43/43, NOTES.md 41/43, Memro search alone 19/43, Claude Code auto-memory 0/43 | journal 2026-11-19 |

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

Order of work:
1. Ship what works: the npm release of memro-cli with the hooks, and the
   claim flow (ADR 0013 §2), so real agents aren't stuck at 1,000 memories.
2. Benchmark the case a notes file can't follow: a rule saved in Claude Code
   and obeyed by Codex in another clone. That is the Hacker News result.
3. Retrieval: improve it or stop leaning on it in messaging; 45% for a plain
   fact among 3,000 is weak.
4. Have someone outside the team rerun the benchmarks from the published command.
5. Team features: the shared board (ADR 0019), reviewing and editing the rules
   the hook captured, organisation controls.

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
- Work on Memro's search is judged by whether it moves the plain-fact relay
  arm, not by leaderboard scores.
- The hooks become the core of the product, so their failure modes (keyword
  rule detection, the 200-rule listing, failing open) need tests and limits
  stated up front.
- Open: what the rulebook does past 200 rules per repo, and how users review
  rules the hook saved wrongly.
