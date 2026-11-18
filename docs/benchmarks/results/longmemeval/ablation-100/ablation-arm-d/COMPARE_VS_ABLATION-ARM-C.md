# Benchmark Comparison: `ablation-arm-c` -> `ablation-arm-d`

- **Baseline Run (`A`)**: 100 questions (runs/ablation-100/ablation-arm-c)
- **New Run (`B`)**: 100 questions (runs/ablation-100/ablation-arm-d)
- **Server Commit**: `bfd4a7347f2e8a88b7ca9bdd562206045e0c03f4`
- **Run B Cost**: $0.0038

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  75.0% | **-6.2%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 4 (4/0) |
| multi-session | 27 |  55.6% |  55.6% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 12 (8/4) | 12 (7/5) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  66.7% |  83.3% | **+16.7%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 96.2% / 96.2% | 96.2% / 96.2% | 7 (3/4) | 7 (3/4) |
| overall | 100 |  76.0% |  76.0% | +0.0% ⚪ | 99.0% / 99.0% | 99.0% / 99.0% | 24 (16/8) | 24 (15/9) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 7.51 | 7.60 | +0.09 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 5 | 6 | +1 |
| Search latency p50 / p95 | 171 ms / 662 ms | 164 ms / 511 ms | -6 ms / -151 ms |

## Question Flips (Total Fixed: 2, Total Regressed: 2)

### Fixed (Wrong in A -> Right in B: 2)
- **`73d42213`** (`multi-session`): What time did I reach the clinic on Monday?
- **`b6025781`** (`single-session-preference`): I'm planning my meal prep next week, any suggestions for new recipes?

### Regressed (Right in A -> Wrong in B: 2)
- **`88432d0a`** (`multi-session`): How many times did I bake something in the past two weeks?
- **`ba61f0b9`** (`knowledge-update`): How many women are on the team led by my former manager Rachel?
