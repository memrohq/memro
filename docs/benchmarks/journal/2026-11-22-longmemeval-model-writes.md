# 2026-11-22: LongMemEval-100, memories written by a model

Pass bar written after the raw control was graded and before any facts-arm
answer existed; results are added below it without changing it.

- **Harness**: `bench/longmemeval/run.py` with `WRITE_MODE=raw|facts|facts+raw`, branch `bench/model-writes`
- **Server**: `main` at `07e60ba`, local, fresh Postgres 17 + pgvector 0.8.7 (Homebrew; Docker's storage was corrupted by a full disk)
- **Extraction**: Claude Haiku through `claude -p` (no tools, hooks or MCP), one call per session, cached in `runs/_facts/claude-haiku`
- **Answers and grading**: `gpt-4o-mini`, top-15 hybrid, session expansion 3, as in every earlier run

## Question

Every earlier run stored each exchange word for word. Does storing what a
model extracts from each session (one fact per memory, relative dates turned
into dates) answer more questions correctly?

## Pass bar

Compared question by question with today's raw control (84/100):

- **Facts better**: McNemar p < 0.05 against raw. At 84% that needs roughly
  8 questions fixed for at most 1 broken, so about 91% or more.
- **Tie**: no significant difference. Reported as a tie, with the change in
  prompt tokens and per-type counts.
- **Facts worse**: significant the other way.
- Per-type differences (knowledge-update 16, temporal 26, multi-session 27)
  are reported but not claimed: each type alone is too small.

## Deviations before the run

- The first extraction prompt put the whole session in Claude Code's system
  prompt; Haiku answered "NONE" for 51% of sessions, including 2 of 3 tries on
  a session full of facts. The run was stopped, the cache deleted, and the
  session moved into the message (empty rate 20% after the fix). No answers
  from the first attempt were kept.

## Grader check (before any facts-arm answer)

`judge.py --model claude-sonnet` grades on the Claude plan through `claude -p`
with the same official prompts. On the raw control it agrees with
`gpt-4o-mini` on 97 of 100 answers (84 vs 83 correct). The pass bar above
stays on `gpt-4o-mini`, as written; both arms are also graded with
`claude-sonnet` and reported alongside.

## Results

**Tie.** Facts are not significantly better or worse than raw, but use about
a third of the prompt tokens.

| | Raw | Facts (Haiku) | Fixed | Broken | Exact McNemar p |
|---|---|---|---|---|---|
| `gpt-4o-mini` (pass bar) | 84/100 | 80/100 | 8 | 12 | 0.50 |
| `claude-sonnet` | 83/100 | 79/100 | 9 | 13 | 0.52 |
| Answer prompt tokens, mean | 10,763 | 3,135 | | | |

Per type (`gpt-4o-mini`, not claimed):

| Type | Raw | Facts |
|---|---|---|
| knowledge-update | 14/16 | 13/16 |
| multi-session | 20/27 | 21/27 |
| single-session-assistant | 11/11 | 6/11 |
| single-session-preference | 5/6 | 3/6 |
| single-session-user | 14/14 | 14/14 |
| temporal-reasoning | 20/26 | 23/26 |

The facts arm loses most on single-session-assistant (5 of 11). Those
questions ask what the assistant said; whether extraction drops assistant
content has not been checked yet.

The run paused at 56/100 when the Claude plan's daily limit was hit and was
resumed with the same command. Answered questions and cached extractions were
kept, and nothing was rerun.

Raw rows for both arms are in
[`results/longmemeval/write-raw-100`](../results/longmemeval/write-raw-100/)
and [`write-facts-haiku-100`](../results/longmemeval/write-facts-haiku-100/).
The 19 MB extraction cache is not copied.

## What it means

- Model-written facts don't make Memro answer more questions correctly. That
  adds to earlier ties: Memro's search against plain RAG (2026-11-15) and
  rules against a notes file (2026-11-19).
- About the same accuracy for 29% of the prompt tokens is a real gain. It
  matters most for the hooks, which add context to every prompt.
- None of this counts against the rules direction (ADR 0036). Better
  retrieval was never the bet. Rules pushed by the hooks are the one result
  where Memro clearly beat something (Claude Code's auto-memory).

## Next

1. One `WRITE_MODE=facts+raw` run on the same 100. It could keep the
   temporal gain and win back the assistant questions. If it also ties, stop
   and treat facts as a way to cut tokens, not to raise accuracy.
2. Check whether extraction drops what the assistant said in the 5 lost
   single-session-assistant questions.
3. Put the contradiction-strategy and retrieval-tuning steps behind the rules
   work.
4. The cross-tool relay (agent A in Claude Code, B in Codex) is the test
   that decides the rules direction: a single repo file can't carry a rule
   between the two tools. A tie with the native arm (CLAUDE.md / AGENTS.md)
   would mean rethinking the rules pitch.
