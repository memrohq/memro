# Benchmark Comparison: `baseline-100` -> `baseline-100`

- **Baseline Run (`A`)**: 100 questions (../memro-bench-layout/bench/longmemeval/runs/baseline-100)
- **New Run (`B`)**: 100 questions (../memro-bench-layout/bench/longmemeval/runs/baseline-100)
- **Server Commit**: `5d0ec7a38220c799bba65f64df3a822aef56f4d5`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000}`
- **Run B Cost**: $0.1714

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  81.2% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 3 (3/0) | 3 (3/0) |
| multi-session | 27 |  59.3% |  59.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 11 (9/2) | 11 (9/2) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  83.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 96.2% / 96.2% | 96.2% / 96.2% | 7 (4/3) | 7 (4/3) |
| overall | 100 |  78.0% |  78.0% | +0.0% ⚪ | 99.0% / 99.0% | 99.0% / 99.0% | 22 (17/5) | 22 (17/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 9.14 | +0.00 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 7 | 7 | +0 |
| Search latency p50 / p95 | 229 ms / 639 ms | 229 ms / 639 ms | +0 ms / +0 ms |

## Question Flips (Total Fixed: 0, Total Regressed: 0)

### Fixed (Wrong in A -> Right in B: 0)
- *(none)*

### Regressed (Right in A -> Wrong in B: 0)
- *(none)*
