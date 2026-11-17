# Benchmark Comparison: `lme-A-main` -> `lme-B-pr60`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/lme-A-main)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/lme-B-pr60)
- **Server Commit**: `894d14192c858f758737b0ce009377dec1a76c70`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`
- **Run B Cost**: $0.1746

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  87.5% | **+6.2%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 2 (2/0) |
| multi-session | 27 |  63.0% |  55.6% | **-7.4%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 10 (9/1) | 12 (10/2) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  83.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 85.7% / 85.7% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 88.5% / 88.5% | 96.2% / 96.2% | 7 (4/3) | 7 (4/3) |
| overall | 100 |  79.0% |  78.0% | **-1.0%** 🔴 | 95.0% / 95.0% | 99.0% / 99.0% | 21 (17/4) | 22 (17/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.13 | 9.14 | +0.01 |
| Abstention accuracy | 100.0% (6/6) | 83.3% (5/6) | -16.7% |
| Said 'no information' while evidence present | 4 | 6 | +2 |
| Search latency p50 / p95 | 1037 ms / 1940 ms | 1004 ms / 1748 ms | -33 ms / -192 ms |

Paired test on flips: 1 fixed vs 2 regressed, McNemar p = 1.000 (not significant at 0.05).

## Evidence (exact answer-bearing exchanges; deterministic, no LLM)

| Metric | A | B |
|---|---|---|
| Evidence recall@15 | 93.3% | 93.3% |
| Evidence precision@15 | 10.8% | 10.8% |
| Questions with all evidence in the prompt | 85/94 | 85/94 |

Evidence gained on 0 questions, lost on 0.

## Question Flips (Total Fixed: 1, Total Regressed: 2)

### Fixed (Wrong in A -> Right in B: 1)
- **`184da446`** (`knowledge-update`): How many pages of 'A Short History of Nearly Everything' have I read so far?

### Regressed (Right in A -> Wrong in B: 2)
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`gpt4_372c3eed_abs`** (`multi-session`): How many years in total did I spend in formal education from high school to the completion
