"""Summarise a run: QA accuracy per type, retrieval recall (R@5/R@10/R@15),
failure evidence breakdown, duplicate slots, abstention, latency and cost.

    python report.py runs/full-continuity
"""
import json
import pathlib
import statistics
import sys

from common import LEDGER

NO_INFO_PHRASES = [
    "don't know", "do not know", "no information", "not provided", "not mentioned",
    "does not mention", "does not contain", "cannot be determined", "no record",
    "don't have any information", "not specify", "not specified", "unknown"
]

run = pathlib.Path(sys.argv[1])
rows = {json.loads(l)["question_id"]: json.loads(l) for l in (run / "hyp.jsonl").read_text().splitlines() if l.strip()}
meta_file = run / "meta.json"
meta = json.loads(meta_file.read_text()) if meta_file.exists() else {}


def labels(model):
    f = run / f"eval-{model}.jsonl"
    return {json.loads(l)["question_id"]: json.loads(l)["label"] for l in f.read_text().splitlines()} if f.exists() else {}


mini, big = labels("gpt-4o-mini"), labels("gpt-4o")
types = sorted({r["question_type"] for r in rows.values()})


def recall(r, k):
    """Any evidence session in the top k (abstention questions have none and are skipped)."""
    if not r["answer_session_ids"]:
        return None
    return any(s in r["answer_session_ids"] for s in r["retrieved_sessions"][:k])


def pct(xs):
    xs = [x for x in xs if x is not None]
    return f"{100 * sum(xs) / len(xs):5.1f}% ({sum(xs)}/{len(xs)})" if xs else "    -"


lines = [
    f"# LongMemEval_S on Memro: {run.name}", "",
    f"Questions answered: {len(rows)}  ·  graded (gpt-4o-mini): {len(mini)}",
]
if meta.get("commit"):
    lines.append(f"Server commit: `{meta['commit']}`")
if meta.get("engine_options"):
    lines.append(f"Engine options: `{json.dumps(meta['engine_options'])}`")

lines.extend([
    "",
    "| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |",
    "|---|---|---|---|---|---|---|",
])

for t in types + ["overall"]:
    ids = [i for i, r in rows.items() if t == "overall" or r["question_type"] == t]
    fails = [i for i in ids if i in mini and not mini[i]]
    in_ctx = sum(1 for i in fails if rows[i]["answer_session_ids"] and set(rows[i]["answer_session_ids"]).issubset(set(rows[i]["retrieved_sessions"][:15])))
    missed = len(fails) - in_ctx
    fail_str = f"{len(fails)} ({in_ctx}/{missed})" if fails else "0 (0/0)"

    lines.append(
        f"| {t} | {len(ids)} | {pct([mini.get(i) for i in ids if i in mini])} | "
        f"{pct([recall(rows[i], 5) for i in ids])} | {pct([recall(rows[i], 10) for i in ids])} | "
        f"{pct([recall(rows[i], 15) for i in ids])} | {fail_str} |"
    )

if big:
    both = [i for i in big if i in mini]
    agree = sum(mini[i] == big[i] for i in both)
    lines += ["", f"Judge cross-check: gpt-4o agrees with gpt-4o-mini on {agree}/{len(both)} sampled answers; "
              f"gpt-4o accuracy on the sample {pct([big[i] for i in both])}."]

# Synthesis and quality metrics
dups = [len(r["retrieved_sessions"][:15]) - len(set(r["retrieved_sessions"][:15])) for r in rows.values()]
avg_dups = sum(dups) / len(dups) if dups else 0.0

abs_ids = [i for i, r in rows.items() if ("_abs" in i or not r["answer_session_ids"]) and i in mini]
abs_correct = sum(1 for i in abs_ids if mini.get(i))
abs_str = f"{100 * abs_correct / len(abs_ids):5.1f}% ({abs_correct}/{len(abs_ids)})" if abs_ids else "N/A"

no_info_in_ctx = 0
for i, r in rows.items():
    if i in mini and not mini.get(i):
        gold = set(r["answer_session_ids"])
        if gold and gold.issubset(set(r["retrieved_sessions"][:15])):
            h = r["hypothesis"].lower()
            ans_text = h.split("answer:")[-1] if "answer:" in h else h
            if any(p in ans_text for p in NO_INFO_PHRASES) or any(p in h for p in NO_INFO_PHRASES):
                no_info_in_ctx += 1

lines.extend([
    "",
    "## Retrieval & Synthesis Diagnostics",
    f"- **Average duplicate-session slots**: {avg_dups:.2f} of 15 slots",
    f"- **Abstention accuracy**: {abs_str}",
    f"- **Failed with evidence present saying 'no info' / 'don't know'**: {no_info_in_ctx}",
])

s = [r["search_ms"] for r in rows.values() if "search_ms" in r]
if s:
    s.sort()
    lines += ["", f"Search latency: p50 {statistics.median(s):.0f} ms, p95 {s[int(0.95 * (len(s) - 1))]:.0f} ms "
              f"(local Docker Postgres, {statistics.mean(r['memories'] for r in rows.values()):.0f} memories per question)."]

if meta.get("token_usage"):
    tu = meta["token_usage"]
    lines += ["", f"Run token usage & cost: ${meta.get('cost_usd', 0.0):.4f}"]
    for m, v in tu.items():
        lines.append(f"- {m}: ${v.get('usd', 0.0):.4f}, {v.get('prompt_tokens', 0):,} in / {v.get('completion_tokens', 0):,} out tokens, {v.get('calls', 0)} calls")
elif LEDGER.exists():
    led = json.loads(LEDGER.read_text())
    lines += ["", f"OpenAI spend so far (all runs ledger): ${led['total_usd']:.4f}"]
    for m, v in led["by_model"].items():
        lines.append(f"- {m}: ${v['usd']:.4f}, {v['prompt_tokens']:,} in / {v['completion_tokens']:,} out tokens, {v['calls']} calls")

text = "\n".join(lines)
(run / "REPORT.md").write_text(text + "\n")
print(text)
