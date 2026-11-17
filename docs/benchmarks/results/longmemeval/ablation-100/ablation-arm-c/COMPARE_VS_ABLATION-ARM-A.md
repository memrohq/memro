# Benchmark Comparison: `ablation-arm-a` -> `ablation-arm-c`

- **Baseline Run (`A`)**: 100 questions (runs/ablation-100/ablation-arm-a)
- **New Run (`B`)**: 100 questions (runs/ablation-100/ablation-arm-c)
- **Server Commit**: `bfd4a7347f2e8a88b7ca9bdd562206045e0c03f4`
- **Run B Cost**: $0.0038

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  81.2% | **-6.2%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 3 (3/0) |
| multi-session | 27 |  66.7% |  55.6% | **-11.1%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 9 (7/2) | 12 (8/4) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  66.7% |  66.7% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 2 (2/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 96.2% / 96.2% | 96.2% / 96.2% | 7 (4/3) | 7 (3/4) |
| overall | 100 |  80.0% |  76.0% | **-4.0%** 🔴 | 99.0% / 99.0% | 99.0% / 99.0% | 20 (15/5) | 24 (16/8) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 7.51 | -1.63 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 4 | 5 | +1 |
| Search latency p50 / p95 | 163 ms / 356 ms | 171 ms / 662 ms | +8 ms / +305 ms |

## Question Flips (Total Fixed: 1, Total Regressed: 5)

### Fixed (Wrong in A -> Right in B: 1)
- **`88432d0a`** (`multi-session`): How many times did I bake something in the past two weeks?

### Regressed (Right in A -> Wrong in B: 5)
- **`129d1232`** (`multi-session`): How much money did I raise in total through all the charity events I participated in?
- **`2133c1b5`** (`knowledge-update`): How long have I been living in my current apartment in Harajuku?
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`73d42213`** (`multi-session`): What time did I reach the clinic on Monday?
- **`ef66a6e5`** (`multi-session`): How many sports have I played competitively in the past?
