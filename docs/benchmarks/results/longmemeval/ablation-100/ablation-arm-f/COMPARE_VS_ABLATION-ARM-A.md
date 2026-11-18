# Benchmark Comparison: `ablation-arm-a` -> `ablation-arm-f`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-a)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/ablation-100/ablation-arm-f)
- **Server Commit**: `4b785c6023db90638e85324ce9a8524092e78687`
- **Run B Cost**: $0.0038

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% |  75.0% | **-12.5%** 🔴 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 4 (4/0) |
| multi-session | 27 |  66.7% |  74.1% | **+7.4%** 🟢 | 100.0% / 96.3% | 100.0% / 100.0% | 9 (7/2) | 7 (5/2) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  66.7% |  83.3% | **+16.7%** 🟢 | 100.0% / 100.0% | 100.0% / 100.0% | 2 (2/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  69.2% | **-3.8%** 🔴 | 96.2% / 96.2% | 96.2% / 96.2% | 7 (4/3) | 8 (5/3) |
| overall | 100 |  80.0% |  80.0% | +0.0% ⚪ | 99.0% / 98.0% | 99.0% / 99.0% | 20 (15/5) | 20 (15/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 8.37 | -0.77 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 4 | 5 | +1 |
| Search latency p50 / p95 | 163 ms / 356 ms | 208 ms / 866 ms | +45 ms / +509 ms |

## Question Flips (Total Fixed: 3, Total Regressed: 3)

### Fixed (Wrong in A -> Right in B: 3)
- **`88432d0a`** (`multi-session`): How many times did I bake something in the past two weeks?
- **`b6025781`** (`single-session-preference`): I'm planning my meal prep next week, any suggestions for new recipes?
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?

### Regressed (Right in A -> Wrong in B: 3)
- **`2133c1b5`** (`knowledge-update`): How long have I been living in my current apartment in Harajuku?
- **`ba61f0b9`** (`knowledge-update`): How many women are on the team led by my former manager Rachel?
- **`gpt4_68e94287`** (`temporal-reasoning`): Which event happened first, my participation in the #PlankChallenge or my post about vegan
