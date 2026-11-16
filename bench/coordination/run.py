"""Multi-agent collision benchmark: do agents working on one repo at the same
time lose each other's work, and does Memro coordination stop it?

Three agents (two Claude Code, one Codex) each do two of six small tasks on a
fixture repo, all at once. The tasks are chosen to collide: four touch
src/cart.js, three touch src/index.js, two touch src/format.js. Arms:

  shared     one folder, no coordination (agents side by side, as in herdr)
  worktrees  one git worktree per agent, merged into main at the end
  memro      one folder, Memro hooks (claim files, wait for busy ones)

Score: tasks whose hidden acceptance test passes in the final repo, plus wall
time and cost. Run from the repo root:

  python3 bench/coordination/run.py --arms shared,worktrees,memro --runs 3

Needs `claude` on PATH, Codex (CODEX, default: the copy bundled with the
ChatGPT app), and for the memro arm `memro connect`'s saved key plus Codex
hooks installed and trusted (`/hooks` in Codex).
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
}
# Each agent does its tasks in order, alongside the others.
AGENTS = {"claude-a": ("claude", ["T1", "T3"]), "claude-b": ("claude", ["T4", "T5"]), "codex-c": ("codex", ["T6", "T2"])}

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
               "--permission-mode", "acceptEdits", "--allowedTools", "Bash(node --test*)",
               # No user settings or MCP servers: arms differ only in the project's hooks.
               "--setting-sources", "project,local", "--strict-mcp-config"]
    else:
        cmd = [CODEX, "exec", "--json", "-s", "workspace-write", "--skip-git-repo-check", "-C", str(cwd),
               "-c", 'model_reasoning_effort="low"', "-c", "mcp_servers.memro.enabled=false"]
        if arm != "memro":
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


def accept(repo):
    """Which tasks' hidden acceptance tests pass in this checkout."""
    shutil.copytree(HERE / "accept", repo / "accept-tests")
    passed = {}
    for t in TASKS:
        passed[t] = sh("node", "--test", f"accept-tests/{t}.test.js", cwd=repo, check=False).returncode == 0
    base_ok = sh("node", "--test", "test/base.test.js", cwd=repo, check=False).returncode == 0
    shutil.rmtree(repo / "accept-tests")
    return passed, base_ok


def one_run(arm, workdir):
    run_id = uuid.uuid4().hex[:8]
    repo = workdir / f"{arm}-{run_id}"
    make_repo(repo, run_id)
    cwds = {name: repo for name in AGENTS}
    if arm == "worktrees":
        for name in AGENTS:
            cwds[name] = workdir / f"{arm}-{run_id}-{name}"
            sh("git", "worktree", "add", "-q", "-b", name, str(cwds[name]), cwd=repo)
    if arm == "memro":
        # The project-scoped Claude Code hooks, exactly as `memro connect --project` installs them.
        sh("node", "-e", f'import("{ROOT}/cli/src/clients.js").then(m => m.installHooks({{scope: "project", cwd: "{repo}"}}))', cwd=ROOT)

    def agent(name):
        kind, tasks = AGENTS[name]
        return name, [dict(task=t, **run_agent(kind, PROMPT.format(task=TASKS[t]), cwds[name], arm)) for t in tasks]

    t0 = time.time()
    with ThreadPoolExecutor(len(AGENTS)) as pool:
        turns = dict(pool.map(agent, AGENTS))
    wall = round(time.time() - t0, 1)

    merge = {}
    if arm == "worktrees":
        git = ("git", "-c", "user.name=bench", "-c", "user.email=bench@memro.co")
        for name in AGENTS:
            sh("git", "add", "-A", cwd=cwds[name])
            sh(*git, "commit", "-qm", name, "--allow-empty", cwd=cwds[name])
            ok = sh(*git, "merge", "--no-edit", name, cwd=repo, check=False).returncode == 0
            if not ok:
                sh("git", "merge", "--abort", cwd=repo, check=False)
            merge[name] = "merged" if ok else "conflict"
    passed, base_ok = accept(repo)
    return {
        "arm": arm, "run_id": run_id, "wall_seconds": wall, "passed": passed, "tasks_passed": sum(passed.values()),
        "base_tests_pass": base_ok, "merge": merge, "turns": turns,
        "claude_cost_usd": round(sum(t.get("cost_usd") or 0 for ts in turns.values() for t in ts), 4),
        "codex_tokens": sum((t.get("tokens") or {}).get("input_tokens", 0) + (t.get("tokens") or {}).get("output_tokens", 0)
                            for ts in turns.values() for t in ts),
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--arms", default="shared,worktrees,memro")
    ap.add_argument("--runs", type=int, default=3)
    ap.add_argument("--out", default=str(ROOT / "bench" / "runs" / "coordination" / time.strftime("%Y%m%d-%H%M%S")))
    args = ap.parse_args()
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
                print(f"run {i + 1} {arm:9} {r['tasks_passed']}/6 tasks  base {'ok' if r['base_tests_pass'] else 'BROKEN'}"
                      f"  {r['wall_seconds']}s  ${r['claude_cost_usd']} + {r['codex_tokens']} codex tokens  {r['merge'] or ''}", flush=True)
    print(f"results: {out / 'results.jsonl'}  repos: {workdir}")


if __name__ == "__main__":
    main()
