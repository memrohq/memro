# Benchmark Comparison: `ablation-arm-a` -> `ablation-arm-d`

- **Baseline Run (`A`)**: 100 questions (runs/ablation-100/ablation-arm-a)
- **New Run (`B`)**: 100 questions (runs/ablation-100/ablation-arm-d)
- **Server Commit**: `bfd4a7347f2e8a88b7ca9bdd562206045e0c03f4`
- **Run B Cost**: $0.0038

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  75.0% | **-12.5%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 4 (4/0) |
| multi-session | 27 |  66.7% |  55.6% | **-11.1%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 9 (7/2) | 12 (7/5) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  66.7% |  83.3% | **+16.7%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 96.2% / 96.2% | 96.2% / 96.2% | 7 (4/3) | 7 (3/4) |
| overall | 100 |  80.0% |  76.0% | **-4.0%** 🔴 | 99.0% / 99.0% | 99.0% / 99.0% | 20 (15/5) | 24 (15/9) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 7.60 | -1.54 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 4 | 6 | +2 |
| Search latency p50 / p95 | 163 ms / 356 ms | 164 ms / 511 ms | +2 ms / +154 ms |

## Question Flips (Total Fixed: 1, Total Regressed: 5)

### Fixed (Wrong in A -> Right in B: 1)
- **`b6025781`** (`single-session-preference`): I'm planning my meal prep next week, any suggestions for new recipes?

### Regressed (Right in A -> Wrong in B: 5)
- **`129d1232`** (`multi-session`): How much money did I raise in total through all the charity events I participated in?
- **`2133c1b5`** (`knowledge-update`): How long have I been living in my current apartment in Harajuku?
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`ba61f0b9`** (`knowledge-update`): How many women are on the team led by my former manager Rachel?
- **`ef66a6e5`** (`multi-session`): How many sports have I played competitively in the past?
