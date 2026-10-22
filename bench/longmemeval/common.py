"""Shared settings for the LongMemEval run: the OpenAI key (from ../.env, never
printed), prices, and a cost ledger that every OpenAI call goes through so the
run stops at a hard budget."""
import fcntl
import json
import os
import pathlib
import time
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent
BENCH = ROOT.parent
RUNS = BENCH / "runs"
RUNS.mkdir(exist_ok=True)
LEDGER = RUNS / "ledger.json"

# USD per 1M tokens (OpenAI list prices; input, output).
PRICES = {
    "text-embedding-3-small": (0.02, 0.0),
    "gpt-4o-mini": (0.15, 0.60),
    "gpt-4o": (2.50, 10.00),
}

BUDGET_USD = float(os.environ.get("BENCH_BUDGET_USD", "2.00"))


def openai_key() -> str:
    """Read OPENAI_API_KEY from the environment or memro-benchmark/.env."""
    key = os.environ.get("OPENAI_API_KEY")
    env = BENCH / ".env"
    if not key and env.exists():
        for line in env.read_text().splitlines():
            if line.startswith("OPENAI_API_KEY="):
                key = line.split("=", 1)[1].strip().strip('"').strip("'")
    if not key:
        raise SystemExit("OPENAI_API_KEY not found (set it or create memro-benchmark/.env)")
    return key


class BudgetExceeded(RuntimeError):
    pass


def _ledger_update(fn):
    LEDGER.touch(exist_ok=True)
    with open(LEDGER, "r+") as f:
        fcntl.flock(f, fcntl.LOCK_EX)
        text = f.read()
        data = json.loads(text) if text.strip() else {"total_usd": 0.0, "by_model": {}}
        result = fn(data)
        f.seek(0)
        f.truncate()
        json.dump(data, f, indent=2)
        return result


def spent() -> float:
    return _ledger_update(lambda d: d["total_usd"])


def check_budget(estimate_usd: float = 0.0) -> None:
    """Refuse a call that could take the run past the budget."""
    total = spent()
    if total + estimate_usd > BUDGET_USD:
        raise BudgetExceeded(f"budget ${BUDGET_USD:.2f} reached (spent ${total:.4f})")


def record(model: str, prompt_tokens: int, completion_tokens: int = 0) -> float:
    pin, pout = PRICES[model]
    cost = (prompt_tokens * pin + completion_tokens * pout) / 1e6

    def add(d):
        d["total_usd"] = d["total_usd"] + cost
        m = d["by_model"].setdefault(model, {"usd": 0.0, "prompt_tokens": 0, "completion_tokens": 0, "calls": 0})
        m["usd"] += cost
        m["prompt_tokens"] += prompt_tokens
        m["completion_tokens"] += completion_tokens
        m["calls"] += 1
    _ledger_update(add)
    return cost


def openai_post(path: str, body: dict, retries: int = 8) -> dict:
    """POST to the OpenAI API with backoff on 429/5xx. Never logs the key."""
    req_body = json.dumps(body).encode()
    delay = 2.0
    for attempt in range(retries):
        req = urllib.request.Request(
            "https://api.openai.com" + path, data=req_body, method="POST",
            headers={"Authorization": f"Bearer {openai_key()}", "Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=120) as r:
                return json.loads(r.read())
        except urllib.error.HTTPError as e:
            msg = e.read().decode(errors="replace")[:300]
            if e.code in (429, 500, 502, 503, 504) and attempt < retries - 1:
                if "insufficient_quota" in msg:
                    raise BudgetExceeded("OpenAI says the account is out of credit (insufficient_quota)")
                time.sleep(delay)
                delay = min(delay * 2, 60)
                continue
            raise RuntimeError(f"OpenAI {path} HTTP {e.code}: {msg}")
        except urllib.error.URLError:
            if attempt < retries - 1:
                time.sleep(delay)
                delay = min(delay * 2, 60)
                continue
            raise
    raise RuntimeError("unreachable")


def chat(model: str, messages: list, max_tokens: int, estimate_usd: float) -> str:
    check_budget(estimate_usd)
    r = openai_post("/v1/chat/completions", {"model": model, "messages": messages,
                                             "temperature": 0, "max_tokens": max_tokens})
    u = r.get("usage", {})
    record(model, u.get("prompt_tokens", 0), u.get("completion_tokens", 0))
    return r["choices"][0]["message"]["content"]
