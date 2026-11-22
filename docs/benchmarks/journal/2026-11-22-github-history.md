# 2026-11-22: GitHub history — does the connector save an agent tokens?

Plan, pass bar and predictions written before the full run; results are added
below them without changing them.

- **Harness**: `bench/github_history/` (`fetch.py`, `run.py`, `questions.json`), branch `bench/github-history`
- **Corpus**: every issue and pull request in `memrohq/memro` with its comments (109 items, 7 comments), fetched as the GitHub connector fetches them and turned into memories by the connector's own code (`server/internal/connectors/github_dump_test.go`): 170 memories
- **Server**: local, built from `main` at `07e60ba` (search code unchanged since), Postgres 17 + pgvector
- **Agent**: Claude Code, `--model haiku`, headless, a fresh session per question and arm, a short system prompt replacing Claude Code's own; no CLAUDE.md, settings or other MCP servers
- **Grading**: Claude Sonnet, against a reference answer, yes or no

## Question

When a coding agent needs something from a repo's past ("why does X work
this way?", "what's the limit on Y?"), does Memro's GitHub connector let it
answer with fewer tokens than looking it up with `gh`, without answering
worse?

## Arms

| Arm | The agent can use |
|---|---|
| none | nothing (the floor: what Haiku guesses) |
| gh | `gh pr`, `gh issue`, `gh search` on the repo, nothing else |
| memro-raw | Memro's MCP `recall`, over the 170 memories the connector stores |
| memro-facts | the same, over facts Claude Haiku extracted from each item (one memory per fact, the connector's header in front) |

Arms are interleaved per question in a fixed random order.

## Questions

30 questions in `questions.json`, written from 28 pull requests and 2 open
issues, each with a reference answer and the item it comes from. They ask
what a developer would ask, in other words than the PR where possible; some
have to name a command or flag.

## Measures

- **Correct**: Sonnet's yes/no against the reference.
- **Input tokens**: all input tokens the agent's turns read (input + cache
  writes + cache reads), from Claude Code's result. Also turns, seconds and
  cost.

## Pass bar

- **Memro saves tokens**: memro-raw reads fewer input tokens than gh on
  significantly more questions (two-sided sign test p < 0.05, so at least
  21 of 30), with the median per-question ratio reported.
- **At no accuracy cost**: memro-raw is not significantly less accurate than
  gh (exact McNemar p < 0.05 the other way would mean it is).
- Both must hold to claim that the connector saves tokens. Anything else is
  reported as it falls, with the numbers.
- memro-facts against memro-raw and gh: reported, not claimed (one extraction
  run, no repeat).
- If **none** gets more than 5 right, the questions leak or Haiku knows the
  repo, and the accuracy comparison is void.

## Predictions

- gh needs a listing and one or more `gh pr view` calls per question, so it
  reads 2–3× the tokens of memro-raw, which needs one or two recalls.
- Accuracy is close between gh and memro-raw: 109 items is few enough that
  `gh pr list` shows them all.
- memro-facts reads fewer tokens than memro-raw and loses some detail.
- none gets 0–3 right.

## Known biases

- One person wrote the questions from PR bodies, so their wording may suit
  search more than a real developer's would.
- A small repo favours gh; a repo with thousands of PRs would favour search.
- One model, one run per question and arm, 30 questions.
- Designed and run by Memro, on a local build.

## Results

**The pass bar was not met. memro-raw reads about the same tokens as gh,
not fewer, and is not significantly more or less accurate.** The prediction
of 2–3× fewer tokens was wrong.

| Arm | Correct | Median input tokens | Median turns | Median seconds | Median cost |
|---|---|---|---|---|---|
| none | 0/30 | 921 | 1 | 3.5 | – |
| gh | **29/30** | 16,558 | 3 | 15.7 | $0.0017 |
| memro-raw | 26/30 | 16,222 | 2 | **6.2** | $0.0013 |
| memro-facts | 23/30 | **14,318** | 3 | 6.1 | **$0.0008** |

| Against gh | Fewer tokens | Sign test p | Median token ratio | Right where gh wasn't / the reverse | McNemar p |
|---|---|---|---|---|---|
| memro-raw | 15/30 | 1.0 | 0.99 | 0 / 3 | 0.25 |
| memro-facts | 19/30 | 0.20 | 0.91 | 0 / 6 | **0.03** |

- **Tokens: a tie.** memro-raw read fewer on exactly half the questions.
- **Accuracy: gh 29, memro-raw 26**, not significant. memro-facts is
  significantly worse than gh (23 vs 29, p = 0.03): six answers needed a
  detail the extracted facts had dropped (the default proof-of-work
  difficulty, what #97 left out, why the uploads were manual).
- none got 0/30, so the questions don't leak and Haiku doesn't know the repo.
- q07 (how many rules the hook shows) was missed by every arm. Its reference
  answer comes from #100, and later PRs changed rule storage, so agents
  rightly hedged. The question is flawed, but it counts against every arm
  alike.

**Deviations found after the run.**
- `gh search` also allows `gh search code`, so in 4 questions (q04, q08,
  q11, q27) the gh arm searched the repo's source too, which the Memro arms
  can't. Both arms got all four right, so it changed no score, though it
  may have changed gh's tokens.
- In q19 a gh agent ran `grep` on the file where Claude Code had saved its own
  long `gh` output. Claude Code allows read-only commands without asking. The
  agent read nothing but its own result.
- The Memro arms carried no comparable advantage.

## Why (exploratory, not part of the bar)

- **The Memro MCP tool list is most of Memro's tokens.** A session that only
  says "hi" reads 828 tokens without Memro's MCP server and 6,018 with it:
  about 5,200 for the 16 tool definitions, read again on every model call.
  memro-raw's cheapest question used 14,754 tokens, so in a 2-turn session
  about 10,000 of the ~16,000 are the tool list, and what recall returned is
  roughly 4,000–6,000. gh carries almost no fixed cost but reads whole PR
  bodies. gh's spread (7k–44k) is wide, while memro-raw's is narrow (15k–25k).
- **Memro answers 2.5× faster** (6 s vs 16 s median) in fewer turns. It is
  also about a quarter cheaper per question, because the repeated tool list
  is billed as cache reads. Neither was a pre-set measure.
- **memro-raw's 4 misses (gh got 3 of them right) were ranking misses.**
  Each agent called recall once or twice with `limit` 5, and the right PR
  wasn't in the five. For q06 it found #98 (which floated the opt-out) and
  not #100 (which shipped it). For q09 it found #91's handoff notes, not
  #98's first-edit memories. gh searched, then opened the PR and read it.

## What it means

- On a 109-item repo, the GitHub connector doesn't save an agent tokens
  over `gh`. It answers in less than half the time, at a similar cost and a
  similar (not significantly lower) accuracy.
- The cheapest win is the MCP tool list: ~5,200 tokens a call for 16 tools,
  most of which a recall-only session never uses. Cutting it to the tools a
  session needs would put memro-raw at roughly a third of gh's tokens on
  these numbers. That is an estimate to test, not a result.
- Extracted facts are the wrong trade here. As on LongMemEval (2026-11-22),
  they cut tokens a little and lose detail, and here the lost detail was the
  answer six times.

## Next

1. Slim or split the MCP tool list (recall/remember only by default, or
   shorter descriptions) and rerun the same 30 questions. Pass bar as above.
2. A larger repo, where `gh pr list` can't show everything.
3. Recall's default `limit` of 5 for agents: try 10 and report misses.

Raw rows: [`results/github-history/run-20261122`](../results/github-history/run-20261122/).

## Follow-up: a slim tool list (plan written before the run)

`memro-mcp` now takes `MEMRO_MCP_TOOLS`, a comma-separated list of the tools
to list (unset: all 26, as before). With `recall,remember,update_memory`, a
session that only says "hi" reads 2,515 tokens instead of 6,018, so the
per-call overhead drops from ~5,200 to ~1,700.

**Run.** The same 30 questions, a new run (`run-slim-20261122`) with three
arms interleaved: gh, memro-raw (all tools, a replication of the first run)
and **memro-slim** (memro-raw's memories, the slim list). Every arm is still
allowed to call only what it was allowed before.

**Pass bar.** The same as above, for memro-slim against gh in this run:
fewer input tokens on at least 21 of 30 (sign test p < 0.05), and not
significantly less accurate (McNemar).

**Prediction.** memro-slim's median falls to about 9,000 (two turns × 3,500
fewer), around 0.55 of gh, and its accuracy matches memro-raw's, since the
retrieval is the same.

### Follow-up results

**The pass bar was met. With the slim tool list, Memro read fewer input
tokens than gh on 26 of 30 questions (sign test p = 0.00006), a median of
0.64 of gh's, at the same accuracy (28/30 each).**

| Arm | Correct | Median input tokens | Range | Median seconds | Median cost |
|---|---|---|---|---|---|
| gh | 28/30 | 15,208 | 6,490–36,884 | 13.9 | $0.0015 |
| memro-raw (26 tools) | 28/30 | 16,871 | 14,979–24,834 | 6.2 | $0.0015 |
| **memro-slim (3 tools)** | **28/30** | **10,320** | 8,062–11,907 | **6.0** | $0.0016 |

| Against gh | Fewer tokens | Sign test p | Median ratio | Right where gh wasn't / the reverse | McNemar p |
|---|---|---|---|---|---|
| memro-raw | 10/30 | 0.10 | 1.20 | 1 / 1 | 1.0 |
| memro-slim | **26/30** | **0.00006** | **0.64** | 2 / 2 | 1.0 |

- **The replication holds.** memro-raw again ties gh on tokens (slightly
  more this time). Its accuracy rose from 26 to 28 with the same memories,
  which shows how much one run per question moves.
- **The prediction was close.** The median was 10,320, against the predicted
  9,000; slim sessions took a median of 3 turns, not 2.
- **Tokens fell, but dollars didn't.** The repeated tool list was billed as
  cache reads, which cost a tenth as much, so memro-slim's median cost
  ($0.0016) is about the same as gh's ($0.0015). The saving is in context
  room and in how predictable a session is: memro-slim never went past
  11,907 tokens, while gh went up to 36,884.
- **Misses** were again ranking misses: q09 (both Memro arms found #91's
  handoff notes, not #98) and q18. gh missed q06 and q07 (the flawed
  question).

### What it means now

- On this repo, Memro's GitHub connector, with the MCP server listing only
  the tools a session needs, answers history questions with about a third
  fewer tokens than `gh`, 2.3× faster, and with the same accuracy. It is not
  cheaper in dollars on Claude's cache pricing.
- Without the slim list it saves nothing: all 26 tool definitions cost
  ~5,200 tokens on every call. memro-cli should set `MEMRO_MCP_TOOLS` when it
  connects a tool, or the server should list fewer tools by default.
- Still open: a larger repo, more than one run per question, and recall's
  ranking misses (`limit` 5).

Raw rows: [`results/github-history/run-slim-20261122`](../results/github-history/run-slim-20261122/).
