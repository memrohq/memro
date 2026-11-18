# Benchmark Comparison: `ablation-arm-b` -> `ablation-arm-c`

- **Baseline Run (`A`)**: 100 questions (runs/ablation-100/ablation-arm-b)
- **New Run (`B`)**: 100 questions (runs/ablation-100/ablation-arm-c)
- **Server Commit**: `bfd4a7347f2e8a88b7ca9bdd562206045e0c03f4`
- **Run B Cost**: $0.0038

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  81.2% | **-6.2%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 3 (3/0) |
| multi-session | 27 |  59.3% |  55.6% | **-3.7%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 11 (7/4) | 12 (8/4) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  66.7% | **-16.7%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 2 (2/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  69.2% |  73.1% | **+3.8%** 🟢 | 96.2% / 96.2% | 96.2% / 96.2% | 8 (4/4) | 7 (3/4) |
| overall | 100 |  78.0% |  76.0% | **-2.0%** 🔴 | 99.0% / 99.0% | 99.0% / 99.0% | 22 (14/8) | 24 (16/8) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 7.63 | 7.51 | -0.12 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 3 | 5 | +2 |
| Search latency p50 / p95 | 166 ms / 808 ms | 171 ms / 662 ms | +5 ms / -146 ms |

## Question Flips (Total Fixed: 2, Total Regressed: 4)

### Fixed (Wrong in A -> Right in B: 2)
- **`88432d0a`** (`multi-session`): How many times did I bake something in the past two weeks?
- **`gpt4_c27434e8`** (`temporal-reasoning`): Which project did I start first, the Ferrari model or the Japanese Zero fighter plane mode

### Regressed (Right in A -> Wrong in B: 4)
- **`2133c1b5`** (`knowledge-update`): How long have I been living in my current apartment in Harajuku?
- **`2b8f3739`** (`multi-session`): What is the total amount of money I earned from selling my products at the markets?
- **`73d42213`** (`multi-session`): What time did I reach the clinic on Monday?
- **`b6025781`** (`single-session-preference`): I'm planning my meal prep next week, any suggestions for new recipes?
