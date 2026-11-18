# Benchmark Comparison: `ablation-arm-a` -> `ablation-arm-b`

- **Baseline Run (`A`)**: 100 questions (runs/ablation-100/ablation-arm-a)
- **New Run (`B`)**: 100 questions (runs/ablation-100/ablation-arm-b)
- **Server Commit**: `bfd4a7347f2e8a88b7ca9bdd562206045e0c03f4`
- **Run B Cost**: $0.0038

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  87.5% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 2 (2/0) |
| multi-session | 27 |  66.7% |  59.3% | **-7.4%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 9 (7/2) | 11 (7/4) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  66.7% |  83.3% | **+16.7%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  69.2% | **-3.8%** 🔴 | 96.2% / 96.2% | 96.2% / 96.2% | 7 (4/3) | 8 (4/4) |
| overall | 100 |  80.0% |  78.0% | **-2.0%** 🔴 | 99.0% / 99.0% | 99.0% / 99.0% | 20 (15/5) | 22 (14/8) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 7.63 | -1.51 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 4 | 3 | -1 |
| Search latency p50 / p95 | 163 ms / 356 ms | 166 ms / 808 ms | +3 ms / +452 ms |

## Question Flips (Total Fixed: 1, Total Regressed: 3)

### Fixed (Wrong in A -> Right in B: 1)
- **`b6025781`** (`single-session-preference`): I'm planning my meal prep next week, any suggestions for new recipes?

### Regressed (Right in A -> Wrong in B: 3)
- **`129d1232`** (`multi-session`): How much money did I raise in total through all the charity events I participated in?
- **`ef66a6e5`** (`multi-session`): How many sports have I played competitively in the past?
- **`gpt4_c27434e8`** (`temporal-reasoning`): Which project did I start first, the Ferrari model or the Japanese Zero fighter plane mode
