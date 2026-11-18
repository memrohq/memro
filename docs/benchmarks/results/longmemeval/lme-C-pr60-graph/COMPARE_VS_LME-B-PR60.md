# Benchmark Comparison: `lme-B-pr60` -> `lme-C-pr60-graph`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/lme-B-pr60)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/lme-C-pr60-graph)
- **Server Commit**: `894d14192c858f758737b0ce009377dec1a76c70`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`
- **Run B Cost**: $0.1735

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  87.5% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 2 (2/0) |
| multi-session | 27 |  55.6% |  66.7% | **+11.1%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 12 (10/2) | 9 (7/2) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  83.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 85.7% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  69.2% | **-3.8%** 🔴 | 88.5% / 96.2% | 96.2% / 96.2% | 7 (4/3) | 8 (5/3) |
| overall | 100 |  78.0% |  80.0% | **+2.0%** 🟢 | 95.0% / 99.0% | 99.0% / 99.0% | 22 (17/5) | 20 (15/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 9.14 | +0.00 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 6 | 2 | -4 |
| Search latency p50 / p95 | 1004 ms / 1748 ms | 849 ms / 2366 ms | -156 ms / +618 ms |

Paired test on flips: 3 fixed vs 1 regressed, McNemar p = 0.625 (not significant at 0.05).

## Evidence (exact answer-bearing exchanges; deterministic, no LLM)

| Metric | A | B |
|---|---|---|
| Evidence recall@15 | 93.3% | 93.3% |
| Evidence precision@15 | 10.8% | 10.8% |
| Questions with all evidence in the prompt | 85/94 | 85/94 |

Evidence gained on 0 questions, lost on 0.

## Question Flips (Total Fixed: 3, Total Regressed: 1)

### Fixed (Wrong in A -> Right in B: 3)
- **`1192316e`** (`multi-session`): What is the total time it takes I to get ready and commute to work?
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`46a3abf7`** (`multi-session`): How many tanks do I currently have, including the one I set up for my friend's kid?

### Regressed (Right in A -> Wrong in B: 1)
- **`gpt4_483dd43c`** (`temporal-reasoning`): Which show did I start watching first, 'The Crown' or 'Game of Thrones'?
