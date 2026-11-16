# 2026-11-16: Multi-agent collision benchmark, pilot

- **Date**: 2026-11-16
- **Harness**: `bench/coordination/run.py` (this branch); hooks from memrohq/memro PR #86 (`7fba233`)
- **Run directory**: `bench/runs/coordination/pilot-20261116/results.jsonl` (local, not committed)
- **Agents**: 2 × Claude Code 2.1.311 (`--model haiku`), 1 × Codex CLI 0.167.0-alpha.3.2 (`gpt-6-luna`, reasoning effort low)
- **Arms**: 3 runs each; shared and worktrees ran first, memro straight after (not interleaved; see caveats)
- **Cost**: $0.14 of Claude in total, and 1.55M Codex tokens (mostly cached system prompt)

## 1. Goal & hypothesis

Agents working side by side on one repo, as in herdr, can lose each other's
work. Hypothesis: Memro's hooks (claim each file, wait for a busy one, release
at the end of the turn) keep every task's work, without a merge step, at some
cost in wall time.

## 2. Setup

A fixture shop library (`bench/coordination/fixture`) and six small tasks that
collide on purpose: four touch `src/cart.js`, three `src/index.js`, two
`src/format.js`. Each agent does two tasks in order, all three agents at once.
A task counts only if its hidden acceptance test (`bench/coordination/accept`)
passes in the final repo.

- **shared**: one folder, no coordination.
- **worktrees**: one git worktree and branch per agent; the branches are merged
  into main one by one at the end, and a branch that conflicts is not merged
  (nobody resolves it).
- **memro**: one folder, with Memro's Claude Code and Codex hooks.

## 3. Results

| Arm | Tasks landed | Runs with lost work | Median wall time | Claude cost |
|---|---|---|---|---|
| shared | 17/18 | 1/3 | 47 s | $0.053 |
| worktrees | 6/18 | 3/3 (2 of 3 branches conflict every run) | 49 s | $0.037 |
| memro | **18/18** | **0/3** | 77 s | $0.048 |

**The shared-folder loss was silent.** In run `3f2490aa`, Codex reported
"Exported clearCart from index.js… `node --test` passes", but the final
`index.js` has no `clearCart`: a Claude agent rewrote the same export line a
moment later. Every agent reported success.

**Worktrees lose most of the work at merge time.** Every run, `claude-a`'s
branch merged and `claude-b`'s and `codex-c`'s conflicted (on `cart.js` and
`index.js`), so four of six tasks needed a human to resolve. That is the
strict reading; a person or an agent resolving conflicts would land more, at
the cost of their time.

**Memro is slower.** Turns that touched a file another agent was editing took
longer (Codex T6 about 50 s against about 33 s in shared), which fits waiting
for the file. The hooks don't log their waits yet, so this is inferred from
timings, not counted.

## 4. Caveats

- **Small sample.** 17/18 against 18/18 is one task. The shared-folder loss
  rate needs many more runs to be measured; the pilot shows that silent loss
  happens, not how often.
- **The edit tools already protect a lot.** Claude Code's Edit and Codex's
  apply_patch refuse to apply against a file that changed since it was read,
  which is why the shared folder lost only one task. Losses come from
  near-simultaneous rewrites of the same lines.
- **Arms were not interleaved**: memro ran after the other two, so model
  latency at a different time may account for some of its wall time.
- **Not measured**: shell writes (the hooks don't watch Bash), `git stash` or
  `checkout` by one agent wiping another's uncommitted work, and Memro's
  worktree warning (the memro arm used one folder).

## 5. Next

- Log hook waits and refusals so the memro arm reports them instead of
  inferring them from time.
- Launch run: more agents (mostly Claude haiku, at most one Codex, per cost
  preference), 20 tasks, 10 interleaved runs per arm, and larger tasks that
  rewrite whole files.
- Add a worktrees + memro arm to measure the merge-conflict warning.
- Add a git-hazard task (one agent stashes or checks out mid-run).
