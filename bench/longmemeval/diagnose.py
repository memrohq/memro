"""Failure analysis for a graded run: rebuild the exact prompt each wrong
answer was given, check whether the answer-bearing exchanges were in it, and
optionally re-answer the same prompt with another model.

    python diagnose.py runs/baseline-100                  # free: no LLM calls
    python diagnose.py runs/baseline-100 --reanswer gpt-4o

Retrieval is replayed against the same server (embeddings come from the
cache), and each replay is checked against the run's retrieved_sessions so the
rebuilt prompt is the one the answering model saw. Writes
<run>/diagnose/failures.jsonl, prompts/<id>.txt, and FAILURES.md.
"""
import argparse
import json
import pathlib

from common import RunTracker, set_run_tracker
from compare import NO_INFO_PHRASES
from judge import grade
from run import (API, MemroClient, answer_texts, ask, build_prompt, context_ids, drop_agent, load_data,
                 new_agent, retrieve)


def abstained(hyp):
    h = hyp.lower()
    tail = h.split("answer:")[-1]
    return any(p in tail for p in NO_INFO_PHRASES)


def final_answer(hyp):
    return hyp.split("Answer:")[-1].strip() if "Answer:" in hyp else hyp.strip()


def diagnose(q, row, reanswer):
    client = MemroClient(API, api_key=None)
    agent = new_agent(client)
    try:
        hits, meta, session_memories, _ = retrieve(q, client, agent)
    finally:
        drop_agent(agent)
    replay_sessions = [meta[h["memory_id"]]["session"] for h in hits]
    ids = context_ids(hits, meta, session_memories)
    prompt = build_prompt(q, ids, meta)

    gold = answer_texts(q)
    shown = {meta[i]["content"] for i in ids}
    gold_sessions = set(q["answer_session_ids"])
    out = {
        "question_id": q["question_id"], "question_type": q["question_type"],
        "question": q["question"], "question_date": q["question_date"],
        "expected": q["answer"], "hypothesis": row["hypothesis"],
        "abstained": abstained(row["hypothesis"]),
        "replay_matches_run": replay_sessions == row["retrieved_sessions"],
        "context_items": len(ids), "context_sessions": len({meta[i]["session"] for i in ids}),
        "answer_exchanges": len(gold), "answer_exchanges_in_context": len(gold & shown),
        "answer_sessions": len(gold_sessions),
        "answer_sessions_in_context": len(gold_sessions & {meta[i]["session"] for i in ids}),
        "prompt_chars": len(prompt),
    }
    if reanswer:
        hyp2 = ask(prompt, reanswer)
        out["reanswer_model"] = reanswer
        out["reanswer"] = hyp2
        out["reanswer_correct"] = grade({**row, "hypothesis": hyp2}, "gpt-4o-mini")
    return out, prompt


def evidence_label(r):
    if r["answer_exchanges"] == 0:
        return "abstention question" if "_abs" in r["question_id"] else "no marked evidence"
    if r["answer_exchanges_in_context"] == r["answer_exchanges"]:
        return "all evidence shown"
    if r["answer_exchanges_in_context"] == 0:
        return "no evidence shown"
    return "partial evidence"


def write_report(run, results):
    lines = [f"# Failure analysis: {run.name}", "",
             f"{len(results)} wrong answers. \"Evidence\" counts the exchanges LongMemEval marks as holding "
             "the answer (has_answer) that were in the prompt the answering model saw.", ""]
    counts = {}
    for r in results:
        counts[evidence_label(r)] = counts.get(evidence_label(r), 0) + 1
    lines += ["| Evidence in prompt | Failures |", "|---|---|"]
    lines += [f"| {k} | {v} |" for k, v in sorted(counts.items(), key=lambda kv: -kv[1])]
    lines += ["", f"- Abstained (\"don't know\" etc.): {sum(r['abstained'] for r in results)}",
              f"- Replay matched the run's retrieval: {sum(r['replay_matches_run'] for r in results)}/{len(results)}"]
    re = [r for r in results if "reanswer" in r]
    if re:
        lines.append(f"- Re-answered correctly by {re[0]['reanswer_model']} on the same prompt: "
                     f"{sum(r['reanswer_correct'] for r in re)}/{len(re)}")
    lines += ["", "## Questions", ""]
    for r in sorted(results, key=lambda r: (r["question_type"], r["question_id"])):
        lines += [
            f"### `{r['question_id']}` · {r['question_type']} · {evidence_label(r)}"
            f" ({r['answer_exchanges_in_context']}/{r['answer_exchanges']} exchanges,"
            f" {r['answer_sessions_in_context']}/{r['answer_sessions']} sessions)",
            "",
            f"- **Question** ({r['question_date']}): {r['question']}",
            f"- **Expected**: {r['expected']}",
            f"- **Answered**: {final_answer(r['hypothesis'])[:400]}",
        ]
        if "reanswer" in r:
            mark = "✅" if r["reanswer_correct"] else "❌"
            lines.append(f"- **{r['reanswer_model']}** {mark}: {final_answer(r['reanswer'])[:400]}")
        lines += [f"- Prompt: `prompts/{r['question_id']}.txt` ({r['context_items']} items from "
                  f"{r['context_sessions']} sessions, {r['prompt_chars']:,} chars)", ""]
    (run / "diagnose" / "FAILURES.md").write_text("\n".join(lines) + "\n")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("run_dir")
    ap.add_argument("--model", default="gpt-4o-mini", help="judge whose labels define the failures")
    ap.add_argument("--reanswer", default=None, help="re-answer each rebuilt prompt with this model")
    args = ap.parse_args()
    run = pathlib.Path(args.run_dir)
    rows = {json.loads(l)["question_id"]: json.loads(l) for l in (run / "hyp.jsonl").read_text().splitlines() if l.strip()}
    labels = {json.loads(l)["question_id"]: json.loads(l)["label"]
              for l in (run / f"eval-{args.model}.jsonl").read_text().splitlines() if l.strip()}
    failed = sorted(i for i, ok in labels.items() if not ok)
    data = {q["question_id"]: q for q in load_data() if q["question_id"] in set(failed)}

    out_dir = run / "diagnose"
    (out_dir / "prompts").mkdir(parents=True, exist_ok=True)
    if args.reanswer:
        set_run_tracker(RunTracker(out_dir))

    results = []
    for i in failed:
        r, prompt = diagnose(data[i], rows[i], args.reanswer)
        (out_dir / "prompts" / f"{i}.txt").write_text(prompt)
        results.append(r)
        print(f"{i} {r['question_type']}: {evidence_label(r)}, replay_match={r['replay_matches_run']}"
              + (f", reanswer_correct={r['reanswer_correct']}" if args.reanswer else ""), flush=True)
    (out_dir / "failures.jsonl").write_text("".join(json.dumps(r) + "\n" for r in results))
    write_report(run, results)
    print(f"-> {out_dir / 'FAILURES.md'}")


if __name__ == "__main__":
    main()
