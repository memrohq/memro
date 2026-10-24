# Benchmark Comparison: `control-same-server-100` -> `temporal-engine-100`

- **Baseline Run (`A`)**: 100 questions (bench/longmemeval/runs/control-same-server-100)
- **New Run (`B`)**: 100 questions (bench/longmemeval/runs/temporal-engine-100)
- **Server Commit**: `e7d4bd1e3610b05f9844a3616e700a5b70c5888a`
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score"}`
- **Run B Cost**: $0.1761

## Category Accuracy & Retrieval Delta

| Category | n | Acc A | Acc B | Delta | R@5 A / B | R@15 A / B | Fails A (ctx/miss) | Fails B (ctx/miss) |
|---|---|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% |  81.2% | +0.0% ⚪ | 100.0% / 93.8% | 100.0% / 100.0% | 3 (3/0) | 3 (3/0) |
| multi-session | 27 |  70.4% |  70.4% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 8 (6/2) | 8 (6/2) |
| single-session-assistant | 11 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| single-session-preference | 6 |  83.3% |  83.3% | +0.0% ⚪ | 100.0% / 100.0% | 100.0% / 100.0% | 1 (1/0) | 1 (1/0) |
| single-session-user | 14 | 100.0% | 100.0% | +0.0% ⚪ | 100.0% / 92.9% | 100.0% / 100.0% | 0 (0/0) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% |  73.1% | +0.0% ⚪ | 96.2% / 88.5% | 96.2% / 96.2% | 7 (4/3) | 7 (4/3) |
| overall | 100 |  81.0% |  81.0% | +0.0% ⚪ | 99.0% / 95.0% | 99.0% / 99.0% | 19 (14/5) | 19 (14/5) |

## Synthesis & Quality Metrics

| Metric | Baseline (`A`) | New (`B`) | Delta |
|---|---|---|---|
| Duplicate session slots (avg / 15) | 9.14 | 9.14 | +0.00 |
| Abstention accuracy | 83.3% (5/6) | 83.3% (5/6) | +0.0% |
| Said 'no information' while evidence present | 3 | 2 | -1 |
| Search latency p50 / p95 | 158 ms / 324 ms | 373 ms / 1396 ms | +214 ms / +1072 ms |

## Question Flips (Total Fixed: 1, Total Regressed: 1)

### Fixed (Wrong in A -> Right in B: 1)
- **`d905b33f`** (`multi-session`): What percentage discount did I get on the book from my favorite author?

### Regressed (Right in A -> Wrong in B: 1)
- **`46a3abf7`** (`multi-session`): How many tanks do I currently have, including the one I set up for my friend's kid?
