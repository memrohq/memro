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
clear gap here (notes ≤ 6/10, Memro + hooks ≥ 9/10) is the Hacker News
result; a tie is published as a tie.

**Why locally.** Production caps an agent nobody has claimed at 1,000
memories, and the claim flow of ADR 0013 §2 isn't built (see below), so the
benchmark agent can't hold 3,000. A local server from `main` has no cap. The
write-up says the numbers come from a local build of `main`.

**Steps.**

1. Postgres with pgvector (only the client tools are installed now):

       brew install postgresql@17 pgvector
       brew services start postgresql@17
       createdb memro_bench

   Or, with Docker Desktop running: `docker run -d --name memro-bench-pg -e POSTGRES_PASSWORD=bench -p 5432:5432 pgvector/pgvector:pg17`.

2. The server from `main`, in `memro/server` (migrations run on startup and
   create the `vector` extension). Use the embedding provider production uses
   (check `EMBEDDING_PROVIDER` / `EMBEDDING_MODEL` in memro-cloud's deploy;
   the default is OpenAI `text-embedding-3-small`):

       DATABASE_URL="postgres://localhost/memro_bench?sslmode=disable" \
       OPENAI_API_KEY="$(grep ^OPENAI_API_KEY= ../../memro-benchmark/.env | cut -d= -f2-)" \
       PORT=8081 go run ./cmd/memro-api

   Check: `curl localhost:8081/health`.

3. Smoke test, then the run (from the repo root, with the MCP server's
   Python, `mcp/.venv`):

       export MEMRO_API_URL=http://localhost:8081 MEMRO_CLI=$PWD/cli
       mcp/.venv/bin/python bench/relay/run.py --setting huge --traps R1 --arms notes,memro-hooks --runs 1 --out bench/runs/relay/smoke-huge
       mcp/.venv/bin/python bench/relay/run.py --setting huge --traps R1,R2,R4 --arms notes,memro,memro-hooks --runs 5 --out bench/runs/relay/huge-20261119

   About an hour and under $1 of Claude; preloading 3,000 memories costs a few
   cents of embeddings.

4. Write it up as §9 of `benchmarks/journal/2026-11-17-relay-benchmark.md`,
   copy the results to `benchmarks/results/relay/huge-20261119/` (local paths
   replaced) with a row in its index, and update the launch status page.

5. Stop Postgres afterwards: `brew services stop postgresql@17`.

**Left over.** An unclaimed bench agent created on production while trying
the claim route is saved at `~/.memro/bench-agent.json` (mode 600) and holds
no memories; delete the file, or the agent with `DELETE /identity/{id}`.

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
