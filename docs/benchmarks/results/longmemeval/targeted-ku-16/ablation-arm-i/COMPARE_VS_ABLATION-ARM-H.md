# Benchmark Comparison: `ablation-arm-h` -> `ablation-arm-i`

- **Baseline Run (`A`)**: 16 questions (bench/longmemeval/runs/targeted-ku-16/ablation-arm-h)
- **New Run (`B`)**: 16 questions (bench/longmemeval/runs/targeted-ku-16/ablation-arm-i)
- **Server Commit**: `87950213da014e97424f3ec2cbf8e1876c9c7f5b`
- **Run B Cost**: $0.0004

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  87.5% | **+6.2%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 2 (2/0) |
| overall | 16 |  81.2% |  87.5% | **+6.2%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 2 (2/0) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 7.88 | 7.88 | +0.00 |
| Abstention accuracy | 0.0% (0/0) | 0.0% (0/0) | +0.0% |
| Said 'no information' while evidence present | 0 | 0 | +0 |
| Search latency p50 / p95 | 193 ms / 318 ms | 205 ms / 304 ms | +12 ms / -14 ms |

## Question Flips (Total Fixed: 1, Total Regressed: 0)

### Fixed (Wrong in A -> Right in B: 1)
- **`ba61f0b9`** (`knowledge-update`): How many women are on the team led by my former manager Rachel?

### Regressed (Right in A -> Wrong in B: 0)
- *(none)*
