# Benchmark Comparison: `ablation-arm-c` -> `ablation-arm-e`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-c)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-e)
- **Server Commit**: `4b785c6023db90638e85324ce9a8524092e78687`
- **Run B Cost**: $0.0039

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  81.2% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 3 (3/0) |
| multi-session | 27 |  55.6% |  70.4% | **+14.8%** 🟢 | 100.0% / 96.3% | 100.0% / 100.0% | 12 (8/4) | 8 (6/2) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  66.7% |  83.3% | **+16.7%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 92.9% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 96.2% / 88.5% | 96.2% / 96.2% | 7 (3/4) | 7 (4/3) |
| overall | 100 |  76.0% |  81.0% | **+5.0%** 🟢 | 99.0% / 95.0% | 99.0% / 99.0% | 24 (16/8) | 19 (14/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 7.51 | 9.14 | +1.63 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 5 | 4 | -1 |
| Search latency p50 / p95 | 171 ms / 662 ms | 203 ms / 428 ms | +32 ms / -234 ms |

## Question Flips (Total Fixed: 7, Total Regressed: 2)

### Fixed (Wrong in A -> Right in B: 7)
- **`129d1232`** (`multi-session`): How much money did I raise in total through all the charity events I participated in?
- **`2133c1b5`** (`knowledge-update`): How long have I been living in my current apartment in Harajuku?
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`73d42213`** (`multi-session`): What time did I reach the clinic on Monday?
- **`b6025781`** (`single-session-preference`): I'm planning my meal prep next week, any suggestions for new recipes?
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?
- **`ef66a6e5`** (`multi-session`): How many sports have I played competitively in the past?

### Regressed (Right in A -> Wrong in B: 2)
- **`88432d0a`** (`multi-session`): How many times did I bake something in the past two weeks?
- **`ba61f0b9`** (`knowledge-update`): How many women are on the team led by my former manager Rachel?
