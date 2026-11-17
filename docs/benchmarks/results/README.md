# Benchmark results

Raw results of every benchmark run, kept next to the write-ups so anyone can
check a number or rerun the report. The write-ups are in
[`../journal/`](../journal/); each run folder here is named as in its post.

Local paths are replaced with `~` and `$TMPDIR`. Server logs (`api.log`) are
left out: they are debugging output, not results. Partial or stopped runs keep
their own names (`*.bak`, `*.partial-*`) and aren't counted in any post.

| Folder | What | Write-up |
|---|---|---|
| `coordination/pilot-20261116` | collision pilot, 3 agents incl. Codex | [2026-11-16](../journal/2026-11-16-coordination-collision-pilot.md) |
| `coordination/launch-20261117` | launch run, 4 arms × 10 | [2026-11-17](../journal/2026-11-17-coordination-collision-launch.md) |
| `coordination/launch-20261117-v2` | rerun with the retry fix | [2026-11-17 §6](../journal/2026-11-17-coordination-collision-launch.md) |
| `coordination/light*-20261117` | runs from the `memro-light` worktree | none yet |
| `coordination/hold10-wait80-20261118` | 10 s holds, 80 s waits | [2026-11-18](../journal/2026-11-18-coordination-land-and-holds.md) |
| `coordination/land-smoke-20261118`, `land-20261118` | the four worktree arms, `memro land` | [2026-11-18](../journal/2026-11-18-coordination-land-and-holds.md) |
| `relay/pilot-20261117`, `pilot2-20261117`, `v2-20261118` | relay: does memory reach the next agent | [2026-11-17](../journal/2026-11-17-relay-benchmark.md) |
| `relay/smoke*` | harness smoke tests | – |
| `longmemeval/*` | LongMemEval runs (`ledger.*.json` lists them per worktree) | [2026-10-23](../journal/2026-10-23-longmemeval-so-far.md), [2026-11-15 baselines](../journal/2026-11-15-baselines-no-memory-and-plain-rag.md), [2026-11-15 evolution](../journal/2026-11-15-evolution-engine-and-contradictions.md) |
| `locomo/locomo-openai` | LoCoMo scores | none yet |

Print a coordination or relay table again with the harness's `--report`:

    python3 bench/coordination/run.py --report docs/benchmarks/results/coordination/land-20261118/results.jsonl
    mcp/.venv/bin/python bench/relay/run.py --report docs/benchmarks/results/relay/v2-20261118/results.jsonl

A file named `<name>.<worktree>.<ext>` is a run of the same name from another
worktree whose results differ.
