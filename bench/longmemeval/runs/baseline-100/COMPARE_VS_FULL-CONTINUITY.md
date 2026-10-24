# Benchmark Comparison: `full-continuity` -> `baseline-100`

- **Baseline Run (`A`)**: 500 questions (bench/longmemeval/runs/full-continuity)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/baseline-100)
- **Server Commit**: `5d0ec7a38220c799bba65f64df3a822aef56f4d5`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000}`
- **Run B Cost**: $0.1714

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  82.1% |  81.2% | **-0.8%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 14 (14/0) | 3 (3/0) |
| multi-session | 27 |  67.7% |  59.3% | **-8.4%** 🔴 | 97.7% / 100.0% | 100.0% / 100.0% | 43 (37/6) | 11 (9/2) |
| single-session-assistant | 11 |  96.4% | 100.0% | **+3.6%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 0 (0/0) |
| single-session-preference | 6 |  70.0% |  83.3% | **+13.3%** 🟢 | 90.0% / 100.0% | 96.7% / 100.0% | 9 (8/1) | 1 (1/0) |
| single-session-user | 14 |  95.7% | 100.0% | **+4.3%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 0 (0/0) |
| temporal-reasoning | 26 |  69.2% |  73.1% | **+3.9%** 🟢 | 95.5% / 96.2% | 98.5% / 96.2% | 41 (30/11) | 7 (4/3) |
| overall | 100 |  77.6% |  78.0% | **+0.4%** 🟢 | 97.6% / 99.0% | 99.4% / 99.0% | 112 (94/18) | 22 (17/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 8.62 | 9.14 | +0.52 |
| Abstention accuracy | 73.3% (22/30) | 83.3% (5/6) | +10.0% |
| Said 'no information' while evidence present | 15 | 7 | -8 |
| Search latency p50 / p95 | 401 ms / 1074 ms | 229 ms / 639 ms | -172 ms / -435 ms |

## Question Flips (Total Fixed: 4, Total Regressed: 4)

### Fixed (Wrong in A -> Right in B: 4)
- **`a89d7624`** (`single-session-preference`): I'm planning a trip to Denver soon. Any suggestions on what to do there?
- **`ba358f49`** (`multi-session`): How many years will I be when my friend Rachel gets married?
- **`ef66a6e5`** (`multi-session`): How many sports have I played competitively in the past?
- **`gpt4_483dd43c`** (`temporal-reasoning`): Which show did I start watching first, 'The Crown' or 'Game of Thrones'?

### Regressed (Right in A -> Wrong in B: 4)
- **`73d42213`** (`multi-session`): What time did I reach the clinic on Monday?
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?
- **`e3038f8c`** (`multi-session`): How many rare items do I have in total?
- **`gpt4_372c3eed_abs`** (`multi-session`): How many years in total did I spend in formal education from high school to the completion
