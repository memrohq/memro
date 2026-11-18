"""Relay benchmark: does an agent that picks up another's work avoid its known
mistakes? Agent A gets a task plus a fact from the user that the code doesn't
show; a fresh agent B then gets a related task whose obvious solution breaks
that fact. Hidden tests check B's work. Design: bench/relay/README.md.

Arms differ only in how A's knowledge can reach B:
  none   nothing: B starts fresh
  notes  both are told to keep and read NOTES.md (the free baseline)
  memro  both have Memro's MCP tools, told to recall and remember
  memro-hooks  the same plus Memro's hooks (memro connect's default): they
         save rules the user states, show the repo's rules at every prompt,
         and show a file's memories when it's first edited
  memro-plain  as memro-hooks, but with --plant the rule is a plain memory,
         not one the rule hook saved, so it has to be found by search
  automem  Claude Code's own auto-memory, switched on; with --plant it holds
         the earlier facts as memory files plus a MEMORY.md index (Claude
         Code loads the index's first 200 lines)

Settings (--setting), where B works and what it has to search:
  same   B in A's checkout (stage 1)
  clone  B is a teammate in a fresh clone; A's work is committed first,
         NOTES.md included, so the notes file follows (its strongest form)
  big    as same, but NOTES.md and Memro already hold 300 earlier facts on
         other topics, a dozen of them near the trap's subject
  huge   as big with 3,000 earlier facts, 100 of them earlier rules for this
         repo: more than an agent reads whole (Read shows 2,000 lines)

Claude Code's own auto-memory is switched off in every arm but automem, so it
can't carry the fact on its own. Each memro arm uses its own throwaway Memro
agent (not tied to any account), wiped before every relay and deleted at the end.

A run stops when an agent fails (a usage limit, a crash) without recording
that relay; the same command with the same --out picks up where it stopped.

Run with the MCP server's Python, which has the Memro SDK:

  mcp/.venv/bin/python bench/relay/run.py --runs 5
  mcp/.venv/bin/python bench/relay/run.py --report bench/runs/relay/<run>/results.jsonl
"""

import argparse
import json
import math
import os
import random
import re
import shutil
import signal
import subprocess
import tempfile
import time
import uuid
from collections import Counter
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
FIXTURE = HERE.parent / "coordination" / "fixture"
MEMRO_MCP = ROOT / "mcp" / ".venv" / "bin" / "memro-mcp"
MEMRO_API = os.environ.get("MEMRO_API_URL", "https://api.memro.co")  # a local server for runs past the 1,000-memory cap
# The CLI whose hooks the memro-hooks arm installs (MEMRO_CLI: another checkout's cli/).
MEMRO_CLI = Path(os.environ.get("MEMRO_CLI", ROOT / "cli")).resolve()

# A's own task never touches what the fact is about, so the fact leaves no
# trace in the code (pilot v1: A wrote it into comments and B copied them).
TRAPS = {
    "R1": dict(  # a project rule mentioned in passing
        a="Add itemCount(cart) to src/cart.js: the sum of qty over all items. Export it from src/index.js.",
        fact="By the way, a rule for this whole repo: never use Intl or toLocaleString anywhere. Our POS devices run a Node build without ICU, and it crashed in production last month.",
        b="Add formatMoney(cents, locale, currency) to src/format.js and export it from src/index.js. It must handle two cases: "
          "formatMoney(123456, \"en-US\", \"USD\") is \"$1,234.56\" and formatMoney(123456, \"de-DE\", \"EUR\") is \"1.234,56 €\".",
        keyword=r"\bintl\b|\bicu\b|tolocalestring",
        # --plant: the rule as A would leave it, and B's task with and without the rule's words.
        rule="Never use Intl or toLocaleString anywhere in this repo: the POS devices run a Node build without ICU, and it crashed in production.",
        seen=r"\bicu\b",
        direct="Add formatMoney(cents, locale, currency) to src/format.js and export it from src/index.js. The result should match what "
               "toLocaleString or Intl.NumberFormat gives for that locale: formatMoney(123456, \"en-US\", \"USD\") is \"$1,234.56\" and "
               "formatMoney(123456, \"de-DE\", \"EUR\") is \"1.234,56 €\".",
        indirect="Add formatMoney(cents, lang, currency) to src/format.js and export it from src/index.js. It must handle two cases: "
                 "formatMoney(123456, \"en-US\", \"USD\") is \"$1,234.56\" and formatMoney(123456, \"de-DE\", \"EUR\") is \"1.234,56 €\".",
    ),
    "R2": dict(  # a decision made before any code exists
        a="Add removeItem(cart, id) to src/cart.js: a new cart without the item with that id, without mutating the input. Export it from src/index.js.",
        fact="Heads-up for later: when we add coupons or any other discount, the discounted total must always round down to a whole cent. Rounding up overcharged customers last quarter and finance had to refund them.",
        b="Add couponTotal(cart, percent) to src/cart.js: the cart total in cents with percent% off. Export it from src/index.js.",
        keyword=r"round(s|ing|ed)? down|math\.floor|\bfloor\b",
        rule="When we add coupons or any other discount, the discounted total must always round down to a whole cent: rounding up overcharged customers.",
        seen=r"round(s|ing|ed)? down",
        direct="Add couponTotal(cart, percent) to src/cart.js: the cart total in cents with a coupon discount of percent%. Export it from src/index.js.",
        # No coupon, discount or rounding words; the hidden tests accept saleTotal too.
        indirect="Add saleTotal(cart, percent) to src/cart.js: what the customer pays, in cents, during a sale of percent% off. Export it from src/index.js.",
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

# The huge setting: what a project gathers over a year. 100 earlier rules for
# this repo (none about the traps) and 2,885 other facts, shuffled together.
MORE_AREAS = AREAS + ["billing export", "tax reports", "gift cards", "returns portal", "shipping labels", "store locator",
                      "push notifications", "SMS gateway", "admin audit log", "session store", "SSO login", "invoice PDFs",
                      "price import", "catalog sync", "wishlist", "reviews moderation", "fraud checks", "loyalty program",
                      "referral codes", "newsletter signup", "cookie banner", "status page", "on-call rota", "staging data",
                      "seed scripts", "Docker images", "Terraform modules", "DNS records", "TLS certificates", "log retention",
                      "metrics dashboards", "alert rules", "queue workers", "cron jobs", "search index"]
MORE_FACTS = ["{a}: was rewritten in 2025 and the old version is archived.", "{a}: has its own README with setup steps.",
              "{a}: is tested in CI on every pull request.", "{a}: sends its errors to the #alerts channel.",
              "{a}: uses the shared HTTP client with tracing headers.", "{a}: keeps its config in config/{a}.yaml.",
              "{a}: has a runbook in the ops wiki.", "{a}: is owned by the commerce team.",
              "{a}: was load tested at 500 requests per second.", "{a}: depends on Postgres 16.",
              "{a}: writes audit entries for every admin change.", "{a}: has a staging copy refreshed weekly.",
              "{a}: uses UUIDs, not integer ids.", "{a}: paginates with cursors, not offsets.",
              "{a}: caches lookups for five minutes.", "{a}: was migrated off the old queue in March.",
              "{a}: has a kill switch in the admin panel.", "{a}: is documented in the API reference.",
              "{a}: logs in JSON lines.", "{a}: runs in two regions.", "{a}: was last reviewed for security in October.",
              "{a}: uses the design system's components.", "{a}: has end-to-end tests in the e2e folder.",
              "{a}: reads feature flags at startup.", "{a}: is behind the API gateway.", "{a}: has a p95 latency budget of 300 ms.",
              "{a}: exports metrics with the service name as a label.", "{a}: was built by a contractor and later taken over.",
              "{a}: has a nightly consistency check.", "{a}: stores files in the shared bucket under its own prefix.",
              "{a}: sends webhooks signed with HMAC.", "{a}: uses semantic versioning for its client.",
              "{a}: has rate limits per account.", "{a}: retries webhooks for 24 hours.", "{a}: has dashboards in Grafana.",
              "{a}: is part of the quarterly disaster recovery drill.", "{a}: uses the company's ESLint config.",
              "{a}: has a changelog kept by hand.", "{a}: emits events to the analytics pipeline.",
              "{a}: has a maintenance window on Sunday mornings.", "{a}: accepts only TLS 1.2 or newer.",
              "{a}: has its API documented with OpenAPI.", "{a}: is monitored by synthetic checks every minute.",
              "{a}: uses idempotency keys on writes.", "{a}: was split out of the monolith last year.",
              "{a}: reads secrets at startup only.", "{a}: has a feature freeze before Black Friday.",
              "{a}: has an on-call owner listed in the service catalog.", "{a}: publishes a weekly usage report."]
RULE_FACTS = ["Never call the {a} directly from the browser; go through the gateway.",
              "Always add a migration note when changing the {a}.",
              "Never log personal data in the {a}.",
              "Always ship changes to the {a} behind a feature flag first."]
REPO_RULES = [r.format(a=a) for r in RULE_FACTS for a in AREAS]  # 100
HUGE = NEAR + [f.format(a=a) for f in MORE_FACTS for a in MORE_AREAS][:2900 - len(NEAR)]
BENCH_REPO = "shop"  # every relay repo's origin name, so earlier rules are this repo's rules


ARMS = ("none", "notes", "automem", "memro", "memro-hooks", "memro-plain")
PLANT = {"start": 0, "25": .25, "50": .5, "75": .75, "end": 1}


def plant_rule(lines, rules, at):
    """lines with the traps' rules that far down (in the huge setting 75% and end are past Read's first 2,000 lines)."""
    i = round(len(lines) * PLANT[at])
    return lines[:i] + list(rules) + lines[i:]


def auto_memory_dir(repo):
    """Where Claude Code keeps a project's auto-memory: its path with / and . as -."""
    return Path.home() / ".claude" / "projects" / re.sub(r"[/.]", "-", str(repo.resolve())) / "memory"


def earlier_notes(setting):
    """NOTES.md lines and Memro memories already there when A starts."""
    if setting == "big":
        return EARLIER, [{"content": f} for f in EARLIER]
    if setting == "huge":
        lines = HUGE + REPO_RULES
        random.Random(0).shuffle(lines)
        memories = [{"content": f} for f in HUGE] + [
            {"content": f"Rule for {BENCH_REPO}: {r}", "type": "procedural", "metadata": {"source": "memro-hook", "repo": BENCH_REPO}}
            for r in REPO_RULES]
        return lines, memories
    return [], []

TAIL = " Keep the existing behaviour and tests passing (run `node --test`). Do not edit or add test files. Do not commit."
PROMPT_A = "You are working in a small JavaScript repo. Task: {task}{fact}" + TAIL
PROMPT_B = "You are working in a small JavaScript repo. Task: {task}" + TAIL

# Word for word the same in both arms but for where it's kept.
RECORD = "When you learn something a future agent working here would need (a decision and why, a constraint, something that failed), "
CLAUDE_MD = {
    "notes": "## Project notes\n\nBefore you start, read NOTES.md if it exists. " + RECORD + "add one short line to NOTES.md.\n",
    "memro-hooks": "## Memro (long-term memory)\n\nBefore you start, call the memro `recall` tool with a short query about your task. " + RECORD + "save one short sentence with the memro `remember` tool.\n",
    "memro": "## Memro (long-term memory)\n\nBefore you start, call the memro `recall` tool with a short query about your task. " + RECORD + "save one short sentence with the memro `remember` tool.\n",
    "automem": "## Project memory\n\nBefore you start, check your auto memory for anything about your task. " + RECORD + "save it to your auto memory.\n",
}
CLAUDE_MD["memro-plain"] = CLAUDE_MD["memro-hooks"]


def sh(*cmd, cwd, check=True, env=None):
    return subprocess.run(cmd, cwd=cwd, check=check, capture_output=True, text=True, env=env)


def make_repo(path, arm, setting, rules=None, at=None):
    shutil.copytree(FIXTURE, path)
    if (setting in ("big", "huge") or rules) and arm == "notes":
        lines = earlier_notes(setting)[0]
        (path / "NOTES.md").write_text("".join(f"- {f}\n" for f in (plant_rule(lines, rules, at) if rules else lines)))
    # Claude Code's own memory would carry the fact between A and B in every arm.
    (path / ".claude").mkdir()
    (path / ".claude" / "settings.json").write_text(json.dumps({"autoMemoryEnabled": arm == "automem"}))
    if arm == "automem" and (setting in ("big", "huge") or rules):
        # One file per memory and a one-line index, as Claude Code writes them.
        lines = earlier_notes(setting)[0]
        lines = plant_rule(lines, rules, at) if rules else lines
        mem = auto_memory_dir(path)
        mem.mkdir(parents=True)
        for i, f in enumerate(lines):
            (mem / f"fact-{i}.md").write_text(f"---\nname: fact-{i}\ndescription: {f}\nmetadata:\n  type: project\n---\n\n{f}\n")
        (mem / "MEMORY.md").write_text("".join(f"- [fact-{i}](fact-{i}.md) — {f}\n" for i, f in enumerate(lines)))
    if arm in CLAUDE_MD:
        (path / "CLAUDE.md").write_text(CLAUDE_MD[arm])
    if arm in ("memro-hooks", "memro-plain"):
        # Installed under a throwaway HOME: ~/.memro/hooks/ is the user's own hook.
        home = path.parent / "hook-home"
        sh("node", "-e", f'import("{MEMRO_CLI}/src/clients.js").then(m => m.installHooks({{scope: "project", cwd: "{path}"}}))',
           cwd=MEMRO_CLI, env={**os.environ, "HOME": str(home)})
    sh("git", "init", "-q", "-b", "main", cwd=path)
    # Memro names a repo by its origin, so every relay is the same repo to the hooks.
    sh("git", "remote", "add", "origin", f"git@bench.memro.co:bench/{BENCH_REPO}.git", cwd=path)
    sh("git", "add", "-A", cwd=path)
    sh("git", "-c", "user.name=bench", "-c", "user.email=bench@memro.co", "commit", "-qm", "fixture", cwd=path)


def run_agent(prompt, cwd, mcp_config=None, env=None):
    """One headless Claude Code turn: cost, time, the tools it called, its last message."""
    cmd = ["claude", "-p", prompt, "--model", "haiku", "--output-format", "stream-json", "--verbose",
           "--permission-mode", "acceptEdits", "--setting-sources", "project,local", "--strict-mcp-config",
           "--include-hook-events", "--allowedTools", "Bash(node --test*)"]
    if mcp_config:
        cmd += ["mcp__memro__recall", "mcp__memro__remember", "--mcp-config", str(mcp_config)]
    t0 = time.time()
    # No stdin, or claude -p waits 3 s for piped input.
    p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=900, env=env, stdin=subprocess.DEVNULL)
    out = {"seconds": round(time.time() - t0, 1), "exit": p.returncode, "tools": [], "cost_usd": None, "result": "", "shown": ""}
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
        else:
            out["shown"] += line + "\n"  # what reached the agent: tool results and hook output
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
    """One throwaway Memro agent for a memro arm, wiped before every relay."""

    def __init__(self, workdir, setting, arm, rules=None, at=None):
        from memro import Agent, MemroClient
        self.agent = Agent.create(client=MemroClient(MEMRO_API))
        # Earlier facts stay for the whole run; wipe() removes only what a relay added.
        memories = earlier_notes(setting)[1]
        if rules:
            # Saved among the earlier facts at the same depth as in NOTES.md, so it's no newer than they are;
            # memro-plain stores it as a plain memory, the others as the rule hook would save it.
            memories = plant_rule(memories, [{"content": r} if arm == "memro-plain" else {
                "content": f"Rule for {BENCH_REPO}: {r}", "type": "procedural",
                "metadata": {"source": "memro-hook", "repo": BENCH_REPO}} for r in rules], at)
        # The batch endpoint takes up to 500 at a time.
        self.earlier = {i for k in range(0, len(memories), 500) for i in retry(lambda: self.agent.remember_batch(memories[k:k + 500]))}
        creds = self.agent.to_dict()
        # The hooks sign in with an API key, which a signed agent can make for itself.
        self.api_key = self.agent.client.request("POST", "/api-keys", json_body={"name": "relay bench"},
                                                 agent_id=creds["agent_id"], private_key=creds["private_key"]).json()["key"]
        self.env = {**os.environ, "MEMRO_API_KEY": self.api_key, "MEMRO_API_URL": MEMRO_API}
        self.config = workdir / f"mcp-{arm}.json"
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


def one_relay(trap, arm, workdir, memory, setting="same", plant=None, phrasing=None, rules=()):
    t = TRAPS[trap]
    repo = workdir / f"{trap}-{arm}-{uuid.uuid4().hex[:8]}"
    # Every trap's rule is planted in every relay, so all arms hold the same facts.
    rule = list(rules) if plant and arm != "none" else None
    make_repo(repo, arm, setting, rule, plant)
    mcp = None
    env = None
    if arm.startswith("memro"):
        memory.wipe()  # the planted rule, if any, is one of its earlier memories
        mcp = memory.config
        env = memory.env if arm in ("memro-hooks", "memro-plain") else None
    if plant:
        b = run_agent(PROMPT_B.format(task=t[phrasing]), repo, mcp, env)
        passed = accept(repo, trap)
        shutil.rmtree(auto_memory_dir(repo).parent, ignore_errors=True)
        return {
            "trap": trap, "arm": arm, "setting": setting, "plant": plant, "phrasing": phrasing, "repo": str(repo),
            "a_ok": True, "recorded": bool(rule), "kept": "",
            "b_looked": any(x["name"] == "mcp__memro__recall" or "NOTES.md" in x["input"] or ".claude/projects" in x["input"]
                            for x in b["tools"]),
            # The rule's own words (not in either task) came back in a tool result or hook output.
            "b_saw_rule": bool(re.search(t["seen"], b.pop("shown"), re.I)),
            "b_func": passed.get("func", False), "trap_avoided": passed.get("trap", False),
            "a": None, "b": b, "cost_usd": round(b["cost_usd"] or 0, 4),
        }
    a = run_agent(PROMPT_A.format(task=t["a"], fact=f" Note from the user: {t['fact']}" if t["fact"] else ""), repo, mcp, env)
    a.pop("shown")
    a_ok = accept(repo, trap).get("a", False)
    kept = (repo / "NOTES.md").read_text() if arm == "notes" and (repo / "NOTES.md").exists() else ""
    known = set(earlier_notes(setting)[0])
    kept = "\n".join(l for l in kept.splitlines() if l[2:] not in known)
    if arm.startswith("memro"):
        kept = "\n".join(memory.texts())
    recorded = bool(t["keyword"] and re.search(t["keyword"], kept, re.I))
    if setting == "clone":
        # A's work lands on main, notes file and all; B is a teammate who clones it.
        sh("git", "add", "-A", cwd=repo)
        sh("git", "-c", "user.name=bench", "-c", "user.email=bench@memro.co", "commit", "-qm", "A's work", "--allow-empty", cwd=repo)
        sh("git", "clone", "-q", str(repo), f"{repo}-b", cwd=workdir)
        repo = Path(f"{repo}-b")
        sh("git", "remote", "set-url", "origin", f"git@bench.memro.co:bench/{BENCH_REPO}.git", cwd=repo)
    b = run_agent(PROMPT_B.format(task=t["b"]), repo, mcp, env)
    b.pop("shown")
    passed = accept(repo, trap)
    shutil.rmtree(auto_memory_dir(repo).parent, ignore_errors=True)
    looked = any(x["name"] == "mcp__memro__recall" or "NOTES.md" in x["input"] or ".claude/projects" in x["input"] for x in b["tools"])
    return {
        "trap": trap, "arm": arm, "setting": setting, "repo": str(repo), "a_ok": a_ok, "recorded": recorded, "kept": kept[-600:],
        "b_looked": looked, "b_func": passed.get("func", False),
        # The control has no trap: its "avoided" is just the task.
        "trap_avoided": passed.get("trap", passed.get("func", False)),
        "a": a, "b": b, "cost_usd": round((a["cost_usd"] or 0) + (b["cost_usd"] or 0), 4),
    }


def wilson(k, n, z=1.96):
    c, h = (k + z * z / 2) / (n + z * z), z * (k * (n - k) / n + z * z / 4) ** 0.5 / (n + z * z)
    return max(0, c - h), min(1, c + h)


def fisher(k1, n1, k2, n2):
    """Two-sided Fisher exact p for k1/n1 against k2/n2."""
    K, N = k1 + k2, n1 + n2
    p = lambda k: math.comb(n1, k) * math.comb(n2, K - k) / math.comb(N, K)
    return min(1.0, sum(p(k) for k in range(max(0, K - n2), min(n1, K) + 1) if p(k) <= p(k1) * (1 + 1e-9)))


assert fisher(9, 10, 10, 10) == 1.0 and 0.001 < fisher(40, 50, 50, 50) < 0.002


def report(paths):
    rows = [json.loads(l) for p in paths for l in open(p)]
    pct = lambda xs: f"{sum(xs)}/{len(xs)}"
    print(f"{'trap':5} {'arm':6} {'trap avoided':>13} {'B task':>7} {'A recorded':>11} {'B looked':>9} {'B secs':>7} {'B saw rule':>10}")
    for trap in TRAPS:
        for arm in ARMS:
            rs = [r for r in rows if r["trap"] == trap and r["arm"] == arm]
            if not rs:
                continue
            secs = sorted(r["b"]["seconds"] for r in rs)[len(rs) // 2]
            print(f"{trap:5} {arm:11} {pct([r['trap_avoided'] for r in rs]):>13} {pct([r['b_func'] for r in rs]):>7} "
                  f"{pct([r['recorded'] for r in rs]) if TRAPS[trap]['keyword'] else '-':>11} {pct([r['b_looked'] for r in rs]):>9} {secs:>7} "
                  f"{pct([r['b_saw_rule'] for r in rs]) if 'b_saw_rule' in rs[0] else '-':>10}")
    # Per arm and rule position, with Fisher's p against the notes file at the same position.
    for plant in sorted({r.get("plant") for r in rows}, key=lambda p: PLANT.get(p, -1)):
        cell = lambda arm: [r["trap_avoided"] for r in rows if r["arm"] == arm and r.get("plant") == plant and TRAPS[r["trap"]]["keyword"]]
        notes = cell("notes")
        for arm in ARMS:
            xs = cell(arm)
            if xs:
                lo, hi = wilson(sum(xs), len(xs))
                vs = f"  vs notes p={fisher(sum(xs), len(xs), sum(notes), len(notes)):.3g}" if notes and arm != "notes" else ""
                print(f"R1-R3 rule at {plant or '-':5} {arm:11} trap avoided {pct(xs)} (95% CI {lo:.0%}-{hi:.0%}){vs}")
    print(f"cost ${sum(r['cost_usd'] for r in rows):.2f}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--arms", default="none,notes,memro")
    ap.add_argument("--traps", default=",".join(TRAPS))
    ap.add_argument("--runs", type=int, default=5)
    ap.add_argument("--setting", choices=["same", "clone", "big", "huge"], default="same")
    ap.add_argument("--out", default=str(ROOT / "bench" / "runs" / "relay" / time.strftime("%Y%m%d-%H%M%S")))
    ap.add_argument("--report", metavar="RESULTS_JSONL", nargs="+")
    ap.add_argument("--plant", choices=list(PLANT),
                    help="skip agent A: the rule is already this far down NOTES.md, the auto-memory index and the "
                         "Memro agent's earlier memories, and only B runs")
    ap.add_argument("--phrasing", choices=["direct", "indirect"], default="indirect",
                    help="with --plant: whether B's task repeats the rule's words")
    args = ap.parse_args()
    if args.plant:
        assert all("rule" in TRAPS[t] for t in args.traps.split(",")), "--plant needs traps with a rule (R1, R2)"
    if args.report:
        return report(args.report)
    out = Path(args.out).resolve()
    out.mkdir(parents=True, exist_ok=True)
    # Outside any project: Claude Code reads CLAUDE.md files from parent folders.
    workdir = Path(tempfile.mkdtemp(prefix="memro-relay-")).resolve()
    arms = args.arms.split(",")
    assert set(arms) <= set(ARMS), f"arms are {ARMS}"
    results = out / "results.jsonl"
    # Resume: relays already recorded in this --out count toward --runs.
    done = Counter((r["trap"], r["arm"]) for r in map(json.loads, open(results))) if results.exists() else Counter()
    # A stopped run still deletes its throwaway agents (the finally below).
    signal.signal(signal.SIGTERM, lambda *_: exit(143))
    memories = {}
    rules = [TRAPS[t]["rule"] for t in args.traps.split(",")] if args.plant else []
    try:
        for arm in arms:
            if arm.startswith("memro"):
                memories[arm] = BenchMemory(workdir, args.setting, arm, rules, args.plant)
        with open(results, "a") as f:
            for i in range(args.runs):
                for trap in args.traps.split(","):
                    for arm in arms:  # interleaved, so a slow model hour doesn't land on one arm
                        if done[(trap, arm)] > i:
                            continue
                        r = one_relay(trap, arm, workdir, memories.get(arm), args.setting, args.plant, args.phrasing, rules)
                        if r["b"]["exit"] or (r["a"] and r["a"]["exit"]):
                            # A usage limit or a crash isn't a result: stop, so a rerun with this --out redoes it.
                            raise SystemExit(f"agent failed, stopping (rerun to resume): {r['b']['result'][-300:]}")
                        f.write(json.dumps(r) + "\n")
                        f.flush()
                        print(f"run {i + 1} {trap} {arm:11} A {'ok' if r['a_ok'] else 'FAIL'}  recorded {r['recorded']}  "
                              f"B looked {r['b_looked']}  B task {r['b_func']}  trap avoided {r['trap_avoided']}  ${r['cost_usd']}", flush=True)
    finally:
        for m in memories.values():
            m.close()
    report([results])
    print(f"results: {out / 'results.jsonl'}  repos: {workdir}")


if __name__ == "__main__":
    main()
