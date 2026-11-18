# Benchmark Comparison: `lme-B-pr60` -> `lme-D-neg`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/lme-B-pr60)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/lme-D-neg)
- **Server Commit**: `894d14192c858f758737b0ce009377dec1a76c70`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`
- **Run B Cost**: $0.1716

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  81.2% | **-6.2%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 3 (3/0) |
| multi-session | 27 |  55.6% |  70.4% | **+14.8%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 12 (10/2) | 8 (7/1) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  83.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 85.7% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  69.2% | **-3.8%** 🔴 | 88.5% / 96.2% | 96.2% / 96.2% | 7 (4/3) | 8 (5/3) |
| overall | 100 |  78.0% |  80.0% | **+2.0%** 🟢 | 95.0% / 99.0% | 99.0% / 99.0% | 22 (17/5) | 20 (16/4) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 9.14 | +0.00 |
| Abstention accuracy | 83.3% (5/6) | 100.0% (6/6) | +16.7% |
| Said 'no information' while evidence present | 6 | 2 | -4 |
| Search latency p50 / p95 | 1004 ms / 1748 ms | 1298 ms / 2791 ms | +294 ms / +1043 ms |

Paired test on flips: 4 fixed vs 2 regressed, McNemar p = 0.688 (not significant at 0.05).

## Evidence (exact answer-bearing exchanges; deterministic, no LLM)

| Metric | A | B |
|---|---|---|
| Evidence recall@15 | 93.3% | 93.3% |
| Evidence precision@15 | 10.8% | 10.8% |
| Questions with all evidence in the prompt | 85/94 | 85/94 |

Evidence gained on 0 questions, lost on 0.

## Question Flips (Total Fixed: 4, Total Regressed: 2)

### Fixed (Wrong in A -> Right in B: 4)
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`46a3abf7`** (`multi-session`): How many tanks do I currently have, including the one I set up for my friend's kid?
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?
- **`gpt4_372c3eed_abs`** (`multi-session`): How many years in total did I spend in formal education from high school to the completion

### Regressed (Right in A -> Wrong in B: 2)
- **`2133c1b5`** (`knowledge-update`): How long have I been living in my current apartment in Harajuku?
- **`gpt4_483dd43c`** (`temporal-reasoning`): Which show did I start watching first, 'The Crown' or 'Game of Thrones'?
