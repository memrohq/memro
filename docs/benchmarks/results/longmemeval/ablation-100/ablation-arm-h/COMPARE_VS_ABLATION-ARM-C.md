# Benchmark Comparison: `ablation-arm-c` -> `ablation-arm-h`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-c)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-h)
- **Server Commit**: `2e2adaf390efd7974bc37a146a321c427b3a7875`
- **Run B Cost**: $0.0039

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  81.2% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 3 (3/0) |
| multi-session | 27 |  55.6% |  74.1% | **+18.5%** 🟢 | 100.0% / 96.3% | 100.0% / 100.0% | 12 (8/4) | 7 (7/0) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  66.7% |  66.7% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 2 (2/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 96.2% / 96.2% | 96.2% / 96.2% | 7 (3/4) | 7 (5/2) |
| overall | 100 |  76.0% |  81.0% | **+5.0%** 🟢 | 99.0% / 98.0% | 99.0% / 99.0% | 24 (16/8) | 19 (17/2) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 7.51 | 7.42 | -0.09 |
| Abstention accuracy | 83.3% (5/6) | 100.0% (6/6) | +16.7% |
| Said 'no information' while evidence present | 5 | 5 | +0 |
| Search latency p50 / p95 | 171 ms / 662 ms | 167 ms / 320 ms | -4 ms / -342 ms |

## Question Flips (Total Fixed: 8, Total Regressed: 3)

### Fixed (Wrong in A -> Right in B: 8)
- **`1192316e`** (`multi-session`): What is the total time it takes I to get ready and commute to work?
- **`129d1232`** (`multi-session`): How much money did I raise in total through all the charity events I participated in?
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`73d42213`** (`multi-session`): What time did I reach the clinic on Monday?
- **`8e91e7d9`** (`multi-session`): What is the total number of siblings I have?
- **`b6025781`** (`single-session-preference`): I'm planning my meal prep next week, any suggestions for new recipes?
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?
- **`gpt4_372c3eed_abs`** (`multi-session`): How many years in total did I spend in formal education from high school to the completion

### Regressed (Right in A -> Wrong in B: 3)
- **`46a3abf7`** (`multi-session`): How many tanks do I currently have, including the one I set up for my friend's kid?
- **`88432d0a`** (`multi-session`): How many times did I bake something in the past two weeks?
- **`caf03d32`** (`single-session-preference`): I've been struggling with my slow cooker recipes. Any advice on getting better results?
