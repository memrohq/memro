# Coordination benchmark v3: plan and pass bars

Written 2026-11-18, before the runs, so the result can't be fitted to them.
Whatever comes out is published, including ties and losses.

## The claim we want to be able to make

Parallel coding agents lose work in a shared folder and collide at merge time
in worktrees. **Any of the usual setups plus Memro lands more of the agents'
work, loses none, and passes knowledge from one agent to the next**, at a
stated cost in time. Memro sits under the tools people use to run agents
(herdr, tmux, cmux, Orca, T3 Code, Paradev); it is compared as "setup + Memro"
against "setup alone", not as a replacement for them.

## Setups (arms)

| Arm | Stands for |
|---|---|
| `shared` | agents side by side in one folder: herdr, tmux, cmux panes |
| `herdr` | the same, run in real herdr panes with its status and wake |
| `worktrees` | one worktree per agent, merged afterwards without resolving conflicts |
| `worktrees-land` | one worktree per agent; each agent rebases, resolves and fast-forwards its own work with git (the strongest worktree baseline) |
| `memro` | one folder with Memro hooks |
| `memro-worktrees-land` | worktrees with Memro hooks, landing through `memro land` |

Each comparison to a named tool is checked against that tool's documentation
before it is published; tools we can't run are described by the workflow
they use, not by name alone.

## Measures

- **Tasks landed**: hidden acceptance tests pass on `main` at the end.
- **Work lost**: tasks an agent finished that aren't on `main` (overwritten, stashed away, or left on a branch).
- **Merge conflicts** and **human steps** (an agent stopping to ask the user).
- **Wall time** and **cost**.
- Relay benchmark: **facts respected** by the next agent (`bench/relay`).

## Pass bars, to clear before anything is published

1. Landing (this amendment, ADR 0019 §8): `memro-worktrees-land` lands ≥ 90/100 on the launch task set, and more than `worktrees-land`.
2. Speed: `memro` median wall time ≤ 1.3× `shared`, with no task lost to an agent giving up.
3. Memory: on the relay traps, Memro respects the fact clearly more often than a notes file when the next agent is in a fresh clone or another tool (stage 2). Same-checkout parity with a notes file is reported as a tie.
4. Generality: the gaps in 1–3 hold on the v3 task set below, not only on the shop fixture.

## The v3 task set and runs

- Tasks cut from a real open-source repository's history, at three overlap levels (low, medium, high); real work overlaps less than the shop fixture, and the low level shows that honestly.
- Mixed tools: Claude Code with Codex and Gemini CLI, where only Memro shares state across tools.
- Two models: Haiku and one stronger model.
- 20 runs per arm and level, arms interleaved; results with 95% confidence intervals.
- One command reruns everything; raw `results.jsonl` and transcripts are published with the write-up.

## Order

1. Finish and write up the relay v2 and the hold-time runs (2026-11-18).
2. Land arms on the launch task set: bar 1.
3. Hold and wait defaults from the hold-time run: bar 2.
4. Relay stage 2: bar 3 (automatic capture first if agents don't call `remember`).
5. v3 task set, mixed tools, herdr arm: bar 4.
6. Publish: journal post, ADR and runbook, numbers and rerun command on memro.co, then a Show HN post led by the data, including where Memro is slower.
