# 2026-11-15: Baselines — no memory, plain RAG and Memro

- **Date**: 2026-11-15
- **Server commit**: `c5e9c83` (main with PR #67) for all three arms. Each run's `meta.json` records the harness checkout, not the server binary.
- **Run directories**: `bench/longmemeval/runs/arm-{none,rag,memro}-100`
- **Dataset / subset**: LongMemEval_S, `bench/longmemeval/subset_100.json`
- **Engine options**: as in each `meta.json`. All arms: top-15, `gpt-4o-mini` answers and grades, `text-embedding-3-small`, same prompt.
  - `none`: no retrieval; the prompt says "(nothing found)".
  - `rag`: semantic search only, no session expansion, no conflict handling.
  - `memro`: hybrid search (vectors + full text, RRF), 3-session expansion, conflict resolution on.
- **Cost**: $0.32 for answers and grading (600 `gpt-4o-mini` calls). Embeddings are called by the server and not in the ledger.

## 1. Goal & hypothesis

An external review asked what memory adds over no memory and over naive
RAG. Hypotheses: memory beats no memory by a wide margin; Memro beats plain
semantic top-k.

## 2. Results

| Arm | Accuracy | 95% CI (Wilson) | Abstention | Recall@15 | Evidence exchanges in prompt | Search p50 / p95 |
|---|---|---|---|---|---|---|
| none | 6% | 2.8–12.5% | 6/6 | — | 0/176 | — |
| rag | 81% | 72.2–87.5% | 5/6 | 98.9% | 164/176 | 1,089 / 3,643 ms |
| memro | 81% | 72.2–87.5% | 6/6 | 98.9% | 167/176 | 757 / 2,341 ms |

| Type | n | none | rag | memro |
|---|---|---|---|---|
| knowledge-update | 16 | 0 | 14 | 12 |
| multi-session | 27 | 2 | 19 | 20 |
| single-session-assistant | 11 | 0 | 11 | 11 |
| single-session-preference | 6 | 0 | 4 | 5 |
| single-session-user | 14 | 4 | 14 | 14 |
| temporal-reasoning | 26 | 0 | 19 | 19 |

Paired (McNemar): rag vs none, 76 questions only rag got right and 1 only
none, p < 0.0001. memro vs rag, 4 each way, p = 1.0. The six questions `none`
got right are abstentions ("I don't know" is correct).

Search latency includes the query embedding call; the two arms ran one after
the other against the same local server, so they are comparable to each other,
not to production.

## 3. What this says

- **Memory is the whole score.** Without it the model answers only abstention
  questions.
- **On this subset Memro's extras show no gain over plain semantic top-15.**
  Keyword fusion, session expansion and conflict demotion put three more
  evidence exchanges into prompts (167 vs 164 of 176) but did not change the
  number of correct answers. Knowledge updates were 12 vs 14, not significant
  (n = 16), and the opposite of what conflict resolution is meant to do;
  those two questions are the first to look at.
- With ±8-point intervals at n = 100 and answer-model noise of a few points
  between identical runs (2026-11-15 evolution post), a small real difference
  either way is not ruled out.

## 4. Next

1. Look at the two knowledge-update questions where rag was right and memro
   wrong: `184da446` (memro answered 20 pages, gold 220, so conflict handling
   may have kept the older count) and `2133c1b5`.
2. Run both arms on all 500 questions, twice each.
3. Report the comparison in the LongMemEval paper (MR-2026-01) as measured.
