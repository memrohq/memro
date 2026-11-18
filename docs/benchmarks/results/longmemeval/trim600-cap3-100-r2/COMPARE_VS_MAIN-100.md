# Benchmark Comparison: `main-100` -> `trim600-cap3-100-r2`

- **Baseline Run (`A`)**: 100 questions (runs/main-100)
- **New Run (`B`)**: 100 questions (runs/trim600-cap3-100-r2)
- **Server Commit**: `f71c5d99f590ec5c3f6b8dd2c5cd1a237725be5c`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "assistant_chars": 600, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`
- **Run B Cost**: $0.0684

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  87.5% | **+6.2%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 2 (2/0) |
| multi-session | 27 |  63.0% |  63.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 10 (8/2) | 10 (10/0) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  66.7% | **-16.7%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 2 (2/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 85.7% / 85.7% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  76.9% | **+3.8%** 🟢 | 88.5% / 88.5% | 96.2% / 96.2% | 7 (4/3) | 6 (4/2) |
| overall | 100 |  79.0% |  80.0% | **+1.0%** 🟢 | 95.0% / 95.0% | 99.0% / 99.0% | 21 (16/5) | 20 (18/2) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 8.49 | -0.65 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 4 | 5 | +1 |
| Search latency p50 / p95 | 168 ms / 624 ms | 120 ms / 250 ms | -47 ms / -374 ms |

Paired test on flips: 6 fixed vs 5 regressed, McNemar p = 1.000 (not significant at 0.05).

## Evidence (exact answer-bearing exchanges; deterministic, no LLM)

| Metric | A | B |
|---|---|---|
| Evidence recall@15 | 93.3% | 93.4% |
| Evidence precision@15 | 10.8% | 10.9% |
| Questions with all evidence in the prompt | 85/94 | 88/94 |

Evidence gained on 3 questions, lost on 0.

## Question Flips (Total Fixed: 6, Total Regressed: 5)

### Fixed (Wrong in A -> Right in B: 6)
- **`1192316e`** (`multi-session`): What is the total time it takes I to get ready and commute to work?
- **`184da446`** (`knowledge-update`): How many pages of 'A Short History of Nearly Everything' have I read so far?
- **`28dc39ac`** (`multi-session`): How many hours have I spent playing games in total?
- **`8e91e7d9`** (`multi-session`): What is the total number of siblings I have?
- **`gpt4_21adecb5`** (`temporal-reasoning`): How many months passed between the completion of my undergraduate degree and the submissio
- **`gpt4_a1b77f9c`** (`temporal-reasoning`): How many weeks in total do I spent on reading 'The Nightingale' and listening to 'Sapiens:

### Regressed (Right in A -> Wrong in B: 5)
- **`73d42213`** (`multi-session`): What time did I reach the clinic on Monday?
- **`a89d7624`** (`single-session-preference`): I'm planning a trip to Denver soon. Any suggestions on what to do there?
- **`ba358f49`** (`multi-session`): How many years will I be when my friend Rachel gets married?
- **`ef66a6e5`** (`multi-session`): How many sports have I played competitively in the past?
- **`gpt4_9a159967`** (`temporal-reasoning`): Which airline did I fly with the most in March and April?
