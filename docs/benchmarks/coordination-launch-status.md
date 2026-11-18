# Agent coordination launch: where it stands

Updated 2026-11-18. The pass bars are in
[coordination-v3-plan.md](coordination-v3-plan.md), written before the runs
and not changed since; this page tracks results against them and the launch
decisions around them. Every number links to a journal post, and every run's
raw results are in [`results/`](results/README.md).

## Position

The tools people run parallel agents with launch, isolate and show agents;
none shares state across tools or keeps memory between sessions. Memro sits
under them, so every claim is "setup + Memro" against "setup alone", never
"Memro instead of X".

| Tool | Main strength | How agents are kept apart | Benchmark arm that stands for it |
|---|---|---|---|
| herdr | agent-aware terminal panes, status and wake | one shared folder | `shared` (a real `herdr` arm is planned) |
| tmux, cmux | terminal orchestration | one shared folder | `shared` |
| Orca, T3 Code, Paradev | parallel agents in built-in worktrees | one worktree per agent | `worktrees`, `worktrees-land` |

Each tool's actual merge behaviour is to be checked against its documentation
before it is named publicly; until then posts describe the workflow, not the
tool.

## Results against the pass bars

| Bar | Result | Status | Write-up |
|---|---|---|---|
| 1. Worktrees + Memro land ≥ 90/100 and more than plain git | `memro land` 100/100, agents landing with plain git 100/100, merged afterwards 20/100. Memro 38% cheaper (every run) and 21% faster (median) | ≥ 90 met; "more than git" not met | [2026-11-18](journal/2026-11-18-coordination-land-and-holds.md) |
| 2. One folder: Memro ≤ 1.3× the shared folder's time, no give-ups | 100/100 vs 99/100, no give-ups, but 2.0× the time (97 s vs 49 s) | not met | [2026-11-18](journal/2026-11-18-coordination-land-and-holds.md) |
| 3. Memory beats a notes file where the file can't follow | Same checkout: tie (15/15 each). Teammate's clone, notes committed: tie (10/10 each). 300-fact memory: tools alone lost (6/10 vs 10/10); with the hooks of ADR 0035, Memro 10/10, notes 9/10 | not met (ties, one relay ahead) | [relay §5, §7, §8](journal/2026-11-17-relay-benchmark.md) |
| 4. Holds on the v3 task set (real repo, mixed tools, two models) | not run | – | – |

What is solid already, and can be said publicly:
- A shared folder loses work to one agent's `git stash` (3 of 10 runs) and
  worktrees merged afterwards lose most overlapping work (4 of 5 branches
  conflict); agents that land their own work fix the second.
- Memory, of any kind, is the whole difference for a fresh agent: without it
  the obvious mistake was made 30 of 30 times on the working traps.
- A notes file written by an agent can drift from what the user said; the
  hooks keep the user's words.

## Next, in order

The steps for each, with commands, are in the [roadmap](../roadmap.md).

1. **Speed in one folder (bar 2)**: plan claims, so overlapping tasks are split
   or ordered at the prompt instead of queued edit by edit.
2. **Landing under load (bar 1)**: 10 agents, more overlap, a stronger model;
   with 5 Haiku agents plain git already lands everything.
3. **Memory a file can't match (bar 3)**: thousands of facts (too many to read
   whole) and rules shared across repos.
4. **v3 task set (bar 4)** with a real `herdr` arm and Codex/Gemini CLI agents.

## Launch decisions

- **GitHub**: [discussion #93](https://github.com/memrohq/memro/discussions/93)
  (Ideas, 2026-11-18) asks how people run parallel agents and whether the
  baselines are fair, linking the plan on `main`.
- **X**: fine to post now about the problem and the pre-set pass bars, linking
  the discussion; no claims about Memro's results.
- **Hacker News**: only after a bar is met with a rerunnable result. One Show
  HN, titled by the problem ("We ran 400 parallel coding-agent sessions – here's
  how they lose work"), first comment stating where Memro is slower, posted on
  a weekday morning US Eastern with someone answering for a few hours.
- **memro.co**: the numbers and the rerun command go on the site with the
  Hacker News post, not before.
- **Results are published as they come out**, ties and losses included.

## "Multiplayer AI"

Sessions anyone can watch, redirect and hand off are being built by the
coding tools themselves, each inside its own tool. Memro already covers
seeing who does what (`memro agents`), redirecting (`memro send`) and handing
off (handoff notes, `memro land`, shared memory) across tools. The missing
piece, a view of it, is planned as a read-only cross-tool team board after the
launch benchmark (ADR 0019, PR #95); live viewing of an agent's steps is left
out, since it needs transcripts and Memro keeps facts, not sessions.

## Open pull requests (2026-11-18)

| PR | What |
|---|---|
| #94 | relay benchmark: harness, stages 1–2, hooks run, write-up |
| #95 | ADR 0019: team board plan |
| #96 | write-up: landing worktrees, shorter holds |
| #97 | every run's raw results in `docs/benchmarks/results/` |
| #98 | hooks save stated rules (ADR 0035); open question: an opt-out for rule saving |
