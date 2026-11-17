# Benchmark Comparison: `lme-D-neg` -> `lme-F-words`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/lme-D-neg)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/lme-F-words)
- **Server Commit**: `894d14192c858f758737b0ce009377dec1a76c70`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`
- **Run B Cost**: $0.1713

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  75.0% | **-6.2%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 4 (4/0) |
| multi-session | 27 |  70.4% |  59.3% | **-11.1%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 8 (7/1) | 11 (9/2) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  83.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  69.2% |  73.1% | **+3.8%** 🟢 | 96.2% / 96.2% | 96.2% / 96.2% | 8 (5/3) | 7 (4/3) |
| overall | 100 |  80.0% |  77.0% | **-3.0%** 🔴 | 99.0% / 99.0% | 99.0% / 99.0% | 20 (16/4) | 23 (18/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 9.14 | +0.00 |
| Abstention accuracy | 100.0% (6/6) | 83.3% (5/6) | -16.7% |
| Said 'no information' while evidence present | 2 | 4 | +2 |
| Search latency p50 / p95 | 1298 ms / 2791 ms | 1257 ms / 3405 ms | -41 ms / +614 ms |

Paired test on flips: 1 fixed vs 4 regressed, McNemar p = 0.375 (not significant at 0.05).

## Evidence (exact answer-bearing exchanges; deterministic, no LLM)

| Metric | A | B |
|---|---|---|
| Evidence recall@15 | 93.3% | 93.3% |
| Evidence precision@15 | 10.8% | 10.8% |
| Questions with all evidence in the prompt | 85/94 | 85/94 |

Evidence gained on 0 questions, lost on 0.

## Question Flips (Total Fixed: 1, Total Regressed: 4)

### Fixed (Wrong in A -> Right in B: 1)
- **`gpt4_483dd43c`** (`temporal-reasoning`): Which show did I start watching first, 'The Crown' or 'Game of Thrones'?

### Regressed (Right in A -> Wrong in B: 4)
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`ba61f0b9`** (`knowledge-update`): How many women are on the team led by my former manager Rachel?
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?
- **`gpt4_372c3eed_abs`** (`multi-session`): How many years in total did I spend in formal education from high school to the completion
