#!/bin/sh
# The rule-depth grid (journal 2026-11-19): 650 B sessions against a local
# server. Rerun to resume; each run picks up where its --out stopped.
cd "$(dirname "$0")/../.." || exit 1
export MEMRO_API_URL=${MEMRO_API_URL:-http://localhost:8081} MEMRO_CLI=$PWD/cli
py=mcp/.venv/bin/python
grid() {
  for at in 25 50 75; do
    $py bench/relay/run.py --setting huge --traps R1,R2 --plant $at --arms notes,automem,memro-hooks,memro-plain --runs 25 --out bench/runs/relay/depth-$at || return 1
  done
  # No memory: the rule's depth doesn't matter, 50 trials in all.
  $py bench/relay/run.py --setting huge --traps R1,R2 --plant 50 --arms none --runs 25 --out bench/runs/relay/depth-none
}
for attempt in 1 2 3 4 5 6 7 8; do
  grid && exec $py bench/relay/run.py --report bench/runs/relay/depth-*/results.jsonl
  echo "attempt $attempt stopped; retrying in 30 minutes" >&2
  sleep 1800
done
exit 1
