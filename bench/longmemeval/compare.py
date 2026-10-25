#!/usr/bin/env python3
"""Compare two LongMemEval runs: per-category deltas, retrieval recall deltas,
and questions flipped wrong->right or right->wrong.

    python compare.py runs/baseline-100 runs/phase1-100 [--ci-gate]
"""
import argparse
import json
import math
import pathlib
import statistics
import sys

NO_INFO_PHRASES = [
    "don't know", "do not know", "no information", "not provided", "not mentioned",
    "does not mention", "does not contain", "cannot be determined", "no record",
    "don't have any information", "not specify", "not specified", "unknown"
]


def load_run(run_path: pathlib.Path, judge_model: str = "gpt-4o-mini"):
    hyp_file = run_path / "hyp.jsonl"
    eval_file = run_path / f"eval-{judge_model}.jsonl"
    if not hyp_file.exists():
        raise SystemExit(f"Missing {hyp_file}")
    if not eval_file.exists():
        raise SystemExit(f"Missing {eval_file}")

    rows = {json.loads(l)["question_id"]: json.loads(l) for l in hyp_file.read_text().splitlines() if l.strip()}
    labels = {json.loads(l)["question_id"]: json.loads(l)["label"] for l in eval_file.read_text().splitlines() if l.strip()}
    
    meta_file = run_path / "meta.json"
    meta = json.loads(meta_file.read_text()) if meta_file.exists() else {}

    return rows, labels, meta


def mcnemar_p(fixed: int, broke: int) -> float:
    """Exact two-sided McNemar test on paired flips: how likely a split this
    uneven is if the change did nothing (answers flip both ways at random)."""
    n = fixed + broke
    if n == 0:
        return 1.0
    k = min(fixed, broke)
    tail = sum(math.comb(n, i) for i in range(k + 1)) / 2 ** n
    return min(1.0, 2 * tail)


def evidence_metrics(rows: dict) -> dict | None:
    """Exchange-level evidence from rows that record it (deterministic: no LLM)."""
    ev = [r for r in rows.values() if r.get("evidence_total")]
    if not ev or "hit_evidence" not in ev[0]:
        return None
    hits = [h for r in rows.values() if "hit_evidence" in r for h in r["hit_evidence"]]
    return {
        "recall15": statistics.mean(sum(r["hit_evidence"]) / r["evidence_total"] for r in ev),
        "precision15": sum(hits) / len(hits) if hits else 0.0,
        "all_in_prompt": sum(r["evidence_in_context"] == r["evidence_total"] for r in ev),
        "n": len(ev),
        "missing": {r["question_id"]: r["evidence_total"] - r["evidence_in_context"] for r in ev},
    }


def compute_metrics(rows, labels):
    types = sorted({r["question_type"] for r in rows.values()})
    categories = types + ["overall"]

    stats = {}
    for cat in categories:
        ids = [i for i, r in rows.items() if (cat == "overall" or r["question_type"] == cat) and i in labels]
        if not ids:
            continue
        correct = sum(1 for i in ids if labels.get(i))
        total = len(ids)
        acc = correct / total if total > 0 else 0.0

        # Retrieval recall
        r5 = sum(1 for i in ids if rows[i]["answer_session_ids"] and any(s in rows[i]["answer_session_ids"] for s in rows[i]["retrieved_sessions"][:5]))
        r10 = sum(1 for i in ids if rows[i]["answer_session_ids"] and any(s in rows[i]["answer_session_ids"] for s in rows[i]["retrieved_sessions"][:10]))
        r15 = sum(1 for i in ids if rows[i]["answer_session_ids"] and any(s in rows[i]["answer_session_ids"] for s in rows[i]["retrieved_sessions"][:15]))
        r_total = sum(1 for i in ids if rows[i]["answer_session_ids"])

        # Failures split
        fails = [i for i in ids if not labels.get(i)]
        in_ctx = sum(1 for i in fails if rows[i]["answer_session_ids"] and set(rows[i]["answer_session_ids"]).issubset(set(rows[i]["retrieved_sessions"][:15])))
        missed = len(fails) - in_ctx

        stats[cat] = {
            "total": total,
            "correct": correct,
            "acc": acc,
            "r5": r5 / r_total if r_total else 0.0,
            "r10": r10 / r_total if r_total else 0.0,
            "r15": r15 / r_total if r_total else 0.0,
            "fails": len(fails),
            "in_ctx": in_ctx,
            "missed": missed,
        }

    # Duplicate slots
    dups = [len(r["retrieved_sessions"][:15]) - len(set(r["retrieved_sessions"][:15])) for r in rows.values()]
    avg_dups = sum(dups) / len(dups) if dups else 0.0

    # Abstention
    abs_ids = [i for i, r in rows.items() if ("_abs" in i or not r["answer_session_ids"]) and i in labels]
    abs_acc = sum(1 for i in abs_ids if labels.get(i)) / len(abs_ids) if abs_ids else 0.0

    # Said no info while evidence present
    no_info_in_ctx = 0
    for i, r in rows.items():
        if i in labels and not labels.get(i):
            gold = set(r["answer_session_ids"])
            if gold and gold.issubset(set(r["retrieved_sessions"][:15])):
                h = r["hypothesis"].lower()
                ans_text = h.split("answer:")[-1] if "answer:" in h else h
                if any(p in ans_text for p in NO_INFO_PHRASES) or any(p in h for p in NO_INFO_PHRASES):
                    no_info_in_ctx += 1

    # Latencies
    s = [r["search_ms"] for r in rows.values() if "search_ms" in r]
    s.sort()
    p50 = statistics.median(s) if s else 0.0
    p95 = s[int(0.95 * (len(s) - 1))] if s else 0.0

    return stats, avg_dups, abs_acc, len(abs_ids), no_info_in_ctx, p50, p95


def main():
    parser = argparse.ArgumentParser(description="Compare two LongMemEval benchmark runs.")
    parser.add_argument("run_a", type=pathlib.Path, help="Baseline run directory")
    parser.add_argument("run_b", type=pathlib.Path, help="New/comparison run directory")
    parser.add_argument("--model", default="gpt-4o-mini", help="Judge model (default: gpt-4o-mini)")
    parser.add_argument("--ci-gate", action="store_true",
                        help="Exit 1 on a significant paired accuracy regression or on lost evidence")
    parser.add_argument("--targets", type=pathlib.Path, help="target question set (bench/longmemeval/targets/*.json)")
    args = parser.parse_args()

    rows_a, labels_a, meta_a = load_run(args.run_a, args.model)
    rows_b, labels_b, meta_b = load_run(args.run_b, args.model)

    stats_a, dups_a, abs_a, abs_n_a, no_info_a, p50_a, p95_a = compute_metrics(rows_a, labels_a)
    stats_b, dups_b, abs_b, abs_n_b, no_info_b, p50_b, p95_b = compute_metrics(rows_b, labels_b)

    # Markdown report
    lines = [
        f"# Benchmark Comparison: `{args.run_a.name}` -> `{args.run_b.name}`",
        "",
        f"- **Baseline Run (`A`)**: {len(rows_a)} questions ({args.run_a})",
        f"- **New Run (`B`)**: {len(rows_b)} questions ({args.run_b})",
    ]
    if meta_b.get("commit"):
        lines.append(f"- **Server Commit**: `{meta_b['commit']}`")
    if meta_b.get("engine_options"):
        lines.append(f"- **Engine Options**: `{json.dumps(meta_b['engine_options'])}`")
    if meta_b.get("cost_usd") is not None:
        lines.append(f"- **Run B Cost**: ${meta_b['cost_usd']:.4f}")

    lines.extend([
        "",
        "## Category Accuracy & Retrieval Delta",
        "",
        "| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |",
        "|---|---|---|---|---|---|---|---|---|",
    ])

    gate_failed = False
    regressions = []

    # Accuracy is only computed over judged answers, so a run that drops
    # questions could look fine. Both runs must cover the same judged set.
    missing_b = sorted(set(labels_a) - set(labels_b))
    unjudged = {n: len(set(r) - set(l)) for n, r, l in (("A", rows_a, labels_a), ("B", rows_b, labels_b))}
    if missing_b:
        gate_failed = True
        regressions.append(f"Run B is missing {len(missing_b)} judged questions present in A: {', '.join(missing_b[:10])}")
    for n, k in unjudged.items():
        if k:
            gate_failed = True
            regressions.append(f"Run {n} has {k} answers without a judge label")

    all_cats = [c for c in stats_a if c != "overall"] + ["overall"]
    for cat in all_cats:
        sa = stats_a.get(cat, {})
        sb = stats_b.get(cat, {})
        if not sa or not sb:
            continue
        delta = sb["acc"] - sa["acc"]
        delta_str = f"{delta * 100:+.1f}%"
        if delta > 0.001:
            delta_str = f"**{delta_str}** 🟢"
        elif delta < -0.001:
            delta_str = f"**{delta_str}** 🔴"
        else:
            delta_str = f"{delta_str} ⚪"

        # Accuracy is gated below with a paired test on the flips. A rerun of an
        # identical config moved overall 3 points and categories up to 19, so
        # per-category deltas are reported, not gated.

        lines.append(
            f"| {cat} | {sb['total']} | {sa['acc'] * 100:5.1f}% | {sb['acc'] * 100:5.1f}% | {delta_str} | "
            f"{sa['r5'] * 100:.1f}% / {sb['r5'] * 100:.1f}% | {sa['r15'] * 100:.1f}% / {sb['r15'] * 100:.1f}% | "
            f"{sa['fails']} ({sa['in_ctx']}/{sa['missed']}) | {sb['fails']} ({sb['in_ctx']}/{sb['missed']}) |"
        )

    lines.extend([
        "",
        "## Synthesis & Quality Metrics",
        "",
        "| Metric | Baseline (`A`) | New (`B`) | Delta |",
        "|---|---|---|---|",
        f"| Duplicate session slots (avg / 15) | {dups_a:.2f} | {dups_b:.2f} | {dups_b - dups_a:+.2f} |",
        f"| Abstention accuracy | {abs_a * 100:.1f}% ({abs_a * abs_n_a:.0f}/{abs_n_a}) | {abs_b * 100:.1f}% ({abs_b * abs_n_b:.0f}/{abs_n_b}) | {(abs_b - abs_a) * 100:+.1f}% |",
        f"| Said 'no information' while evidence present | {no_info_a} | {no_info_b} | {no_info_b - no_info_a:+d} |",
        f"| Search latency p50 / p95 | {p50_a:.0f} ms / {p95_a:.0f} ms | {p50_b:.0f} ms / {p95_b:.0f} ms | {p50_b - p50_a:+.0f} ms / {p95_b - p95_a:+.0f} ms |",
    ])

    # Flipped questions
    common_ids = sorted(set(labels_a.keys()) & set(labels_b.keys()))
    fixed = [i for i in common_ids if not labels_a[i] and labels_b[i]]
    broke = [i for i in common_ids if labels_a[i] and not labels_b[i]]
    p = mcnemar_p(len(fixed), len(broke))
    lines.extend(["", f"Paired test on flips: {len(fixed)} fixed vs {len(broke)} regressed, "
                      f"McNemar p = {p:.3f} ({'significant' if p < 0.05 else 'not significant'} at 0.05)."])
    if len(broke) > len(fixed) and p < 0.05:
        gate_failed = True
        regressions.append(f"Accuracy regressed significantly: {len(broke)} right->wrong vs {len(fixed)} wrong->right (p = {p:.3f})")

    ev_a, ev_b = evidence_metrics(rows_a), evidence_metrics(rows_b)
    if ev_a and ev_b:
        lines.extend(["", "## Evidence (exact answer-bearing exchanges; deterministic, no LLM)", "",
                      "| Metric | A | B |", "|---|---|---|",
                      f"| Evidence recall@15 | {ev_a['recall15'] * 100:.1f}% | {ev_b['recall15'] * 100:.1f}% |",
                      f"| Evidence precision@15 | {ev_a['precision15'] * 100:.1f}% | {ev_b['precision15'] * 100:.1f}% |",
                      f"| Questions with all evidence in the prompt | {ev_a['all_in_prompt']}/{ev_a['n']} | {ev_b['all_in_prompt']}/{ev_b['n']} |"])
        lost = sorted(i for i in ev_b["missing"] if ev_b["missing"][i] > ev_a["missing"].get(i, 0))
        gained = sorted(i for i in ev_b["missing"] if ev_b["missing"][i] < ev_a["missing"].get(i, 0))
        lines.append(f"\nEvidence gained on {len(gained)} questions, lost on {len(lost)}"
                     + (f": {', '.join(lost[:15])}" if lost else "."))
        if ev_b["all_in_prompt"] < ev_a["all_in_prompt"]:
            gate_failed = True
            regressions.append(f"Fewer questions have all their evidence in the prompt: "
                               f"{ev_b['all_in_prompt']} vs {ev_a['all_in_prompt']}")

    if args.targets:
        target_ids = [t["question_id"] for t in json.loads(args.targets.read_text())["questions"]]
        lines.extend(["", f"## Target set `{args.targets.stem}` ({len(target_ids)} questions)", "",
                      "| | A | B |", "|---|---|---|",
                      f"| Correct | {sum(labels_a.get(i, False) for i in target_ids)} | {sum(labels_b.get(i, False) for i in target_ids)} |"])
        if ev_a and ev_b:
            lines.append(f"| All evidence in prompt | {sum(ev_a['missing'].get(i, 1) == 0 for i in target_ids)} "
                         f"| {sum(ev_b['missing'].get(i, 1) == 0 for i in target_ids)} |")

    lines.extend([
        "",
        f"## Question Flips (Total Fixed: {len(fixed)}, Total Regressed: {len(broke)})",
        "",
        f"### Fixed (Wrong in A -> Right in B: {len(fixed)})",
    ])
    if fixed:
        for i in fixed:
            q_row = rows_b.get(i, rows_a.get(i, {}))
            q_text = q_row.get("question", "")[:90].replace("\n", " ")
            q_type = q_row.get("question_type", "")
            lines.append(f"- **`{i}`** (`{q_type}`): {q_text}")
    else:
        lines.append("- *(none)*")

    lines.extend([
        "",
        f"### Regressed (Right in A -> Wrong in B: {len(broke)})",
    ])
    if broke:
        for i in broke:
            q_row = rows_b.get(i, rows_a.get(i, {}))
            q_text = q_row.get("question", "")[:90].replace("\n", " ")
            q_type = q_row.get("question_type", "")
            lines.append(f"- **`{i}`** (`{q_type}`): {q_text}")
    else:
        lines.append("- *(none)*")

    report_text = "\n".join(lines)
    print(report_text)

    # Save comparison to run_b
    (args.run_b / f"COMPARE_VS_{args.run_a.name.upper()}.md").write_text(report_text + "\n")

    if regressions:
        print("\n## Gate Issues\n")
        for r in regressions:
            print(f"- {r}")

    if args.ci_gate and gate_failed:
        print("\n❌ CI GATE FAILED")
        sys.exit(1)


if __name__ == "__main__":
    main()
