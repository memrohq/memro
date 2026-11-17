"""Multi-agent collision benchmark: do agents working on one repo at the same
time lose each other's work, and does Memro coordination stop it?

Agents each do two small tasks on a fixture repo, all at once. The tasks
collide on purpose (most touch src/cart.js and src/index.js), and one (T10)
checks the base tests on a clean tree with `git stash` / `git stash pop` while
the others are mid-edit. Arms:

  shared           one folder, no coordination (agents side by side, as in herdr)
  worktrees        one git worktree per agent, merged into main at the end
  memro            one folder, Memro hooks (claim files, wait for busy ones, git guard)
  memro-worktrees  one worktree per agent with Memro hooks (merge-conflict warnings)

Score: tasks whose hidden acceptance test passes in the final repo, wall time,
cost, and for the memro arms the hooks' waits, refusals and warnings
(~/.memro/hooks/log.jsonl). Run from the repo root:

  python3 bench/coordination/run.py --agents launch --runs 10
  python3 bench/coordination/run.py --report bench/runs/coordination/<run>/results.jsonl

Needs `claude` on PATH; the pilot agent set also needs Codex (CODEX, default:
the copy bundled with the ChatGPT app) with its Memro hooks trusted in
`/hooks`. The memro arms need `memro connect`'s saved key.
"""

import argparse
import json
import os
import shutil
import subprocess
import tempfile
import time
import uuid
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
CODEX = os.environ.get("CODEX", "/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex")

TASKS = {
    "T1": "Add removeItem(cart, id) to src/cart.js: it returns a new cart without the item with that id, without mutating the input. Export it from src/index.js.",
    "T2": "Change total(cart) in src/cart.js to total(cart, { percentOff } = {}): apply a percentage discount to the sum and round to the nearest cent with Math.round. Without percentOff it behaves as before.",
    "T3": 'Change formatPrice(cents) in src/format.js to formatPrice(cents, symbol = "$") so it uses the given currency symbol, e.g. formatPrice(1250, "€") is "€12.50".',
    "T4": "Add itemCount(cart) to src/cart.js: the sum of qty over all items. Export it from src/index.js.",
    "T5": 'Make formatPrice in src/format.js add thousands separators: formatPrice(123456789) is "$1,234,567.89".',
    "T6": "Add clearCart(cart) to src/cart.js: it returns a new cart with the same fields and an empty items array, without mutating the input. Export it from src/index.js.",
    "T7": "Add updateQty(cart, id, qty) to src/cart.js: it returns a new cart where the item with that id has the given qty, without mutating the input. Export it from src/index.js.",
    "T8": 'Add formatCart(cart) to src/format.js: one line per item, "<id> x<qty>: <formatPrice(price * qty)>", joined with newlines. Export it from src/index.js.',
    "T9": "Add totalWithTax(cart, ratePercent) to src/cart.js: the cart total plus that percentage of tax, rounded to the nearest cent with Math.round. Export it from src/index.js.",
    "T10": ("Add hasItem(cart, id) to src/cart.js (true when an item has that id) and export it from src/index.js. "
            "Then check the base tests still pass on the last commit: run `git stash`, then `node --test test/base.test.js`, then `git stash pop`."),
}
# Each agent does its tasks in order, alongside the others. "launch" is all
# Claude Code: the Codex hooks only run after the user trusts them again.
AGENT_SETS = {
    "pilot": {"claude-a": ("claude", ["T1", "T3"]), "claude-b": ("claude", ["T4", "T5"]), "codex-c": ("codex", ["T6", "T2"])},
    "launch": {"claude-a": ("claude", ["T1", "T3"]), "claude-b": ("claude", ["T4", "T5"]), "claude-c": ("claude", ["T6", "T2"]),
               "claude-d": ("claude", ["T7", "T8"]), "claude-e": ("claude", ["T10", "T9"])},
}
AGENTS = AGENT_SETS["launch"]
LOG = Path.home() / ".memro" / "hooks" / "log.jsonl"

PROMPT = ("You are working in a small JavaScript repo. Task: {task} "
          "Keep the existing behaviour and tests passing (run `node --test`). Do not edit or add test files. Do not commit.")


def sh(*cmd, cwd, check=True):
    return subprocess.run(cmd, cwd=cwd, check=check, capture_output=True, text=True)


def make_repo(path, run_id):
    shutil.copytree(HERE / "fixture", path)
    sh("git", "init", "-q", "-b", "main", cwd=path)
    # A unique origin name makes every run's Memro leases distinct.
    sh("git", "remote", "add", "origin", f"git@bench.memro.co:bench/shop-{run_id}.git", cwd=path)
    sh("git", "add", "-A", cwd=path)
    sh("git", "-c", "user.name=bench", "-c", "user.email=bench@memro.co", "commit", "-qm", "fixture", cwd=path)


def run_agent(kind, prompt, cwd, arm):
    """One agent turn, headless. Returns cost/token figures and the final message."""
    t0 = time.time()
    if kind == "claude":
        cmd = ["claude", "-p", prompt, "--model", "haiku", "--output-format", "json",
               "--permission-mode", "acceptEdits", "--allowedTools", "Bash(node --test*)", "Bash(git stash*)",
               # No user settings or MCP servers: arms differ only in the project's hooks.
               "--setting-sources", "project,local", "--strict-mcp-config"]
    else:
        cmd = [CODEX, "exec", "--json", "-s", "workspace-write", "--skip-git-repo-check", "-C", str(cwd),
               "-c", 'model_reasoning_effort="low"', "-c", "mcp_servers.memro.enabled=false"]
        if not arm.startswith("memro"):
            cmd += ["-c", "features.hooks=false"]
        cmd.append(prompt)
    p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=900)
    out = {"kind": kind, "seconds": round(time.time() - t0, 1), "exit": p.returncode}
    if kind == "claude":
        try:
            j = json.loads(p.stdout)
            out.update(cost_usd=j.get("total_cost_usd"), result=(j.get("result") or "")[-400:])
        except json.JSONDecodeError:
            out["result"] = (p.stdout + p.stderr)[-400:]
    else:
        usage, last = {}, ""
        for line in p.stdout.splitlines():
            try:
                ev = json.loads(line)
            except json.JSONDecodeError:
                continue
            if ev.get("type") == "turn.completed":
                usage = ev.get("usage", {})
            item = ev.get("item") or {}
            if item.get("type") == "agent_message":
                last = item.get("text", "")
        out.update(tokens=usage, result=last[-400:] or p.stderr[-400:])
    out["memro_refused"] = "is being changed by" in out.get("result", "")
    return out


def accept(repo, tasks):
    """Which tasks' hidden acceptance tests pass in this checkout."""
    shutil.copytree(HERE / "accept", repo / "accept-tests")
    passed = {}
    for t in tasks:
        passed[t] = sh("node", "--test", f"accept-tests/{t}.test.js", cwd=repo, check=False).returncode == 0
    base_ok = sh("node", "--test", "test/base.test.js", cwd=repo, check=False).returncode == 0
    shutil.rmtree(repo / "accept-tests")
    return passed, base_ok


def hook_events(since, until):
    """The Memro hooks' logged waits, refusals and warnings during one run."""
    counts = {}
    try:
        for line in LOG.read_text().splitlines():
            e = json.loads(line)
            if since <= e.get("t", "") <= until:
                counts[e["outcome"]] = counts.get(e["outcome"], 0) + 1
    except (OSError, ValueError):
        pass
    return counts


def one_run(arm, workdir):
    run_id = uuid.uuid4().hex[:8]
    repo = workdir / f"{arm}-{run_id}"
    make_repo(repo, run_id)
    cwds = {name: repo for name in AGENTS}
    worktrees = arm in ("worktrees", "memro-worktrees")
    if worktrees:
        for name in AGENTS:
            cwds[name] = workdir / f"{arm}-{run_id}-{name}"
            sh("git", "worktree", "add", "-q", "-b", name, str(cwds[name]), cwd=repo)
    if arm.startswith("memro"):
        # The project-scoped Claude Code hooks, exactly as `memro connect --project` installs them.
        for cwd in set(cwds.values()):
            sh("node", "-e", f'import("{ROOT}/cli/src/clients.js").then(m => m.installHooks({{scope: "project", cwd: "{cwd}"}}))', cwd=ROOT)

    def agent(name):
        kind, tasks = AGENTS[name]
        return name, [dict(task=t, **run_agent(kind, PROMPT.format(task=TASKS[t]), cwds[name], arm)) for t in tasks]

    iso = lambda: time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime())
    since, t0 = iso(), time.time()
    with ThreadPoolExecutor(len(AGENTS)) as pool:
        turns = dict(pool.map(agent, AGENTS))
    wall = round(time.time() - t0, 1)
    until = iso() + "Z"

    merge = {}
    if worktrees:
        git = ("git", "-c", "user.name=bench", "-c", "user.email=bench@memro.co")
        for name in AGENTS:
            sh("git", "add", "-A", cwd=cwds[name])
            sh(*git, "commit", "-qm", name, "--allow-empty", cwd=cwds[name])
            ok = sh(*git, "merge", "--no-edit", name, cwd=repo, check=False).returncode == 0
            if not ok:
                sh("git", "merge", "--abort", cwd=repo, check=False)
            merge[name] = "merged" if ok else "conflict"
    tasks = [t for _, ts in AGENTS.values() for t in ts]
    passed, base_ok = accept(repo, tasks)
    # Work a `git stash` took away and never gave back.
    stranded = len(sh("git", "stash", "list", cwd=repo, check=False).stdout.splitlines())
    return {
        "arm": arm, "run_id": run_id, "wall_seconds": wall, "passed": passed, "tasks_passed": sum(passed.values()),
        "tasks": len(tasks), "base_tests_pass": base_ok, "merge": merge, "stranded_stashes": stranded,
        "hooks": hook_events(since, until) if arm.startswith("memro") else {}, "turns": turns,
        "claude_cost_usd": round(sum(t.get("cost_usd") or 0 for ts in turns.values() for t in ts), 4),
        "codex_tokens": sum((t.get("tokens") or {}).get("input_tokens", 0) + (t.get("tokens") or {}).get("output_tokens", 0)
                            for ts in turns.values() for t in ts),
    }


def report(path):
    """A table per arm from a results.jsonl."""
    import statistics as st
    rows = [json.loads(l) for l in open(path)]
    arms = {}
    for r in rows:
        arms.setdefault(r["arm"], []).append(r)
    print(f"{'arm':16} {'runs':>4} {'tasks landed':>14} {'runs w/ loss':>12} {'conflicts':>9} {'stranded':>8} {'median s':>8} {'cost $':>7}  hooks")
    for arm, rs in arms.items():
        landed, total = sum(r["tasks_passed"] for r in rs), sum(r.get("tasks", 6) for r in rs)
        hooks = {}
        for r in rs:
            for k, v in r.get("hooks", {}).items():
                hooks[k] = hooks.get(k, 0) + v
        print(f"{arm:16} {len(rs):>4} {f'{landed}/{total} ({100 * landed / total:.0f}%)':>14} "
              f"{sum(r['tasks_passed'] < r.get('tasks', 6) for r in rs):>12} "
              f"{sum(v == 'conflict' for r in rs for v in r['merge'].values()):>9} "
              f"{sum(r.get('stranded_stashes', 0) for r in rs):>8} {st.median(r['wall_seconds'] for r in rs):>8.0f} "
              f"{sum(r['claude_cost_usd'] for r in rs):>7.2f}  {hooks or ''}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--arms", default="shared,worktrees,memro,memro-worktrees")
    ap.add_argument("--agents", choices=sorted(AGENT_SETS), default="launch")
    ap.add_argument("--runs", type=int, default=3)
    ap.add_argument("--out", default=str(ROOT / "bench" / "runs" / "coordination" / time.strftime("%Y%m%d-%H%M%S")))
    ap.add_argument("--report", metavar="RESULTS_JSONL", help="print the summary table of a finished run and exit")
    args = ap.parse_args()
    if args.report:
        return report(args.report)
    global AGENTS
    AGENTS = AGENT_SETS[args.agents]
    out = Path(args.out).resolve()  # agents run in other folders
    out.mkdir(parents=True, exist_ok=True)
    # Outside any project: Claude Code reads CLAUDE.md files from parent folders.
    workdir = Path(tempfile.mkdtemp(prefix="memro-cbench-")).resolve()
    arms = args.arms.split(",")
    with open(out / "results.jsonl", "a") as f:
        # Interleave arms so a slow model hour doesn't land on one arm.
        for i in range(args.runs):
            for arm in arms:
                r = one_run(arm, workdir)
                f.write(json.dumps(r) + "\n")
                f.flush()
                print(f"run {i + 1} {arm:16} {r['tasks_passed']}/{r['tasks']} tasks  base {'ok' if r['base_tests_pass'] else 'BROKEN'}"
                      f"  stranded {r['stranded_stashes']}  {r['wall_seconds']}s  ${r['claude_cost_usd']}  {r['merge'] or ''} {r['hooks'] or ''}", flush=True)
    report(out / "results.jsonl")
    print(f"results: {out / 'results.jsonl'}  repos: {workdir}")


if __name__ == "__main__":
    main()
