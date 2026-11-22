"""GitHub history benchmark: does Memro's GitHub connector save a coding agent
tokens when it answers questions about a repo's past, compared with the gh CLI?

Each question is put to a fresh headless Claude Code agent (Haiku) in four arms:

  none         no tools (the floor: what the model guesses)
  gh           only `gh pr`, `gh issue` and `gh search` on the repo
  memro-raw    only Memro's MCP recall, over what the GitHub connector stores
               (rendered and chunked by the connector's own code, see
               server/internal/connectors/github_dump_test.go)
  memro-facts  the same, over facts Claude Haiku extracted from each issue/PR

An answer is graded against the reference by Claude Sonnet. Tokens are all the
input tokens the agent's turns read (input + cache writes + cache reads).

    python3 bench/github_history/fetch.py memrohq/memro
    (cd server && MEMRO_GITHUB_DUMP=... MEMRO_GITHUB_CHUNKS=... go test ./internal/connectors -run TestGitHubDumpChunks)
    mcp/.venv/bin/python bench/github_history/run.py --out bench/runs/github-history/<run>
    mcp/.venv/bin/python bench/github_history/run.py --report bench/runs/github-history/<run>

Needs a local server (MEMRO_API, default http://127.0.0.1:18090, started with
MEMORY_SOFT_CAP=100000) and its database (MEMRO_DATABASE_URL) to register agents.
"""
import argparse
import json
import math
import os
import pathlib
import random
import re
import statistics
import subprocess
import sys
import time

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[1]
sys.path.insert(0, str(ROOT / "bench" / "longmemeval"))
from common import claude_cli  # noqa: E402

REPO = os.environ.get("BENCH_REPO", "memrohq/memro")
DATA = HERE / "data"
DUMP = DATA / f"{REPO.replace('/', '__')}.json"
CHUNKS = DATA / f"{REPO.replace('/', '__')}.chunks.jsonl"
FACTS = DATA / "facts-haiku"
API = os.environ.get("MEMRO_API", "http://127.0.0.1:18090")
DB = os.environ.get("MEMRO_DATABASE_URL", "postgres://memro@localhost:55434/memro?sslmode=disable")
MEMRO_MCP = ROOT / "mcp" / ".venv" / "bin" / "memro-mcp"
ARMS = ["none", "gh", "memro-raw", "memro-facts"]

SYSTEM = (f"You answer a developer's question about the history of the GitHub repository {REPO}: "
          "its pull requests and issues. {how} Answer in at most three sentences. "
          "If you can't find the answer, say you don't know rather than guess.")
HOW = {
    "none": "You have no tools; answer from what you know.",
    "gh": f"Look it up with the gh CLI (gh pr, gh issue and gh search, always with -R {REPO}).",
    "memro-raw": "Look it up with the recall tool, which searches a memory synced from the repository's issues and pull requests.",
    "memro-facts": "Look it up with the recall tool, which searches a memory synced from the repository's issues and pull requests.",
}
TOOLS = {
    "none": ["--tools", ""],
    "gh": ["--tools", "Bash", "--allowedTools", "Bash(gh pr:*)", "Bash(gh issue:*)", "Bash(gh search:*)"],
    "memro-raw": ["--tools", "", "--allowedTools", "mcp__memro__recall"],
    "memro-facts": ["--tools", "", "--allowedTools", "mcp__memro__recall"],
}

EXTRACT = ("You will be given one GitHub issue or pull request with its discussion. Write down every fact in it a "
           "developer might later ask about: what changed and why, decisions, numbers, limits, defaults, names of "
           "commands, flags, settings and files, what was left out or is still open. One fact per line, each "
           "self-contained (name the feature, never just \"it\"). No commentary or headings.")
GRADE = ("Question: {q}\nReference answer: {ref}\nResponse: {resp}\n\nDoes the response give the reference answer's "
         "key facts, without contradicting them? Extra correct detail is fine. A response that says it doesn't know "
         "is wrong. Reply with only yes or no.")


def psql(sql):
    return subprocess.run(["psql", DB, "-tAc", sql], capture_output=True, text=True, check=True).stdout.strip()


def facts():
    """One memory per fact Haiku extracted from each item, with the connector's header in front."""
    out = []
    by_item = {}
    for line in CHUNKS.read_text().splitlines():
        c = json.loads(line)
        by_item.setdefault(c["metadata"]["number"], c)
    for n, c in by_item.items():
        f = FACTS / f"{n}.json"
        if not f.exists():
            full = next(i for i in json.loads(DUMP.read_text()) if i["number"] == n)
            text = "\n\n".join([c["content"].split("\n", 1)[0], full.get("body") or ""] +
                               [x["body"] for x in full.get("_comments", [])])
            lines = [re.sub(r"^\s*(?:[-*•]|\d+[.)])\s*", "", l).strip() for l in claude_cli("haiku", EXTRACT, text).splitlines()]
            FACTS.mkdir(parents=True, exist_ok=True)
            f.write_text(json.dumps([l for l in lines if l]))
        header = c["content"].split("\n", 1)[0]
        out += [{"content": f"{header}\n{x}", "metadata": c["metadata"], "event_date": c["event_date"]}
                for x in json.loads(f.read_text())]
    return out


def memory(arm, out):
    """A Memro agent holding the arm's memories, made once per run and reused on resume."""
    saved = out / f"agent-{arm}.json"
    if saved.exists():
        return json.loads(saved.read_text())
    from memro import Agent, MemroClient, generate_keypair
    pub, priv = generate_keypair()
    psql(f"INSERT INTO agent_identities (id, tenant_id) VALUES ('{pub}', '00000000-0000-0000-0000-000000000000')")
    agent = Agent(agent_id=pub, private_key=priv, client=MemroClient(API))
    items = [json.loads(l) for l in CHUNKS.read_text().splitlines()] if arm == "memro-raw" else facts()
    for k in range(0, len(items), 500):
        agent.remember_batch([{"content": m["content"], "type": "semantic", "metadata": m["metadata"],
                               "event_date": m["event_date"]} for m in items[k:k + 500]])
    while int(psql("SELECT count(*) FROM memories m LEFT JOIN memory_embeddings e ON e.memory_id = m.id "
                   f"WHERE m.agent_id = '{pub}' AND e.memory_id IS NULL")):
        time.sleep(2)
    creds = {"agent_id": pub, "private_key": priv, "memories": len(items)}
    saved.write_text(json.dumps(creds))
    return creds


def ask(arm, question, out, creds):
    cwd = out / "_cwd"
    cwd.mkdir(exist_ok=True)
    cmd = ["claude", "-p", question, "--model", "haiku", "--output-format", "stream-json", "--verbose",
           "--system-prompt", SYSTEM.format(how=HOW[arm]), "--setting-sources", "local", "--strict-mcp-config",
           "--no-session-persistence", "--disable-slash-commands", *TOOLS[arm]]
    if creds:
        cfg = out / f"mcp-{arm}.json"
        cfg.write_text(json.dumps({"mcpServers": {"memro": {"command": str(MEMRO_MCP), "env": {
            "MEMRO_BASE_URL": API, "MEMRO_AGENT_ID": creds["agent_id"], "MEMRO_PRIVATE_KEY": creds["private_key"]}}}}))
        cmd += ["--mcp-config", str(cfg)]
    t0 = time.time()
    p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=600, stdin=subprocess.DEVNULL)
    row = {"seconds": round(time.time() - t0, 1), "tools": [], "answer": "", "error": None}
    for line in p.stdout.splitlines():
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        if ev.get("type") == "assistant":
            row["tools"] += [{"name": b["name"], "input": json.dumps(b.get("input", {}))[:300]}
                             for b in ev.get("message", {}).get("content", []) if b.get("type") == "tool_use"]
        elif ev.get("type") == "result":
            u = ev.get("usage") or {}
            row.update(answer=ev.get("result") or "", turns=ev.get("num_turns"), cost_usd=ev.get("total_cost_usd"),
                       input_tokens=u.get("input_tokens", 0) + u.get("cache_creation_input_tokens", 0)
                       + u.get("cache_read_input_tokens", 0), output_tokens=u.get("output_tokens", 0),
                       error=ev.get("result") if ev.get("is_error") else None)
    if not row["answer"] and not row["error"]:
        row["error"] = (p.stderr or p.stdout)[-400:]
    return row


def run(out, ids=None):
    out = out.resolve()  # the agents run in out/_cwd, and the MCP config path must still resolve there
    out.mkdir(parents=True, exist_ok=True)
    qs = [q for q in json.loads((HERE / "questions.json").read_text()) if not ids or q["id"] in ids]
    res = out / "results.jsonl"
    done = {(r["id"], r["arm"]) for r in map(json.loads, res.read_text().splitlines())} if res.exists() else set()
    agents = {a: memory(a, out) for a in ARMS if a.startswith("memro")}
    print({a: c["memories"] for a, c in agents.items()}, "memories")
    rng = random.Random(1122)
    for q in qs:
        arms = ARMS[:]
        rng.shuffle(arms)  # interleaved, in a fixed random order per question
        for arm in arms:
            if (q["id"], arm) in done:
                continue
            row = {"id": q["id"], "arm": arm, **ask(arm, q["question"], out, agents.get(arm))}
            if row["error"]:
                # A usage limit or crash isn't a result: stop, so a rerun with this --out redoes it.
                sys.exit(f"{q['id']} {arm}: {row['error']}")
            row["correct"] = "yes" in claude_cli("sonnet", "You grade answers. Reply with only yes or no.",
                                                 GRADE.format(q=q["question"], ref=q["answer"], resp=row["answer"])).lower()
            with res.open("a") as f:
                f.write(json.dumps(row) + "\n")
            print(f"{q['id']} {arm:11} {'ok ' if row['correct'] else 'bad'} {row['input_tokens']:>7} tok "
                  f"{row['turns']} turns {row['seconds']}s", flush=True)


def sign_test(wins, losses):
    n = wins + losses
    return min(1.0, 2 * sum(math.comb(n, i) for i in range(min(wins, losses) + 1)) / 2 ** n) if n else 1.0


def report(out):
    rows = [json.loads(l) for l in (out / "results.jsonl").read_text().splitlines()]
    by = {(r["id"], r["arm"]): r for r in rows}
    ids = sorted({r["id"] for r in rows})
    print(f"| Arm | Correct | Median input tokens | Mean input tokens | Median turns | Median s | Cost $ |\n|---|---|---|---|---|---|---|")
    for a in ARMS:
        rs = [by[(i, a)] for i in ids if (i, a) in by]
        if rs:
            print(f"| {a} | {sum(r['correct'] for r in rs)}/{len(rs)} | {statistics.median(r['input_tokens'] for r in rs):,.0f} | "
                  f"{statistics.mean(r['input_tokens'] for r in rs):,.0f} | {statistics.median(r['turns'] for r in rs)} | "
                  f"{statistics.median(r['seconds'] for r in rs)} | {sum(r['cost_usd'] or 0 for r in rs):.2f} |")
    for a in ("memro-raw", "memro-facts"):
        pair = [(by[(i, a)], by[(i, "gh")]) for i in ids if (i, a) in by and (i, "gh") in by]
        if not pair:
            continue
        fewer = sum(m["input_tokens"] < g["input_tokens"] for m, g in pair)
        ratio = statistics.median(m["input_tokens"] / g["input_tokens"] for m, g in pair)
        fixed = sum(m["correct"] and not g["correct"] for m, g in pair)
        broken = sum(g["correct"] and not m["correct"] for m, g in pair)
        print(f"\n{a} vs gh: fewer tokens on {fewer}/{len(pair)} (sign test p = {sign_test(fewer, len(pair) - fewer):.3g}), "
              f"median ratio {ratio:.2f}; correct where gh wasn't {fixed}, the reverse {broken} "
              f"(McNemar p = {sign_test(fixed, broken):.3g})")


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", type=pathlib.Path)
    ap.add_argument("--report", type=pathlib.Path)
    ap.add_argument("--ids", nargs="*", help="only these questions (a smoke test)")
    a = ap.parse_args()
    report(a.report) if a.report else run(a.out, a.ids)
