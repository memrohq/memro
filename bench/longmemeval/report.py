"""Summarise a run: QA accuracy per type, retrieval recall, latency and cost.

    python report.py runs/full
"""
import json
import pathlib
import statistics
import sys

from common import LEDGER

run = pathlib.Path(sys.argv[1])
rows = {json.loads(l)["question_id"]: json.loads(l) for l in (run / "hyp.jsonl").read_text().splitlines() if l.strip()}


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


lines = [f"# LongMemEval_S on Memro: {run.name}", "",
         f"Questions answered: {len(rows)}  ·  graded (gpt-4o-mini): {len(mini)}", "",
         "| Type | n | QA accuracy | Recall@5 | Recall@10 |", "|---|---|---|---|---|"]
for t in types + ["overall"]:
    ids = [i for i, r in rows.items() if t == "overall" or r["question_type"] == t]
    lines.append(f"| {t} | {len(ids)} | {pct([mini.get(i) for i in ids if i in mini])} | "
                 f"{pct([recall(rows[i], 5) for i in ids])} | {pct([recall(rows[i], 10) for i in ids])} |")
if big:
    both = [i for i in big if i in mini]
    agree = sum(mini[i] == big[i] for i in both)
    lines += ["", f"Judge cross-check: gpt-4o agrees with gpt-4o-mini on {agree}/{len(both)} sampled answers; "
              f"gpt-4o accuracy on the sample {pct([big[i] for i in both])}."]
s = [r["search_ms"] for r in rows.values()]
if s:
    s.sort()
    lines += ["", f"Search latency: p50 {statistics.median(s):.0f} ms, p95 {s[int(0.95 * (len(s) - 1))]:.0f} ms "
              f"(local Docker Postgres, {statistics.mean(r['memories'] for r in rows.values()):.0f} memories per question)."]
if LEDGER.exists():
    led = json.loads(LEDGER.read_text())
    lines += ["", f"OpenAI spend so far (all runs): ${led['total_usd']:.4f}"]
    for m, v in led["by_model"].items():
        lines.append(f"- {m}: ${v['usd']:.4f}, {v['prompt_tokens']:,} in / {v['completion_tokens']:,} out tokens, {v['calls']} calls")
text = "\n".join(lines)
(run / "REPORT.md").write_text(text + "\n")
print(text)
