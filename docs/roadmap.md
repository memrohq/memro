# Roadmap

What's next for Memro's agent coordination and memory, in order. Results and
pass bars: [benchmarks/coordination-launch-status.md](benchmarks/coordination-launch-status.md).
Updated 2026-11-18.

## Next: the 3,000-fact relay (to run 2026-11-19)

**Why.** With 300 earlier facts, Memro with its hooks kept a fresh agent from
a known mistake 10/10 and a notes file 9/10 (journal 2026-11-17, §8): a tie.
At 300 lines the agent reads the notes file whole. The `huge` setting asks
what happens when it can't: 3,000 earlier facts, 100 of them earlier rules for
the same repo, more than Claude Code's Read shows at once (2,000 lines). A
clear gap here is the Hacker News result; a tie is published as a tie.

**Sample size.** 10 trials per arm can't separate close results: 9/10 vs
10/10 has Fisher p = 1.0, and the old bar (notes ≤ 6/10 vs hooks ≥ 9/10) is
only p = 0.30. So run R1 and R2 15 times each (30 per arm), and the bar is
**notes ≤ 18/30 and Memro + hooks ≥ 28/30** (p ≈ 0.005). The write-up gives
the Wilson 95% interval for every arm and the Fisher p for notes vs hooks.

**Known gaps, said in the write-up.** (Raised by a LinkedIn reader on the
300-fact post.) R2's task says "coupon" and so does the fact, so a keyword
search can find it without real recall; R1's task doesn't repeat Intl/ICU.
In the notes arm, A's line lands at the end of NOTES.md, past the 2,000 lines
Read shows, so a notes loss there is truncation, not lost-in-the-middle.
Follow-up: a trap with no shared keywords, and the fact placed at the start,
middle and end of the file.

**Why locally.** Production caps an agent nobody has claimed at 1,000
memories, and the claim flow of ADR 0013 §2 isn't built (see below), so the
benchmark agent can't hold 3,000. A local server from `main` has the same
1,000 cap by default; start it with `MEMORY_SOFT_CAP=100000`. The write-up
says the numbers come from a local build of `main`.

**Steps.**

1. Postgres with pgvector (only the client tools are installed now):

       brew install postgresql@17 pgvector
       brew services start postgresql@17
       createdb memro_bench

   Homebrew's `postgresql@17` is keg-only and looks for its files under
   `/opt/homebrew/share/postgresql@17` and `/opt/homebrew/lib/postgresql@17`,
   which hold only pgvector's; symlink the keg's `share/postgresql/*` and
   `lib/postgresql/*` into them before `initdb`.

   Or, with Docker Desktop running: `docker run -d --name memro-bench-pg -e POSTGRES_PASSWORD=bench -p 5432:5432 pgvector/pgvector:pg17`.

2. The server from `main`, in `memro/server` (migrations run on startup and
   create the `vector` extension). Use the embedding provider production uses
   (check `EMBEDDING_PROVIDER` / `EMBEDDING_MODEL` in memro-cloud's deploy;
   the default is OpenAI `text-embedding-3-small`):

       DATABASE_URL="postgres://localhost/memro_bench?sslmode=disable" \
       OPENAI_API_KEY="$(grep ^OPENAI_API_KEY= ../../memro-benchmark/.env | cut -d= -f2-)" \
       PORT=8081 RATE_LIMIT_MAX=50000 MEMORY_SOFT_CAP=100000 go run ./cmd/memro-api

   Check: `curl localhost:8081/health`.

3. Smoke test, then the run (from the repo root, with the MCP server's
   Python, `mcp/.venv`):

       export MEMRO_API_URL=http://localhost:8081 MEMRO_CLI=$PWD/cli
       mcp/.venv/bin/python bench/relay/run.py --setting huge --traps R1 --arms notes,memro-hooks --runs 1 --out bench/runs/relay/smoke-huge
       mcp/.venv/bin/python bench/relay/run.py --setting huge --traps R1,R2,R4 --arms notes,memro,memro-hooks --runs 15 --out bench/runs/relay/huge-20261119

   About three hours and around $3 of Claude; preloading 3,000 memories costs a few
   cents of embeddings.

4. Write it up as §9 of `benchmarks/journal/2026-11-17-relay-benchmark.md`,
   copy the results to `benchmarks/results/relay/huge-20261119/` (local paths
   replaced) with a row in its index, and update the launch status page.

5. Stop Postgres afterwards: `brew services stop postgresql@17`.

## Then: the rule-depth grid (same setup, run 2026-11-19)

Plan, predictions and pass bar: `benchmarks/journal/2026-11-19-relay-rule-depth.md`.
Only B runs; R1's and R2's rules sit at 25%, 50% or 75% of the 3,000 facts in
every arm's store (NOTES.md, Claude Code's auto-memory, Memro). 650 B
sessions, about 6 hours and $8:

    caffeinate -dimsu bench/relay/depth-grid.sh

It resumes where it stopped (a usage limit stops a run without recording
the relay) and waits 30 minutes between attempts.

**Left over.** An unclaimed bench agent created on production while trying
the claim route is saved at `~/.memro/bench-agent.json` (mode 600) and holds
no memories; delete the file, or the agent with `DELETE /identity/{id}`.

## Next engineering cycle: capture, delivery, enforcement (ADR 0036)

Memro's product is three capabilities, each measured on its own:

| Capability | Promise | Where it stands | Measured by |
|---|---|---|---|
| **Capture** | Remember what the user actually said, in their words | The prompt hook saves "never / always / from now on" sentences (ADR 0035); keyword detection only | Rules caught from real prompts; wrong saves |
| **Delivery** | The right memory reaches the agent at the right moment | Repo rules at every prompt via `GET /rules` (up to 200); a file's memories on its first edit (ADR 0035 §3). Search alone finds a buried plain fact about 45% of the time | `b_saw_rule` in the relay |
| **Enforcement** | The agent doesn't break a rule the team set | Not built as such. The relay's 43/43 "trap avoided" at 25% depth is compliance (hidden tests), in a two-trap, one-model, pre-seeded setting | Violations prevented, false positives and negatives |

Delivery isn't compliance: 43/43 is the compliance number, and `b_saw_rule`
is the delivery number. Keep them apart in every write-up.

**P0. Historical recall.** Search only looks at memories from the last 90
days (`searchableFilter`, `engine.go`; carried over in the Go rewrite
`87817ca` with no recorded reason), and ordinary search has no recency
weight (`TemporalWeight` 0; only `/search/temporal` defaults to 0.3). Ship
together:
- drop the hard 90-day exclusion;
- a configurable default recency weight, chosen by benchmarking several
  values, with relevance still the main signal;
- eligibility and recency use `document_date`, falling back to `created_at`,
  so an imported two-year-old Slack message isn't treated as new.
  (`created_at`: when Memro stored it; `document_date`: when the source was
  written; `event_date`: when the described event happened.)
- a memory-age benchmark before and after: the same queries against memories
  0–30, 31–90, 91–180, 181–365 and 365+ days old (backdated in the test
  database, since the API stamps `created_at` itself).

**P1. Offline replay benchmark.** The base for every retrieval and
enforcement change: replays recorded queries (the relay's recall calls,
LongMemEval) against a server, in seconds and for cents. Cases: hybrid search
as today, old memories, knowledge updates, contradictions, ambiguous
entities, exact rule retrieval. Reports Recall@5, MRR, age-adjusted recall,
P95 latency and cost per successful recall.

**P2. Pre-edit enforcement, warnings first.** Extends the ADR 0035 hook, which
already shows a file's memories on its first edit, rather than starting over:
- run on every edit and command, not only a file's first edit;
- match deterministically on what a rule names: identifiers (`Intl`,
  `toLocaleString`), paths (`internal/client.go`) and commands;
- warn with the rule and the reason, and leave the agent free to go on; a
  deny mode comes only after the false-positive rate is known;
- a model judges only rules that can't be matched deterministically
  ("don't change public APIs"), and only later.

Validation goes beyond the relay's two traps: more rules, several repos,
Claude Code, Codex and Gemini CLI, two models, real edits. Report precision
and recall of rule matching, false positives and negatives, added latency per
edit, and violations prevented.

**P3. Retrieval experiments, cheap and flagged.** Query expansion (guess the
APIs and terms a task involves) and a hosted cross-encoder reranker, each
behind a flag, judged on the P1 replay and the relay's plain-fact arm. No new
infrastructure: no Rust or C++ rewrite, and no model server yet
(`memro-model-server` is a coding-chat model for Acode, not an embedding or
reranking service).

**Storage stays Postgres + pgvector.** The misses come from queries that
don't mean what the rule means, and any vector database returns the same
neighbours for the same embedding. Memro already moved off Qdrant in the Go
rewrite (`87817ca`), and Postgres carries tenancy, audit, as-of reads, the
`/rules` metadata listing and transactions in one place (ADR 0002). Cheaper
levers, tried on the P1 replay before any stack change:
- a larger embedding model (`text-embedding-3-large`, Voyage, Cohere; ADR
  0024's evaluation), a config change plus a re-embed;
- true BM25 inside Postgres (`pg_search`/ParadeDB) if the keyword side turns
  out to matter; `ts_rank_cd` only approximates it;
- HNSW and per-tenant filter tuning once an agent holds hundreds of thousands
  of memories.

Revisit the stack only at tens of millions of vectors per tenant, query rates
replicas can't serve, or deep graph traversal ADR 0032's tables don't cover.

**P4. Memory evolution: decide on ADR 0032 with evidence.** Use P1 to test
three cases: a job change (the old fact becomes history), a contradicted
preference (the newer one wins) and two people with similar names (kept
apart). Today supersession is rule-based and slightly behind plain RAG on
LongMemEval knowledge updates (12/16 vs 14/16), and entity resolution doesn't
exist. Build ADR 0032 incrementally only where those cases fail.

## After that

1. **Claim flow (ADR 0013 §2).** Accepted but not built:
   `POST /identity/{id}/claims` returns 404. Without it no agent but the one
   `memro connect` creates can hold more than 1,000 memories on production,
   which will hit real users' agents too, not only benchmarks.
2. **Plan claims** (ADR 0019 §8, pass bar 2): claim the files a task will touch
   at the prompt, so overlapping work is split or ordered up front. Memro in
   one folder is 2.0× the shared folder's time today; the bar is 1.3×.
3. **Landing under load** (pass bar 1): 10 agents, more overlap, a stronger
   model. With 5 Haiku agents plain git already lands 100/100, so the test
   can't tell `memro land` (38% cheaper) from git on tasks landed.
4. **Benchmark v3** (pass bar 4): tasks from a real repo at three overlap
   levels, Codex and Gemini CLI agents, a real `herdr` arm, two models.
5. **npm release** of memro-cli with the coordination and memory hooks: after
   the remaining items of the 2026-11-16 release list (plan claims; the
   per-window MCP holder, which needs memro-cloud's core pin bumped).
6. **Team board** in the dashboard (ADR 0019): read-only, cross-tool, after
   the launch benchmark; ask in discussion #93 first whether people want it.
7. **Launch**: Hacker News only after a pass bar is clearly met, with the
   numbers and rerun command on memro.co (see the launch status page).
