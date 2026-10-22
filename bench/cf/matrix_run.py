#!/usr/bin/env python3
"""Multi-Configuration Benchmark Matrix Runner.

Evaluates Memro recall quality across embedding models and vector backends:
1. Baseline: OpenAI text-embedding-3-small + PostgreSQL pgvector
2. Workers AI BGE-small (@cf/baai/bge-small-en-v1.5) + PostgreSQL pgvector
3. Workers AI BGE-M3 (@cf/baai/bge-m3) + PostgreSQL pgvector
4. Workers AI BGE-small + Cloudflare Vectorize

Methodology:
- Dataset: fixed LongMemEval 100-question stratified subset.
- Evaluator: neutral OpenAI gpt-4o-mini judge.
- Metrics: Recall@5, Recall@10, Recall@15, MRR, QA accuracy, p50/p95 latency, cost/1k queries.
"""
import argparse
import datetime as dt
import json
import os
import pathlib
import statistics
import sys
import time

ROOT = pathlib.Path(__file__).resolve().parent
BENCH = ROOT.parent
LONGMEMEVAL = BENCH / "longmemeval"
RUNS = ROOT / "matrix_runs"
RUNS.mkdir(exist_ok=True)

# List prices for cost calculations ($ per 1M tokens or operations)
COST_TABLE = {
    "text-embedding-3-small": {"embed_per_1m": 0.02, "storage": "pgvector"},
    "@cf/baai/bge-small-en-v1.5": {"embed_per_1m": 0.011, "storage": "pgvector"},
    "@cf/baai/bge-m3": {"embed_per_1m": 0.016, "storage": "pgvector"},
    "vectorize_queries": {"per_1m": 0.04},
}


def load_matrix_configs():
    return [
        {
            "id": "baseline_openai_pgvector",
            "name": "Baseline (OpenAI text-embedding-3-small + pgvector)",
            "embed_model": "text-embedding-3-small",
            "backend": "pgvector",
            "embed_port": 18200,
        },
        {
            "id": "workers_ai_bge_small_pgvector",
            "name": "Workers AI BGE-Small + pgvector",
            "embed_model": "@cf/baai/bge-small-en-v1.5",
            "backend": "pgvector",
            "embed_port": 18201,
        },
        {
            "id": "workers_ai_bge_m3_pgvector",
            "name": "Workers AI BGE-M3 + pgvector",
            "embed_model": "@cf/baai/bge-m3",
            "backend": "pgvector",
            "embed_port": 18201,
        },
        {
            "id": "workers_ai_bge_small_vectorize",
            "name": "Workers AI BGE-Small + Cloudflare Vectorize",
            "embed_model": "@cf/baai/bge-small-en-v1.5",
            "backend": "vectorize",
            "embed_port": 18201,
        },
    ]


def compute_mrr(retrieved_sessions: list, gold_sessions: list) -> float:
    for idx, s in enumerate(retrieved_sessions):
        if s in gold_sessions:
            return 1.0 / (idx + 1)
    return 0.0


def evaluate_run(run_dir: pathlib.Path) -> dict:
    hyp_file = run_dir / "hyp.jsonl"
    eval_file = run_dir / "eval-gpt-4o-mini.jsonl"
    meta_file = run_dir / "meta.json"

    if not hyp_file.exists():
        return {}

    rows = [json.loads(l) for l in hyp_file.read_text().splitlines() if l.strip()]
    labels = {json.loads(l)["question_id"]: json.loads(l)["label"]
              for l in eval_file.read_text().splitlines() if l.strip()} if eval_file.exists() else {}

    n = len(rows)
    acc = sum(1 for r in rows if labels.get(r["question_id"])) / n if n else 0.0

    r5_count, r10_count, r15_count, mrr_total = 0, 0, 0, 0.0
    r_eligible = 0
    for r in rows:
        gold = r.get("answer_session_ids", [])
        if not gold:
            continue
        r_eligible += 1
        ret = r.get("retrieved_sessions", [])
        if any(s in gold for s in ret[:5]):
            r5_count += 1
        if any(s in gold for s in ret[:10]):
            r10_count += 1
        if any(s in gold for s in ret[:15]):
            r15_count += 1
        mrr_total += compute_mrr(ret[:15], gold)

    latencies = [r.get("search_ms", 0.0) for r in rows if "search_ms" in r]
    latencies.sort()
    p50 = statistics.median(latencies) if latencies else 0.0
    p95 = latencies[int(0.95 * (len(latencies) - 1))] if latencies else 0.0

    meta = json.loads(meta_file.read_text()) if meta_file.exists() else {}
    cost = meta.get("cost_usd", 0.0)

    return {
        "n": n,
        "qa_accuracy": acc,
        "recall_at_5": r5_count / r_eligible if r_eligible else 0.0,
        "recall_at_10": r10_count / r_eligible if r_eligible else 0.0,
        "recall_at_15": r15_count / r_eligible if r_eligible else 0.0,
        "mrr": mrr_total / r_eligible if r_eligible else 0.0,
        "p50_ms": p50,
        "p95_ms": p95,
        "cost_usd": cost,
    }


def generate_matrix_report(results: dict) -> str:
    lines = [
        "# Cloudflare Startup Grant: Benchmark Matrix Report",
        "",
        f"Generated at: {dt.datetime.now(dt.timezone.utc).strftime('%Y-%m-%d %H:%M:%S UTC')}",
        "Dataset: LongMemEval 100-Question Stratified Subset (Seeded)",
        "Independent Judge: OpenAI gpt-4o-mini",
        "",
        "## Performance & Cost Comparison Matrix",
        "",
        "| Configuration | Embedding Model | Vector Backend | QA Acc | Recall@5 | Recall@15 | MRR | p50 / p95 (ms) | Run Spend | Est. Cost / 1k Qs |",
        "|---|---|---|---|---|---|---|---|---|---|",
    ]

    for cfg_id, data in results.items():
        name = data.get("name", cfg_id)
        model = data.get("embed_model", "-")
        backend = data.get("backend", "-")
        m = data.get("metrics", {})

        if m:
            acc_str = f"{m['qa_accuracy'] * 100:.1f}%"
            r5_str = f"{m['recall_at_5'] * 100:.1f}%"
            r15_str = f"{m['recall_at_15'] * 100:.1f}%"
            mrr_str = f"{m['mrr']:.3f}"
            lat_str = f"{m['p50_ms']:.0f} / {m['p95_ms']:.0f}"
            cost_str = f"${m['cost_usd']:.4f}"
            cost_1k = f"${(m['cost_usd'] / m['n'] * 1000):.2f}" if m.get("n") else "-"
        else:
            acc_str, r5_str, r15_str, mrr_str, lat_str, cost_str, cost_1k = ("-", "-", "-", "-", "-", "-", "-")

        lines.append(f"| {name} | `{model}` | `{backend}` | {acc_str} | {r5_str} | {r15_str} | {mrr_str} | {lat_str} | {cost_str} | {cost_1k} |")

    lines.extend([
        "",
        "## Observations & Key Findings",
        "- **Workers AI Embeddings**: High throughput, lowest cost per token. Compare retrieval recall to ensure no degradation against proprietary embeddings.",
        "- **Vectorize vs pgvector**: Vectorize provides serverless scale, while pgvector enables zero-network-hop relational joins with memory graphs and metadata.",
        "- **Evaluation Isolation**: OpenAI gpt-4o-mini serves as the impartial baseline judge across all tested configurations.",
    ])

    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="Cloudflare Benchmark Matrix Runner")
    parser.add_argument("--subset", default="bench/longmemeval/subset_100.json")
    parser.add_argument("--report-only", action="store_true")
    args = parser.parse_args()

    configs = load_matrix_configs()
    results = {}

    # Check for baseline run
    baseline_dir = BENCH / "longmemeval" / "runs" / "baseline-100"
    if not baseline_dir.exists():
        baseline_dir = BENCH / "runs" / "baseline-100"
    if baseline_dir.exists():
        results["baseline_openai_pgvector"] = {
            "name": "Baseline (OpenAI + pgvector)",
            "embed_model": "text-embedding-3-small",
            "backend": "pgvector",
            "metrics": evaluate_run(baseline_dir),
        }

    report_md = generate_matrix_report(results)
    out_file = RUNS / "MATRIX_REPORT.md"
    out_file.write_text(report_md + "\n")
    print(report_md)
    print(f"\nReport written to: {out_file}")


if __name__ == "__main__":
    main()
