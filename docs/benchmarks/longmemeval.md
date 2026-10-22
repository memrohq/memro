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
- The answer generation step receives TOP_K=15 (`run.py:28`).
- **94 of 112 failed questions (84%) had every gold session among the 15 memories given to the answering model**.
  - At top-5 cutoff: 61 of the 84 failed questions with available gold evidence had every gold session in the top 5.
  - At top-10 cutoff: 84 of 112 failed questions had every gold session in the top 10.
- This demonstrates that retrieval is rarely the failure cause; the bottleneck is primarily synthesis: the answering model failing to aggregate multiple disjoint session turns or compute date differences across conversations.

---

## 5. How to Reproduce

The complete benchmark harness, dataset loaders, and run outputs are included in [`bench/longmemeval/`](../../bench/longmemeval/):

```bash
# 1. Start PostgreSQL with pgvector and Memro API
docker compose up -d

# 2. Navigate to benchmark directory
cd bench/longmemeval

# 3. Run pilot or full evaluation
python run.py --pilot   # Fast 12-question pilot run
python run.py --all     # Full 500-question run (supports resume)

# 4. Grade with LLM judge and view report
python judge.py runs/full-continuity
python report.py runs/full-continuity
```
