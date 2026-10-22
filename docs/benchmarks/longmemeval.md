# LongMemEval Benchmark Report: Memro

*Status: Verified Runs · Date: October 2026 · Dataset: LongMemEval_S*

This report publishes the methodology, evaluation results, latency metrics, and API costs for Memro evaluated on the public **LongMemEval** benchmark suite across 500 challenge questions.

---

## 1. Executive Summary & Full Runs Comparison

We conducted three full evaluations on the complete 500-question LongMemEval_S dataset. All three runs are published in `bench/longmemeval/runs/`:

| Run | QA Accuracy | Recall@5 | Recall@10 | Latency (p50 / p95) | Differences / Configuration |
|---|---|---|---|---|---|
| **full** | **77.6%** (388/500) | **98.6%** (493/500) | **98.8%** (494/500) | 564 ms / 1053 ms | TOP_K=10 memories retrieved per question. |
| **full-fixed** | **75.0%** (375/500) | **97.0%** (485/500) | **98.8%** (494/500) | 394 ms / 910 ms | TOP_K=15 memories retrieved; memories presented as individual turn excerpts without conversation continuity context. |
| **full-continuity** | **77.6%** (388/500) | **97.6%** (488/500) | **99.0%** (495/500) | 401 ms / 1074 ms | TOP_K=15 memories retrieved; memories formatted with conversational turn continuity, improving preference and multi-session synthesis. |

*Notes on Models*:
- **Grading Model**: `gpt-4o-mini` evaluated against official ground truth (agreed with `gpt-4o` on 49/50 sampled answers; `gpt-4o` accuracy on that sample was 74.0%).
- **Embedding Model**: `text-embedding-3-small` (1536-dimensional dense vectors).
- **Environment**: Local Docker PostgreSQL 16 + pgvector (~249 memories ingested per question).

---

## 2. Category Breakdown (`full-continuity`)

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

All LLM and embedding calls were tracked through a deterministic token ledger (`bench/longmemeval/runs/ledger.json`):

- **Cumulative Spend (All Runs & Experiments)**: **$3.5006**
  - `text-embedding-3-small`: $0.8158 (40,792,124 input tokens across 3,879 calls)
  - `gpt-4o-mini`: $2.6456 (16,828,341 prompt tokens, 202,232 completion tokens across 4,097 calls)
  - `gpt-4o` (judge cross-validation sample): $0.0392 (15,208 prompt tokens, 115 completion tokens across 63 calls)
- **Estimated Single Full-Run Pass**: **~$1.25**
  - *Calculation*:
    - Initial text embedding (500 questions × ~249 turns = ~124,500 turns; ~40.8M tokens at $0.02 / 1M tokens): **$0.82** (cached on rerun via `embed_cache.sqlite`).
    - Answering (500 questions × ~4,000 prompt tokens with top 15 memories at $0.15 / 1M tokens + ~100 completion tokens at $0.60 / 1M tokens): **~$0.33**.
    - LLM judge grading (500 questions × ~1,200 prompt tokens + ~50 completion tokens at $0.15/$0.60 per 1M tokens): **~$0.10**.
    - Total estimated per-pass cost: $0.82 + $0.33 + $0.10 ≈ **$1.25** (or **~$0.43** per run when embedding cache is populated).

---

## 4. Analysis: Strengths and Bottlenecks

### What Worked Well:
1. **High Top-K Retrieval**:
   Recall@5 at 97.6% and Recall@10 at 99.0% show that combining dense pgvector embeddings with PostgreSQL full-text search via Reciprocal Rank Fusion ($k=60$) reliably puts the necessary ground-truth sessions into the top context slots.
2. **Single-Session Facts & Updates**:
   Single-session facts achieved 95.7%–96.4% QA accuracy, and knowledge updates reached 82.1%.

### Synthesis vs. Retrieval Bottleneck:
- The answer generation step receives TOP_K=15 (`run.py:42`).
- On a coarse session level, Recall@15 is high: 94 of the 112 failed questions (84%) in the full 500-question run and 17 of 22 failures in the 100-question subset had an evidence-bearing session in the retrieved context.
- **However, measuring recall at the session level masks exchange-level gaps**:
  - In practice, conversations span many turns, while Memro retrieves individual exchanges.
  - When evaluated by exact answer-bearing exchange, **9 of the 22 baseline-100 failures were actually missing evidence** (7 had only partial evidence retrieved, and 2 had zero evidence retrieved).
  - The remaining 13 failures had all necessary evidence retrieved in context, where the failure was synthesis (e.g. 8 questions the answering model failed to reason over, 2 stale knowledge updates, 1 preference abstention, 1 unanswerable question answered anyway, and 1 dataset quirk).
- Therefore, both retrieval granularity (ensuring all answer-bearing exchanges are included) and synthesis (handling updates, temporal ordering, and reasoning) are active areas of improvement.

---

## 5. How to Reproduce

The complete benchmark harness, dataset loaders, subset definitions, and run outputs are included in [`bench/longmemeval/`](../../bench/longmemeval/):

```bash
# 1. Start PostgreSQL with pgvector (using port 55434 or custom DATABASE_URL)
docker run -d --name memro-bench-db -p 55434:5432 \
  -e POSTGRES_USER=memro -e POSTGRES_PASSWORD=memro -e POSTGRES_DB=memro \
  pgvector/pgvector:pg16

# 2. Run Memro API Server locally
DATABASE_URL="postgres://memro:memro@127.0.0.1:55434/memro?sslmode=disable" \
  PORT=18090 go run ./server/cmd/memro-api

# 3. Run evaluation (supports --subset, --pilot, or --all)
python bench/longmemeval/run.py --subset bench/longmemeval/subset_100.json --name ci-subset
python bench/longmemeval/judge.py bench/longmemeval/runs/ci-subset
python bench/longmemeval/report.py bench/longmemeval/runs/ci-subset

# 4. Compare against baseline with CI quality gate
python bench/longmemeval/compare.py bench/longmemeval/runs/baseline-100 bench/longmemeval/runs/ci-subset --ci-gate
```

