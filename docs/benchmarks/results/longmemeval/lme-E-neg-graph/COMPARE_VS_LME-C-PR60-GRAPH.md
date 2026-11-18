# Benchmark Comparison: `lme-C-pr60-graph` -> `lme-E-neg-graph`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/lme-C-pr60-graph)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/lme-E-neg-graph)
- **Server Commit**: `894d14192c858f758737b0ce009377dec1a76c70`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`
- **Run B Cost**: $0.1713

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  75.0% | **-12.5%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 4 (4/0) |
| multi-session | 27 |  66.7% |  63.0% | **-3.7%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 9 (7/2) | 10 (8/2) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  83.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  69.2% |  73.1% | **+3.8%** 🟢 | 96.2% / 96.2% | 96.2% / 96.2% | 8 (5/3) | 7 (4/3) |
| overall | 100 |  80.0% |  78.0% | **-2.0%** 🔴 | 99.0% / 99.0% | 99.0% / 99.0% | 20 (15/5) | 22 (17/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 9.14 | +0.00 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 2 | 4 | +2 |
| Search latency p50 / p95 | 849 ms / 2366 ms | 1122 ms / 4478 ms | +274 ms / +2112 ms |

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
- **`88432d0a`** (`multi-session`): How many times did I bake something in the past two weeks?
- **`gpt4_483dd43c`** (`temporal-reasoning`): Which show did I start watching first, 'The Crown' or 'Game of Thrones'?

### Regressed (Right in A -> Wrong in B: 4)
- **`1192316e`** (`multi-session`): What is the total time it takes I to get ready and commute to work?
- **`2133c1b5`** (`knowledge-update`): How long have I been living in my current apartment in Harajuku?
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`ba61f0b9`** (`knowledge-update`): How many women are on the team led by my former manager Rachel?
