# 2026-11-18: Landing worktrees one at a time, and shorter holds

- **Date**: 2026-11-18
- **Harness**: `bench/coordination/run.py --agents launch` at `bcb6fec` (PR #92: `memro land` and the two land arms), hooks with `MEMRO_HOOK_HOLD_SECONDS=10 MEMRO_HOOK_WAIT_SECONDS=80`
- **Run directories** (local, not committed): `bench/runs/coordination/hold10-wait80-20261118`, `bench/runs/coordination/land-20261118`
- **Agents**: 5 × Claude Code 2.1.311 (`--model haiku`), two tasks each, all at once
- **Plan and pass bars**: `docs/benchmarks/coordination-v3-plan.md`, written before these runs
- **Cost**: $0.99 (holds) + $2.29 (landing) of Claude

## 1. Goal

Two of the plan's bars, on the launch task set:
1. **Landing**: worktrees + Memro land ≥ 90/100, and more than agents landing with plain git.
2. **Speed**: Memro in one folder within 1.3× the shared folder's median time, with no task lost to an agent giving up.

## 2. Shorter holds and longer waits (one folder)

Each edit now holds its file for 10 s instead of 45 s, and a refused edit waits up to 80 s instead of 60 s. 10 runs per arm, interleaved.

| Arm | Tasks landed | Runs with lost work | Median wall time | Hook events |
|---|---|---|---|---|
| shared | 99/100 | 1/10 | 49 s | – |
| memro | **100/100** | 0/10 | 97 s | 70 waits, 2 git refusals, 0 refusals |

- **No agent gave up.** With the longer wait every file freed up in time, so there were no refusals and no lost tasks (the v2 rerun lost 2 to agents asking the user).
- **Still 2.0× slower** (97 s vs 49 s; 120 s with 45 s holds). Bar 2 is **not met**: agents queue on `src/cart.js`, and shorter holds only shorten each wait.

## 3. Landing worktree branches (four worktree arms)

| Arm | Tasks landed | Merge conflicts left | Median wall time | Cost (10 runs) |
|---|---|---|---|---|
| worktrees (merged afterwards) | 20/100 | 40 | 37 s | $0.35 |
| memro-worktrees (warnings) | 20/100 | 40 | 46 s | $0.40 |
| worktrees-land (agents land with git) | **100/100** | 0 | 111 s | $0.95 |
| memro-worktrees-land (`memro land`) | **100/100** | 0 | 88 s | $0.59 |

- **Agents landing their own work is what fixes worktrees**, with or without Memro: 20/100 → 100/100. An agent that rebases knows what its change was for and resolves the conflict; a merge afterwards doesn't.
- **Memro doesn't land more; it lands cheaper and somewhat faster.** Every `memro land` run cost less than every git run ($0.055–0.065 vs $0.084–0.103 per run, −38%). Wall time was lower in most runs (median 88 s vs 111 s, −21%; Mann-Whitney U = 76/100, borderline), with one slow Memro run at 177 s. Why it's cheaper isn't measured yet; likely fewer rebase-and-retry rounds when main moves under an agent, since agents land one at a time.
- Bar 1 is **met on ≥ 90/100 and not met on "more than plain git"**: both land everything.

## 4. What this means for the claim

- Worktree tools that merge branches afterwards lose most of the work when tasks overlap; tools whose agents rebase and land their own work don't. The fair comparison for Memro is the second, and against it Memro saves cost and some time, not tasks.
- In one folder, Memro's edge is safety (no `git stash` losses here or in the launch run's memro arm) at 2× the time.
- Neither result is the headline the plan asks for yet.

## 5. Next

- **Speed in one folder**: agents queue on one file. Claim the files a task will touch at the prompt (plan claims, ADR 0019 §8) so overlapping tasks are ordered or split before the first edit, instead of waiting edit by edit.
- **Explain the landing cost gap** from the transcripts (rebase rounds, conflicts per landing) before quoting it.
- **Harder tests for landing**: more agents, bigger overlap, a stronger model; with 5 Haiku agents plain git already lands everything.

## 6. Caveats

- One model (Haiku), one small task set, 10 runs per arm.
- The two land arms' instructions ask for the same steps but aren't word for word the same.
- Hold and wait times differ from the launch run, so its memro numbers aren't directly comparable.
