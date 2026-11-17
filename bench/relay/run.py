"""Relay benchmark: does an agent that picks up another's work avoid its known
mistakes? Agent A gets a task plus a fact from the user that the code doesn't
show; a fresh agent B then gets a related task whose obvious solution breaks
that fact. Hidden tests check B's work. Design: bench/relay/README.md.

Arms differ only in how A's knowledge can reach B:
  none   nothing: B starts fresh
  notes  both are told to keep and read NOTES.md (the free baseline)
  memro  both have Memro's MCP tools, told to recall and remember

Settings (--setting), where B works and what it has to search:
  same   B in A's checkout (stage 1)
  clone  B is a teammate in a fresh clone; A's work is committed first,
         NOTES.md included, so the notes file follows (its strongest form)
  big    as same, but NOTES.md and Memro already hold 300 earlier facts on
         other topics, a dozen of them near the trap's subject

Claude Code's own auto-memory is switched off in every arm, so it can't carry
the fact on its own. The memro arm uses one throwaway Memro agent (not tied to
any account), wiped before every relay and deleted at the end.

Run with the MCP server's Python, which has the Memro SDK:

  mcp/.venv/bin/python bench/relay/run.py --runs 5
  mcp/.venv/bin/python bench/relay/run.py --report bench/runs/relay/<run>/results.jsonl
"""

import argparse
import json
import re
import shutil
import signal
import subprocess
import tempfile
import time
import uuid
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
FIXTURE = HERE.parent / "coordination" / "fixture"
MEMRO_MCP = ROOT / "mcp" / ".venv" / "bin" / "memro-mcp"
MEMRO_API = "https://api.memro.co"

# A's own task never touches what the fact is about, so the fact leaves no
# trace in the code (pilot v1: A wrote it into comments and B copied them).
TRAPS = {
    "R1": dict(  # a project rule mentioned in passing
        a="Add itemCount(cart) to src/cart.js: the sum of qty over all items. Export it from src/index.js.",
        fact="By the way, a rule for this whole repo: never use Intl or toLocaleString anywhere. Our POS devices run a Node build without ICU, and it crashed in production last month.",
        b="Add formatMoney(cents, locale, currency) to src/format.js and export it from src/index.js. It must handle two cases: "
          "formatMoney(123456, \"en-US\", \"USD\") is \"$1,234.56\" and formatMoney(123456, \"de-DE\", \"EUR\") is \"1.234,56 €\".",
        keyword=r"\bintl\b|\bicu\b|tolocalestring",
    ),
    "R2": dict(  # a decision made before any code exists
        a="Add removeItem(cart, id) to src/cart.js: a new cart without the item with that id, without mutating the input. Export it from src/index.js.",
        fact="Heads-up for later: when we add coupons or any other discount, the discounted total must always round down to a whole cent. Rounding up overcharged customers last quarter and finance had to refund them.",
        b="Add couponTotal(cart, percent) to src/cart.js: the cart total in cents with percent% off. Export it from src/index.js.",
        keyword=r"round(s|ing|ed)? down|math\.floor|\bfloor\b",
    ),
    "R3": dict(  # a migration under way elsewhere
        a="Add hasItem(cart, id) to src/cart.js: true when an item has that id. Export it from src/index.js.",
        fact="FYI, the new mobile app already sends cart items with `unitPrice` instead of `price`, and we'll migrate the rest later. Anything that reads an item's price has to accept both fields.",
        b="Add itemSubtotal(item) to src/cart.js: the item's price times its qty, in cents. Export it from src/index.js.",
        keyword=r"unitprice",
    ),
    "R4": dict(  # control: nothing to remember
        a="Add itemCount(cart) to src/cart.js: the sum of qty over all items. Export it from src/index.js.",
        fact="",
        b="Add hasItem(cart, id) to src/cart.js: true when an item has that id. Export it from src/index.js.",
        keyword=None,
    ),
}

# Earlier project facts for the big setting. None mention the traps' subjects
# (Intl/ICU, rounding direction, unitPrice), so "A recorded it" still only
# matches what A added.
NEAR = [
    "Prices are stored as integer cents everywhere; never store floats.",
    "Currency codes are uppercase ISO 4217 strings such as USD and EUR.",
    "Shipping fees are added after discounts, not before.",
    "Tax is calculated once per order, not per line item.",
    "Receipts print dates as YYYY-MM-DD.",
    "Gift cards can't be combined with coupons.",
    "Refunds go back to the original payment method.",
    "A cart holds at most 100 distinct items.",
    "Product ids are lowercase slugs.",
    "Prices shown to EU customers include VAT.",
    "Negative quantities are rejected by the API, not in cart.js.",
    "The free-shipping threshold is 5000 cents.",
    "Coupons expire at midnight UTC.",
    "Loyalty points are earned on the pre-tax total.",
    "formatPrice keeps the dollar sign as the default symbol for old callers.",
]
AREAS = ["checkout API", "admin dashboard", "search service", "email worker", "CI pipeline", "staging deploy",
         "database migrations", "feature flags", "image uploads", "auth tokens", "webhook handler", "inventory sync",
         "order export", "analytics events", "mobile release", "support inbox tool", "CDN cache", "rate limiter",
         "password reset flow", "nightly backups", "error reporting", "load tests", "translations pipeline",
         "partner API", "warehouse integration"]
RULES = ["{a}: logs at warn level in production; debug logging is for local runs only.",
         "{a}: owned by the platform team; ask them before changing its config.",
         "{a}: every change needs a feature flag so it can be switched off without a deploy.",
         "{a}: retries three times with exponential backoff before giving up.",
         "{a}: secrets come from the secret manager, never from .env files in the repo.",
         "{a}: timeouts are 10 seconds; longer calls go through the job queue.",
         "{a}: tests must not call the network; use the fakes in test/fakes.",
         "{a}: breaking changes need a deprecation notice one release ahead.",
         "{a}: errors are reported with the request id so support can find them.",
         "{a}: dates are stored in UTC and converted only when displayed.",
         "{a}: the old v1 endpoints stay until the March cleanup.",
         "{a}: deploys are frozen on Fridays after 3pm."]
EARLIER = NEAR + [r.format(a=a) for r in RULES for a in AREAS][:300 - len(NEAR)]

TAIL = " Keep the existing behaviour and tests passing (run `node --test`). Do not edit or add test files. Do not commit."
PROMPT_A = "You are working in a small JavaScript repo. Task: {task}{fact}" + TAIL
PROMPT_B = "You are working in a small JavaScript repo. Task: {task}" + TAIL

# Word for word the same in both arms but for where it's kept.
RECORD = "When you learn something a future agent working here would need (a decision and why, a constraint, something that failed), "
CLAUDE_MD = {
    "notes": "## Project notes\n\nBefore you start, read NOTES.md if it exists. " + RECORD + "add one short line to NOTES.md.\n",
    "memro": "## Memro (long-term memory)\n\nBefore you start, call the memro `recall` tool with a short query about your task. " + RECORD + "save one short sentence with the memro `remember` tool.\n",
}


def sh(*cmd, cwd, check=True):
    return subprocess.run(cmd, cwd=cwd, check=check, capture_output=True, text=True)


def make_repo(path, arm, setting):
    shutil.copytree(FIXTURE, path)
    if setting == "big" and arm == "notes":
        (path / "NOTES.md").write_text("".join(f"- {f}\n" for f in EARLIER))
    # Claude Code's own memory would carry the fact between A and B in every arm.
    (path / ".claude").mkdir()
    (path / ".claude" / "settings.json").write_text(json.dumps({"autoMemoryEnabled": False}))
    if arm in CLAUDE_MD:
        (path / "CLAUDE.md").write_text(CLAUDE_MD[arm])
    sh("git", "init", "-q", "-b", "main", cwd=path)
    sh("git", "add", "-A", cwd=path)
    sh("git", "-c", "user.name=bench", "-c", "user.email=bench@memro.co", "commit", "-qm", "fixture", cwd=path)


def run_agent(prompt, cwd, mcp_config=None):
    """One headless Claude Code turn: cost, time, the tools it called, its last message."""
    cmd = ["claude", "-p", prompt, "--model", "haiku", "--output-format", "stream-json", "--verbose",
           "--permission-mode", "acceptEdits", "--setting-sources", "project,local", "--strict-mcp-config",
           "--allowedTools", "Bash(node --test*)"]
    if mcp_config:
        cmd += ["mcp__memro__recall", "mcp__memro__remember", "--mcp-config", str(mcp_config)]
    t0 = time.time()
    p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=900)
    out = {"seconds": round(time.time() - t0, 1), "exit": p.returncode, "tools": [], "cost_usd": None, "result": ""}
    for line in p.stdout.splitlines():
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        if ev.get("type") == "assistant":
            for block in ev.get("message", {}).get("content", []):
                if block.get("type") == "tool_use":
                    out["tools"].append({"name": block["name"], "input": json.dumps(block.get("input", {}))[:200]})
        elif ev.get("type") == "result":
            out.update(cost_usd=ev.get("total_cost_usd"), result=(ev.get("result") or "")[-400:])
    if not out["result"]:
        out["result"] = p.stderr[-400:]
    return out


def accept(repo, trap):
    """Which of the trap's hidden tests pass in this checkout: a, func, trap."""
    shutil.copytree(HERE / "accept" / trap, repo / "accept-tests")
    passed = {f.stem.split(".")[0]: sh("node", "--test", f"accept-tests/{f.name}", cwd=repo, check=False).returncode == 0
              for f in sorted((repo / "accept-tests").glob("*.test.js"))}
    shutil.rmtree(repo / "accept-tests")
    return passed


def retry(fn, tries=4):
    """A Memro API call, retried: one slow request (pilot v2 hit a 30 s timeout) shouldn't end the run."""
    for i in range(tries):
        try:
            return fn()
        except Exception:
            if i == tries - 1:
                raise
            time.sleep(10)


class BenchMemory:
    """One throwaway Memro agent for the memro arm, wiped before every relay."""

    def __init__(self, workdir, setting):
        from memro import Agent, MemroClient
        self.agent = Agent.create(client=MemroClient(MEMRO_API))
        # Earlier facts stay for the whole run; wipe() removes only what a relay added.
        self.earlier = set(retry(lambda: self.agent.remember_batch([{"content": f} for f in EARLIER]))) if setting == "big" else set()
        creds = self.agent.to_dict()
        self.config = workdir / "mcp.json"
        self.config.write_text(json.dumps({"mcpServers": {"memro": {"command": str(MEMRO_MCP), "env": {
            "MEMRO_BASE_URL": MEMRO_API, "MEMRO_AGENT_ID": creds["agent_id"], "MEMRO_PRIVATE_KEY": creds["private_key"]}}}}))

    def added(self):
        return [m for m in retry(self.agent.export).memories if m.id not in self.earlier]

    def texts(self):
        return [m.content for m in self.added()]

    def wipe(self):
        for m in self.added():
            retry(lambda: self.agent.delete_memory(m.id))
        assert not self.added(), "bench memory not empty"

    def close(self):
        self.config.unlink(missing_ok=True)
        retry(self.agent.delete_agent)


def one_relay(trap, arm, workdir, memory, setting="same"):
    t = TRAPS[trap]
    repo = workdir / f"{trap}-{arm}-{uuid.uuid4().hex[:8]}"
    make_repo(repo, arm, setting)
    mcp = None
    if arm == "memro":
        memory.wipe()
        mcp = memory.config
    a = run_agent(PROMPT_A.format(task=t["a"], fact=f" Note from the user: {t['fact']}" if t["fact"] else ""), repo, mcp)
    a_ok = accept(repo, trap).get("a", False)
    kept = (repo / "NOTES.md").read_text() if arm == "notes" and (repo / "NOTES.md").exists() else ""
    kept = "\n".join(l for l in kept.splitlines() if l[2:] not in EARLIER) if setting == "big" else kept
    if arm == "memro":
        kept = "\n".join(memory.texts())
    recorded = bool(t["keyword"] and re.search(t["keyword"], kept, re.I))
    if setting == "clone":
        # A's work lands on main, notes file and all; B is a teammate who clones it.
        sh("git", "add", "-A", cwd=repo)
        sh("git", "-c", "user.name=bench", "-c", "user.email=bench@memro.co", "commit", "-qm", "A's work", "--allow-empty", cwd=repo)
        sh("git", "clone", "-q", str(repo), f"{repo}-b", cwd=workdir)
        repo = Path(f"{repo}-b")
    b = run_agent(PROMPT_B.format(task=t["b"]), repo, mcp)
    passed = accept(repo, trap)
    looked = any(x["name"] == "mcp__memro__recall" or "NOTES.md" in x["input"] for x in b["tools"])
    return {
        "trap": trap, "arm": arm, "setting": setting, "repo": str(repo), "a_ok": a_ok, "recorded": recorded, "kept": kept[-600:],
        "b_looked": looked, "b_func": passed.get("func", False),
        # The control has no trap: its "avoided" is just the task.
        "trap_avoided": passed.get("trap", passed.get("func", False)),
        "a": a, "b": b, "cost_usd": round((a["cost_usd"] or 0) + (b["cost_usd"] or 0), 4),
    }


def report(path):
    rows = [json.loads(l) for l in open(path)]
    pct = lambda xs: f"{sum(xs)}/{len(xs)}"
    print(f"{'trap':5} {'arm':6} {'trap avoided':>13} {'B task':>7} {'A recorded':>11} {'B looked':>9} {'B secs':>7}")
    for trap in TRAPS:
        for arm in ("none", "notes", "memro"):
            rs = [r for r in rows if r["trap"] == trap and r["arm"] == arm]
            if not rs:
                continue
            secs = sorted(r["b"]["seconds"] for r in rs)[len(rs) // 2]
            print(f"{trap:5} {arm:6} {pct([r['trap_avoided'] for r in rs]):>13} {pct([r['b_func'] for r in rs]):>7} "
                  f"{pct([r['recorded'] for r in rs]) if TRAPS[trap]['keyword'] else '-':>11} {pct([r['b_looked'] for r in rs]):>9} {secs:>7}")
    for arm in ("none", "notes", "memro"):
        rs = [r for r in rows if r["arm"] == arm and TRAPS[r["trap"]]["keyword"]]
        if rs:
            print(f"R1-R3 {arm:6} trap avoided {pct([r['trap_avoided'] for r in rs])}  cost ${sum(r['cost_usd'] for r in rs):.2f}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--arms", default="none,notes,memro")
    ap.add_argument("--traps", default=",".join(TRAPS))
    ap.add_argument("--runs", type=int, default=5)
    ap.add_argument("--setting", choices=["same", "clone", "big"], default="same")
    ap.add_argument("--out", default=str(ROOT / "bench" / "runs" / "relay" / time.strftime("%Y%m%d-%H%M%S")))
    ap.add_argument("--report", metavar="RESULTS_JSONL")
    args = ap.parse_args()
    if args.report:
        return report(args.report)
    out = Path(args.out).resolve()
    out.mkdir(parents=True, exist_ok=True)
    # Outside any project: Claude Code reads CLAUDE.md files from parent folders.
    workdir = Path(tempfile.mkdtemp(prefix="memro-relay-")).resolve()
    arms = args.arms.split(",")
    memory = BenchMemory(workdir, args.setting) if "memro" in arms else None
    # A stopped run still deletes its throwaway agent (the finally below).
    signal.signal(signal.SIGTERM, lambda *_: exit(143))
    try:
        with open(out / "results.jsonl", "a") as f:
            for i in range(args.runs):
                for trap in args.traps.split(","):
                    for arm in arms:  # interleaved, so a slow model hour doesn't land on one arm
                        r = one_relay(trap, arm, workdir, memory, args.setting)
                        f.write(json.dumps(r) + "\n")
                        f.flush()
                        print(f"run {i + 1} {trap} {arm:6} A {'ok' if r['a_ok'] else 'FAIL'}  recorded {r['recorded']}  "
                              f"B looked {r['b_looked']}  B task {r['b_func']}  trap avoided {r['trap_avoided']}  ${r['cost_usd']}", flush=True)
    finally:
        if memory:
            memory.close()
    report(out / "results.jsonl")
    print(f"results: {out / 'results.jsonl'}  repos: {workdir}")


if __name__ == "__main__":
    main()
