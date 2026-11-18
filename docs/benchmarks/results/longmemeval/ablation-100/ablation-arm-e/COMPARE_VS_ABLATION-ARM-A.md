# Benchmark Comparison: `ablation-arm-a` -> `ablation-arm-e`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-a)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-e)
- **Server Commit**: `4b785c6023db90638e85324ce9a8524092e78687`
- **Run B Cost**: $0.0039

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  81.2% | **-6.2%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 3 (3/0) |
| multi-session | 27 |  66.7% |  70.4% | **+3.7%** 🟢 | 100.0% / 96.3% | 100.0% / 100.0% | 9 (7/2) | 8 (6/2) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  66.7% |  83.3% | **+16.7%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 92.9% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 96.2% / 88.5% | 96.2% / 96.2% | 7 (4/3) | 7 (4/3) |
| overall | 100 |  80.0% |  81.0% | **+1.0%** 🟢 | 99.0% / 95.0% | 99.0% / 99.0% | 20 (15/5) | 19 (14/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 9.14 | +0.00 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 4 | 4 | +0 |
| Search latency p50 / p95 | 163 ms / 356 ms | 203 ms / 428 ms | +40 ms / +72 ms |

## Question Flips (Total Fixed: 2, Total Regressed: 1)

### Fixed (Wrong in A -> Right in B: 2)
- **`b6025781`** (`single-session-preference`): I'm planning my meal prep next week, any suggestions for new recipes?
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?

### Regressed (Right in A -> Wrong in B: 1)
- **`ba61f0b9`** (`knowledge-update`): How many women are on the team led by my former manager Rachel?
