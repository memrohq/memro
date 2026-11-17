# Benchmark Comparison: `ablation-arm-a` -> `ablation-arm-h`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-a)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-h)
- **Server Commit**: `2e2adaf390efd7974bc37a146a321c427b3a7875`
- **Run B Cost**: $0.0039

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  81.2% | **-6.2%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 3 (3/0) |
| multi-session | 27 |  66.7% |  74.1% | **+7.4%** 🟢 | 100.0% / 96.3% | 100.0% / 100.0% | 9 (7/2) | 7 (7/0) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  66.7% |  66.7% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 2 (2/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 96.2% / 96.2% | 96.2% / 96.2% | 7 (4/3) | 7 (5/2) |
| overall | 100 |  80.0% |  81.0% | **+1.0%** 🟢 | 99.0% / 98.0% | 99.0% / 99.0% | 20 (15/5) | 19 (17/2) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 7.42 | -1.72 |
| Abstention accuracy | 83.3% (5/6) | 100.0% (6/6) | +16.7% |
| Said 'no information' while evidence present | 4 | 5 | +1 |
| Search latency p50 / p95 | 163 ms / 356 ms | 167 ms / 320 ms | +4 ms / -37 ms |

## Question Flips (Total Fixed: 5, Total Regressed: 4)

### Fixed (Wrong in A -> Right in B: 5)
- **`1192316e`** (`multi-session`): What is the total time it takes I to get ready and commute to work?
- **`8e91e7d9`** (`multi-session`): What is the total number of siblings I have?
- **`b6025781`** (`single-session-preference`): I'm planning my meal prep next week, any suggestions for new recipes?
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?
- **`gpt4_372c3eed_abs`** (`multi-session`): How many years in total did I spend in formal education from high school to the completion

### Regressed (Right in A -> Wrong in B: 4)
- **`2133c1b5`** (`knowledge-update`): How long have I been living in my current apartment in Harajuku?
- **`46a3abf7`** (`multi-session`): How many tanks do I currently have, including the one I set up for my friend's kid?
- **`caf03d32`** (`single-session-preference`): I've been struggling with my slow cooker recipes. Any advice on getting better results?
- **`ef66a6e5`** (`multi-session`): How many sports have I played competitively in the past?
