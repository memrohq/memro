# LongMemEval Benchmark Report: Memro

*Status: Verified Run · Date: October 2026 · Dataset: LongMemEval_S (Full Continuity)*

This report publishes the methodology, raw evaluation results, latency metrics, and API costs for Memro evaluated on the public **LongMemEval** benchmark suite across 500 challenge questions.

In keeping with our transparency commitments, we disclose exact costs, hardware setup, and areas where Memro underperformed or has room for architectural improvement.

---

## 1. Executive Summary

| Metric | Result | Notes |
|---|---|---|
| **Total Questions Evaluated** | 500 / 500 | Full continuity run (no cherry-picking) |
| **Grading Model** | `gpt-4o-mini` | Evaluated against official ground truth (agreed with `gpt-4o` on 49/50 sample) |
| **Embedding Model** | `text-embedding-3-small` | 1536-dimensional dense vectors |
| **Overall QA Accuracy** | **77.6%** (388 / 500) | Correct answers confirmed by judge |
| **Recall@5** | **97.6%** (488 / 500) | Ground truth fact present in top 5 items |
| **Recall@10** | **99.0%** (495 / 500) | Ground truth fact present in top 10 items |
| **Query Latency (p50)** | **401 ms** | Local Docker Postgres + pgvector (~249 memories/question) |
| **Query Latency (p95)** | **1074 ms** | Uncached hybrid vector + full-text search |
| **Total Evaluation Cost** | **$2.8664** | Cumulative spend across all runs (~$1.25 for single 500-question pass) |

---

## 2. Category Breakdown

The benchmark evaluates six distinct challenge types:

| Challenge Type | Count (n) | QA Accuracy | Recall@5 | Recall@10 |
|---|---|---|---|---|
| **knowledge-update** | 78 | 82.1% (64/78) | 100.0% (78/78) | 100.0% (78/78) |
| **multi-session** | 133 | 67.7% (90/133) | 97.7% (130/133) | 100.0% (133/133) |
| **single-session-assistant** | 56 | 96.4% (54/56) | 100.0% (56/56) | 100.0% (56/56) |
| **single-session-preference** | 30 | 70.0% (21/30) | 90.0% (27/30) | 96.7% (29/30) |
| **single-session-user** | 70 | 95.7% (67/70) | 100.0% (70/70) | 100.0% (70/70) |
| **temporal-reasoning** | 133 | 69.2% (92/133) | 95.5% (127/133) | 97.0% (129/133) |
| **Overall** | **500** | **77.6% (388/500)** | **97.6% (488/500)** | **99.0% (495/500)** |

---

## 3. Cost & Spend Disclosures

All LLM and embedding calls were tracked through a deterministic token ledger:

- **`text-embedding-3-small`**:
  - Spend: **$0.8158**
  - Tokens: 40,792,124 input / 0 output
  - Calls: 3,879 calls
- **`gpt-4o-mini`** (QA generation and automated grading):
  - Spend: **$2.0114**
  - Tokens: 12,801,684 input / 151,877 output
  - Calls: 3,201 calls
- **`gpt-4o`** (Ambiguity resolution sample):
  - Spend: **$0.0392**
  - Tokens: 15,208 input / 115 output
  - Calls: 63 calls
- **Total Ledger Spend**: **$2.8664** (single full-run pass cost ~$1.25)

---

## 4. Analysis: Strengths and Disclosed Limitations

### What Worked Well:
1. **Near-Perfect Top-K Retrieval**:
   Recall@5 at 97.6% and Recall@10 at 99.0% prove that combining dense pgvector embeddings with PostgreSQL full-text search via Reciprocal Rank Fusion ($k=60$) reliably injects the necessary ground truth into the top context slots.
2. **Knowledge Updates & Single-Session Facts**:
   Single-session facts achieved 95.7%–96.4% accuracy, and knowledge updates reached 82.1%. Memro's explicit memory typing and contradiction detection logic successfully pruned obsolete assertions when updated facts were presented.

### Where Memro Lost or Needs Improvement:
1. **Multi-Session Contexts (67.7%)**:
   When answers required synthesizing disparate facts spread across 5+ distinct historical sessions, accuracy dropped to 67.7%. While Recall@10 was 100%, the LLM sometimes struggled to synthesize multi-hop relationships when presented with flat list contexts.
2. **Temporal Reasoning (69.2%)**:
   Questions requiring complex date differences and sequential event ordering scored 69.2%. While Memro's `BuildTemporalLedger` calculates chronological sequence and day offsets, relative calendar reasoning across distant multi-year sessions remains an active optimization focus.
3. **Synthesis vs. Retrieval Bottleneck**:
   Detailed error inspection across the 112 failed questions showed that **73 of the 112 wrong answers (65.2%) had all required ground-truth evidence successfully retrieved in the top context**. The primary bottleneck was the answering LLM failing to aggregate multi-session facts or compute day differences, rather than retrieval failure.

---

## 5. How to Reproduce

Memro provides two reproducible evaluation pathways:

### A. LongMemEval_S 500-Question Benchmark

The complete benchmark harness, dataset loaders, and scoring scripts are in the [`memro-benchmark`](https://github.com/memrohq/memro-benchmark) repository:

```bash
# 1. Start PostgreSQL with pgvector and Memro API
docker compose up -d

# 2. Clone the benchmark suite
git clone https://github.com/memrohq/memro-benchmark.git
cd memro-benchmark/longmemeval

# 3. Run pilot (12 questions) or full evaluation (500 questions)
python run.py --pilot   # Fast 12-question pilot run
python run.py --all     # Full 500-question run (supports resume)

# 4. Grade with LLM judge and generate report
python judge.py runs/full-continuity
python report.py runs/full-continuity
```

### B. In-Repo Measurable Verification Suite

For fast local verification of multi-tenant isolation, contradiction handling, stale fact handling, and distractor rejection without external LLM dependencies:

```bash
# Run against local Memro server (http://localhost:8081)
python bench/eval_suite.py http://localhost:8081
```

Live web version: [https://memro.co/benchmarks](https://memro.co/benchmarks)
