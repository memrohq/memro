# Benchmark Comparison: `main-100` -> `cap3-100`

- **Baseline Run (`A`)**: 100 questions (runs/main-100)
- **New Run (`B`)**: 100 questions (runs/cap3-100)
- **Server Commit**: `f71c5d99f590ec5c3f6b8dd2c5cd1a237725be5c`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`
- **Run B Cost**: $0.1819

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  81.2% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 3 (3/0) |
| multi-session | 27 |  63.0% |  66.7% | **+3.7%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 10 (8/2) | 9 (9/0) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  83.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 85.7% / 85.7% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  69.2% | **-3.8%** 🔴 | 88.5% / 88.5% | 96.2% / 96.2% | 7 (4/3) | 8 (6/2) |
| overall | 100 |  79.0% |  79.0% | +0.0% ⚪ | 95.0% / 95.0% | 99.0% / 99.0% | 21 (16/5) | 21 (19/2) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 8.49 | -0.65 |
| Abstention accuracy | 83.3% (5/6) | 100.0% (6/6) | +16.7% |
| Said 'no information' while evidence present | 4 | 3 | -1 |
| Search latency p50 / p95 | 168 ms / 624 ms | 126 ms / 304 ms | -42 ms / -321 ms |

Paired test on flips: 2 fixed vs 2 regressed, McNemar p = 1.000 (not significant at 0.05).

## Evidence (exact answer-bearing exchanges; deterministic, no LLM)

| Metric | A | B |
|---|---|---|
| Evidence recall@15 | 93.3% | 93.4% |
| Evidence precision@15 | 10.8% | 10.9% |
| Questions with all evidence in the prompt | 85/94 | 88/94 |

Evidence gained on 3 questions, lost on 0.

## Question Flips (Total Fixed: 2, Total Regressed: 2)

### Fixed (Wrong in A -> Right in B: 2)
- **`8e91e7d9`** (`multi-session`): What is the total number of siblings I have?
- **`gpt4_372c3eed_abs`** (`multi-session`): How many years in total did I spend in formal education from high school to the completion

### Regressed (Right in A -> Wrong in B: 2)
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`gpt4_61e13b3c`** (`temporal-reasoning`): How many weeks passed between the time I sold homemade baked goods at the Farmers' Market 
