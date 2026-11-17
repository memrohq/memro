# Benchmark Comparison: `ablation-arm-a` -> `ablation-arm-g`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-a)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-g)
- **Server Commit**: `2e2adaf390efd7974bc37a146a321c427b3a7875`
- **Run B Cost**: $0.0038

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  87.5% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 2 (2/0) |
| multi-session | 27 |  66.7% |  63.0% | **-3.7%** 🔴 | 100.0% / 96.3% | 100.0% / 100.0% | 9 (7/2) | 10 (10/0) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  66.7% |  83.3% | **+16.7%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 92.9% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 96.2% / 88.5% | 96.2% / 96.2% | 7 (4/3) | 7 (5/2) |
| overall | 100 |  80.0% |  80.0% | +0.0% ⚪ | 99.0% / 95.0% | 99.0% / 99.0% | 20 (15/5) | 20 (18/2) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 8.21 | -0.93 |
| Abstention accuracy | 83.3% (5/6) | 100.0% (6/6) | +16.7% |
| Said 'no information' while evidence present | 4 | 5 | +1 |
| Search latency p50 / p95 | 163 ms / 356 ms | 169 ms / 459 ms | +6 ms / +103 ms |

## Question Flips (Total Fixed: 4, Total Regressed: 4)

### Fixed (Wrong in A -> Right in B: 4)
- **`8e91e7d9`** (`multi-session`): What is the total number of siblings I have?
- **`b6025781`** (`single-session-preference`): I'm planning my meal prep next week, any suggestions for new recipes?
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?
- **`gpt4_372c3eed_abs`** (`multi-session`): How many years in total did I spend in formal education from high school to the completion

### Regressed (Right in A -> Wrong in B: 4)
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`46a3abf7`** (`multi-session`): How many tanks do I currently have, including the one I set up for my friend's kid?
- **`73d42213`** (`multi-session`): What time did I reach the clinic on Monday?
- **`ef66a6e5`** (`multi-session`): How many sports have I played competitively in the past?
