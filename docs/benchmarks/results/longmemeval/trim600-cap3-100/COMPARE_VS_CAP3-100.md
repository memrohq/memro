# Benchmark Comparison: `cap3-100` -> `trim600-cap3-100`

- **Baseline Run (`A`)**: 100 questions (runs/cap3-100)
- **New Run (`B`)**: 100 questions (runs/trim600-cap3-100)
- **Server Commit**: `f71c5d99f590ec5c3f6b8dd2c5cd1a237725be5c`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "assistant_chars": 600, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`
- **Run B Cost**: $0.0685

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  81.2% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 3 (3/0) |
| multi-session | 27 |  66.7% |  70.4% | **+3.7%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 9 (9/0) | 8 (8/0) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  83.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 85.7% / 85.7% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  69.2% |  80.8% | **+11.5%** 🟢 | 88.5% / 88.5% | 96.2% / 96.2% | 8 (6/2) | 5 (3/2) |
| overall | 100 |  79.0% |  83.0% | **+4.0%** 🟢 | 95.0% / 95.0% | 99.0% / 99.0% | 21 (19/2) | 17 (15/2) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 8.49 | 8.49 | +0.00 |
| Abstention accuracy | 100.0% (6/6) | 100.0% (6/6) | +0.0% |
| Said 'no information' while evidence present | 3 | 5 | +2 |
| Search latency p50 / p95 | 126 ms / 304 ms | 98 ms / 165 ms | -27 ms / -139 ms |

Paired test on flips: 6 fixed vs 2 regressed, McNemar p = 0.289 (not significant at 0.05).

## Evidence (exact answer-bearing exchanges; deterministic, no LLM)

| Metric | A | B |
|---|---|---|
| Evidence recall@15 | 93.4% | 93.4% |
| Evidence precision@15 | 10.9% | 10.9% |
| Questions with all evidence in the prompt | 88/94 | 88/94 |

Evidence gained on 0 questions, lost on 0.

## Question Flips (Total Fixed: 6, Total Regressed: 2)

### Fixed (Wrong in A -> Right in B: 6)
- **`1192316e`** (`multi-session`): What is the total time it takes I to get ready and commute to work?
- **`28dc39ac`** (`multi-session`): How many hours have I spent playing games in total?
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`gpt4_21adecb5`** (`temporal-reasoning`): How many months passed between the completion of my undergraduate degree and the submissio
- **`gpt4_61e13b3c`** (`temporal-reasoning`): How many weeks passed between the time I sold homemade baked goods at the Farmers' Market 
- **`gpt4_a1b77f9c`** (`temporal-reasoning`): How many weeks in total do I spent on reading 'The Nightingale' and listening to 'Sapiens:

### Regressed (Right in A -> Wrong in B: 2)
- **`73d42213`** (`multi-session`): What time did I reach the clinic on Monday?
- **`ef66a6e5`** (`multi-session`): How many sports have I played competitively in the past?
