# Benchmark Comparison: `baseline-100-v2` -> `local-pr60`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/baseline-100-v2)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/local-pr60)
- **Server Commit**: `b0269267542bb61fa87c1b76af94496f64a63107`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`
- **Run B Cost**: $0.1746

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  75.0% |  81.2% | **+6.2%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 4 (4/0) | 3 (3/0) |
| multi-session | 27 |  77.8% |  63.0% | **-14.8%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 6 (5/1) | 10 (8/2) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  83.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 85.7% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  69.2% |  73.1% | **+3.8%** 🟢 | 96.2% / 88.5% | 96.2% / 96.2% | 8 (5/3) | 7 (4/3) |
| overall | 100 |  81.0% |  79.0% | **-2.0%** 🔴 | 99.0% / 95.0% | 99.0% / 99.0% | 19 (15/4) | 21 (16/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 9.14 | +0.00 |
| Abstention accuracy | 100.0% (6/6) | 83.3% (5/6) | -16.7% |
| Said 'no information' while evidence present | 3 | 3 | +0 |
| Search latency p50 / p95 | 170 ms / 276 ms | 1154 ms / 2129 ms | +984 ms / +1853 ms |

Paired test on flips: 2 fixed vs 4 regressed, McNemar p = 0.688 (not significant at 0.05).

## Evidence (exact answer-bearing exchanges; deterministic, no LLM)

| Metric | A | B |
|---|---|---|
| Evidence recall@15 | 93.3% | 93.3% |
| Evidence precision@15 | 10.8% | 10.8% |
| Questions with all evidence in the prompt | 85/94 | 85/94 |

Evidence gained on 0 questions, lost on 0.

## Question Flips (Total Fixed: 2, Total Regressed: 4)

### Fixed (Wrong in A -> Right in B: 2)
- **`4b24c848`** (`knowledge-update`): How many tops have I bought from H&M so far?
- **`gpt4_483dd43c`** (`temporal-reasoning`): Which show did I start watching first, 'The Crown' or 'Game of Thrones'?

### Regressed (Right in A -> Wrong in B: 4)
- **`1192316e`** (`multi-session`): What is the total time it takes I to get ready and commute to work?
- **`46a3abf7`** (`multi-session`): How many tanks do I currently have, including the one I set up for my friend's kid?
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?
- **`gpt4_372c3eed_abs`** (`multi-session`): How many years in total did I spend in formal education from high school to the completion
