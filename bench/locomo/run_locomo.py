#!/usr/bin/env python3
"""LoCoMo on Memro: held-out evaluation only.

LoCoMo (snap-research/locomo, CC BY-NC 4.0) confirms improvements developed on
LongMemEval. It is never used to develop Memro: this script reports summary
scores per category only, with no per-question failure list, and nothing from
the dataset goes into Memro's code or prompts.

    python bench/locomo/run_locomo.py --name locomo-openai     # ingest, answer, judge, report

Needs a running Memro server (MEMRO_API) and its database (MEMRO_DATABASE_URL),
like bench/longmemeval/run.py, and data/locomo10.json (gitignored):
    curl -fsSL -o bench/locomo/data/locomo10.json \\
      https://raw.githubusercontent.com/snap-research/locomo/main/data/locomo10.json
"""
import argparse
import concurrent.futures as cf
import datetime as dt
import json
import os
import pathlib
import statistics
import sys
import threading
import time
import uuid

ROOT = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT.parent / "longmemeval"))
from common import BudgetExceeded, RunTracker, chat, set_run_tracker  # noqa: E402
from run import (API, MAX_MEMORY_CHARS, PER_ITEM_CHARS, SESSION_EXPANSION, TOP_K, MemroClient, ask,  # noqa: E402
                 drop_agent, format_session_header, new_agent, pending_embeddings)

DATA = ROOT / "data" / "locomo10.json"
RUNS = ROOT / "runs"
NS = uuid.UUID("0b6f3d52-8c1e-4b8e-9a43-3f6f0c6e2d17")
# Category numbers as in the dataset; names as commonly reported for LoCoMo.
CATEGORIES = {1: "multi-hop", 2: "temporal", 3: "open-domain", 4: "single-hop"}  # 5 (adversarial) excluded

ANSWER_PROMPT = """You have a long-term memory of conversations between {a} and {b}.
Below are excerpts retrieved from that memory, each with the date of its session and its time relative to the current date. The current date is {now}.

{context}

Answer the question using the excerpts. Resolve relative dates ("yesterday", "last week") against the session date they were said in. If the excerpts don't contain the answer, say you don't know. Keep the answer short.

Think briefly, then end with a line "Answer: <your answer>".

Question: {question}"""

JUDGE_PROMPT = """You are grading an answer to a question about past conversations.
Question: {question}
Gold answer: {gold}
Generated answer: {answer}

Mark it CORRECT if the generated answer contains the same information as the gold answer (wording, formatting and date formats may differ; a date or time period that refers to the same moment is correct). Otherwise mark it WRONG. Reply with exactly one word: CORRECT or WRONG."""


def parse_when(s):
    """'1:56 pm on 8 May, 2023' -> datetime."""
    return dt.datetime.strptime(s.replace(" on ", " ").replace(",", ""), "%I:%M %p %d %B %Y")


def fmt(d):
    """The date format bench/longmemeval/run.py's helpers use."""
    return d.strftime("%Y/%m/%d (%a) %H:%M")


def sessions(conv):
    n = 1
    while f"session_{n}" in conv:
        yield n, parse_when(conv[f"session_{n}_date_time"]), conv[f"session_{n}"]
        n += 1


def turn_text(t):
    text = f"{t['speaker']}: {t['text']}"
    if t.get("blip_caption"):
        text += f" [shares a photo: {t['blip_caption']}]"
    return text[:MAX_MEMORY_CHARS]


def ingest(sample, client, agent):
    """One memory per dialogue turn, dated to its session."""
    meta, by_session = {}, {}
    for n, when, turns in sessions(sample["conversation"]):
        sess = str(uuid.uuid5(NS, f"{sample['sample_id']}:{n}"))
        by_session[n] = []
        for t in turns:
            m = client.create_memory(agent.agent_id, agent._private_key, turn_text(t), "episodic",
                                     event_date=when.strftime("%Y-%m-%dT%H:%M:%SZ"), session_id=sess)
            meta[m["id"]] = {"session": n, "date": fmt(when), "content": turn_text(t), "dia_id": t["dia_id"]}
            by_session[n].append(m["id"])
    deadline = time.time() + 1800
    while (left := pending_embeddings(agent.agent_id)) and time.time() < deadline:
        time.sleep(2)
    if left:
        raise RuntimeError(f"{left} memories still not embedded")
    return meta, by_session


def context_for(hits, meta, by_session):
    """Same assembly as LongMemEval's run.py: hits plus every turn of the top
    SESSION_EXPANSION sessions, in date order."""
    ids, top = [h["memory_id"] for h in hits], []
    for h in hits:
        s = meta[h["memory_id"]]["session"]
        if s not in top:
            top.append(s)
        if len(top) >= SESSION_EXPANSION:
            break
    for s in top:
        ids += [m for m in by_session[s] if m not in ids]
    return sorted(ids, key=lambda m: (meta[m]["date"], m))


def answer(q, now, conv, hits, meta, by_session):
    ctx = [f"{format_session_header(meta[m]['date'], now)}\n{meta[m]['content'][:PER_ITEM_CHARS]}"
           for m in context_for(hits, meta, by_session)]
    prompt = ANSWER_PROMPT.format(a=conv["speaker_a"], b=conv["speaker_b"], now=now,
                                  context="\n\n---\n\n".join(ctx) or "(nothing found)", question=q["question"])
    return ask(prompt)


def judge(q, hyp):
    final = hyp.split("Answer:")[-1].strip()
    out = chat("gpt-4o-mini", [{"role": "user", "content": JUDGE_PROMPT.format(
        question=q["question"], gold=q["answer"], answer=final)}], 5, 600 * 0.15 / 1e6)
    return out.strip().upper().startswith("CORRECT")


def run_sample(sample, out_file, lock, workers):
    client = MemroClient(API, api_key=None)
    agent = new_agent(client)
    conv = sample["conversation"]
    try:
        meta, by_session = ingest(sample, client, agent)
        now = fmt(max(w for _, w, _ in sessions(conv)))  # questions are asked after the last session
        dia_to_mem = {v["dia_id"]: k for k, v in meta.items()}

        def one(q):
            hits = client.search(agent.agent_id, q["question"][:500], limit=TOP_K,
                                 private_key=agent._private_key, mode="hybrid")
            hyp = answer(q, now, conv, hits, meta, by_session)
            evidence = [dia_to_mem[e] for e in q.get("evidence", []) if e in dia_to_mem]
            shown = set(context_for(hits, meta, by_session))
            hit_ids = [h["memory_id"] for h in hits]
            row = {"sample_id": sample["sample_id"], "category": q["category"],
                   "correct": judge(q, hyp), "evidence_total": len(evidence),
                   "evidence_in_top5": sum(e in hit_ids[:5] for e in evidence),
                   "evidence_in_top15": sum(e in hit_ids for e in evidence),
                   "evidence_in_context": sum(e in shown for e in evidence)}
            return row  # scores only: questions and answers are not stored

        qs = [q for q in sample["qa"] if q["category"] in CATEGORIES]
        with cf.ThreadPoolExecutor(workers) as ex:
            rows = list(ex.map(one, qs))
        with lock:  # a conversation's rows are written together, so resume never sees half of one
            with open(out_file, "a") as f:
                f.writelines(json.dumps(r) + "\n" for r in rows)
        print(f"done {sample['sample_id']}: {len(meta)} memories, {len(qs)} questions", flush=True)
    finally:
        drop_agent(agent)


def report(rows, name, meta):
    def line(label, rs):
        ev = [r for r in rs if r["evidence_total"]]
        evr = lambda k: statistics.mean(r[k] / r["evidence_total"] for r in ev) if ev else 0
        return (f"| {label} | {len(rs)} | {sum(r['correct'] for r in rs) / len(rs) * 100:.1f}% | "
                f"{evr('evidence_in_top5') * 100:.1f}% | {evr('evidence_in_top15') * 100:.1f}% | "
                f"{evr('evidence_in_context') * 100:.1f}% |")
    out = [f"# LoCoMo on Memro: {name}", "",
           "Held-out evaluation only (summary scores; no per-question results are kept). "
           f"Embedding model: `{meta['embedding_model']}`; answers: `{meta['answer_model']}`; "
           "judge: gpt-4o-mini. Category 5 (adversarial) excluded.", "",
           "| Category | n | Accuracy | Evidence R@5 | Evidence R@15 | Evidence in prompt |", "|---|---|---|---|---|---|"]
    for c, label in CATEGORIES.items():
        rs = [r for r in rows if r["category"] == c]
        if rs:
            out.append(line(f"{c} ({label})", rs))
    out.append(line("**overall**", rows))
    out += ["", f"Cost: ${meta.get('cost_usd', 0):.4f}"]
    return "\n".join(out) + "\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--name", default="locomo")
    ap.add_argument("--workers", type=int, default=8)
    args = ap.parse_args()
    data = json.loads(DATA.read_text())
    run_dir = RUNS / args.name
    run_dir.mkdir(parents=True, exist_ok=True)
    tracker = RunTracker(run_dir)
    tracker.update_metadata(name=args.name, created_at=dt.datetime.now(dt.timezone.utc).isoformat(),
                            embedding_model=os.environ.get("BENCH_EMBEDDING_MODEL", "text-embedding-3-small"),
                            answer_model=os.environ.get("ANSWER_MODEL", "gpt-4o-mini"), top_k=TOP_K,
                            session_expansion=SESSION_EXPANSION, dataset="locomo10.json")
    set_run_tracker(tracker)
    out_file = run_dir / "scores.jsonl"
    done = {json.loads(l)["sample_id"] for l in out_file.read_text().splitlines()} if out_file.exists() else set()
    lock = threading.Lock()
    for sample in data:
        if sample["sample_id"] in done:
            continue  # written all-or-nothing per conversation
        try:
            run_sample(sample, out_file, lock, args.workers)
        except BudgetExceeded as e:
            raise SystemExit(f"STOP: {e}")
    rows = [json.loads(l) for l in out_file.read_text().splitlines() if l.strip()]
    text = report(rows, args.name, tracker.data)
    (run_dir / "REPORT.md").write_text(text)
    print(text)


if __name__ == "__main__":
    main()
