# 2026-11-17: Multi-agent collision benchmark, launch run

- **Date**: 2026-11-17
- **Harness**: `bench/coordination/run.py --agents launch --runs 10` (PR #91 branch, hooks with shell/git guard, briefings and handoff notes)
- **Run directory**: `bench/runs/coordination/launch-20261117/results.jsonl` (local, not committed)
- **Agents**: 5 × Claude Code 2.1.311 (`--model haiku`), two tasks each, all at once. No Codex: its hooks run only after the user trusts the changed entries again.
- **Arms**: 4, interleaved run by run, 10 runs each (40 runs, 400 agent turns)
- **Cost**: $1.76 of Claude in total

## 1. Goal & hypothesis

The pilot (2026-11-16) showed silent loss happens but was too small to say how
often. Hypotheses: (1) in a shared folder without coordination, agents lose
work, including to one agent's `git stash`; (2) Memro's hooks prevent that at a
cost in time; (3) Memro's merge-conflict warnings help agents in worktrees.

## 2. Setup

Ten tasks on the shop fixture, most touching `src/cart.js` and `src/index.js`.
T10 adds a function and then checks the base tests on the last commit with
`git stash`, `node --test`, `git stash pop` while the other agents are mid-edit.

| Arm | Setup |
|---|---|
| shared | one folder, no coordination (agents side by side, as in herdr) |
| worktrees | one worktree per agent, branches merged into main at the end; a conflicting branch is not merged |
| memro | one folder with Memro hooks: claims, 60 s wait for a busy file, git guard, briefings, handoff notes |
| memro-worktrees | one worktree per agent with Memro hooks (merge-conflict warnings) |

## 3. Results

| Arm | Tasks landed | Runs with lost work | Work stranded in `git stash` | Merge conflicts | Median wall time | Hook events |
|---|---|---|---|---|---|---|
| shared | 91/100 | 4/10 | 3 runs | – | 53 s | – |
| worktrees | 20/100 | 10/10 | 1 run | 40 (4 of 5 branches, every run) | 37 s | – |
| **memro** | **96/100** | 3/10 | **0** | – | 119 s | 71 waits, 9 refusals, 5 `git stash` refused |
| memro-worktrees | 20/100 | 10/10 | 0 | 40 | 49 s | 123 merge-conflict warnings |

**`git stash` is the big hazard without coordination.** In 3 of 10 shared runs,
T10's stash took the other agents' uncommitted edits with it, `git stash pop`
then conflicted with edits made in between, and the work stayed in the stash.
Those runs lost 1 to 4 tasks each. With Memro the stash was refused whenever
another agent was mid-turn (5 times), and no run stranded work.

**Memro's own losses come from giving up, not from collisions.** All four lost
tasks were by agents told a file was busy (after the 60 s wait, or by the
briefing) that stopped to ask the user instead of retrying later. That is the
next fix, not a coordination failure: nothing was overwritten.

**Memro costs time.** Agents wait in line for `cart.js` (about 7 waits per
run), so runs take 2.2× as long as the shared folder.

**Warnings don't save worktrees.** With these tasks every branch edits the same
lines, so 4 of 5 branches conflict whether or not the agents are warned (123
warnings, same 20/100). Worktrees isolate agents but move all the collisions
to merge time.

## 4. Caveats

- Shared-folder losses here were mostly noticed by the agents (they saw files
  change under them and stopped), not silent overwrites; the pilot's silent
  overwrite did not recur in 10 runs.
- The worktree arm merges without anyone resolving conflicts; a person or an
  agent resolving them would land more, at the cost of their time.
- The tasks are small and deliberately overlapping; real work overlaps less,
  so absolute rates are higher than in practice. The comparison between arms
  is the point.
- One model (Haiku) and one task set.

## 5. Next

- **Don't give up**: tell a refused agent to finish its other work and retry,
  and keep the briefing from discouraging work on files nobody holds. Measure
  whether memro reaches 100/100.
- **Cut the waiting**: hand a busy file over as soon as the holder's edit is
  done, not at the end of its turn, or let a waiting agent queue.
- **Coordinate merges, not just warn**: for worktrees, a merge order and
  rebasing later branches on earlier ones.

## 6. Rerun with the retry fix (v2)

- **Run directory**: `bench/runs/coordination/launch-20261117-v2/results.jsonl` (local, not committed)
- **Code**: `fff51a8` (each edit holds its file for 45 s instead of the whole turn; refusals tell the agent to finish other work and retry)
- **Arms**: shared and memro only, 10 runs each

| Arm | Tasks landed | Runs with lost work | Work stranded in `git stash` | Median wall time | Hook events |
|---|---|---|---|---|---|
| shared | 98/100 | 2/10 | 0 runs | 52 s | – |
| memro | 98/100 | 1/10 | 0 runs | 120 s | 80 waits, 13 refusals, 5 git refusals |

**It ties.** The shared folder did better than in the first run (no stash
losses this time), and memro gained 2 tasks. Both memro losses were in one
run, and both agents still stopped and asked the user ("Should I keep waiting
for the hold to clear…?") instead of retrying, so the retry wording isn't
enough on its own.

**Waiting didn't drop.** Holding a file for 45 s instead of a whole turn left
the median at 120 s. Haiku agents edit the same file again within 45 s, so
holds keep renewing.

**Next**: retry inside the hook (keep waiting while the holder is still
working) instead of asking the agent to; release a file when its holder's
edit lands rather than after a fixed hold; and measure where same-tool
stale-write refusals don't save the shared folder: mixed tools (Codex),
longer tasks, other machines.
