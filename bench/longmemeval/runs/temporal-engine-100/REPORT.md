# LongMemEval_S on Memro: temporal-engine-100

Questions answered: 100  ·  graded (gpt-4o-mini): 100
Server commit: `e7d4bd1e3610b05f9844a3616e700a5b70c5888a`
Engine options: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% (13/16) |  93.8% (15/16) | 100.0% (16/16) | 100.0% (16/16) | 3 (3/0) |
| multi-session | 27 |  70.4% (19/27) | 100.0% (27/27) | 100.0% (27/27) | 100.0% (27/27) | 8 (6/2) |
| single-session-assistant | 11 | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 0 (0/0) |
| single-session-preference | 6 |  83.3% (5/6) | 100.0% (6/6) | 100.0% (6/6) | 100.0% (6/6) | 1 (1/0) |
| single-session-user | 14 | 100.0% (14/14) |  92.9% (13/14) | 100.0% (14/14) | 100.0% (14/14) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% (19/26) |  88.5% (23/26) |  96.2% (25/26) |  96.2% (25/26) | 7 (4/3) |
| overall | 100 |  81.0% (81/100) |  95.0% (95/100) |  99.0% (99/100) |  99.0% (99/100) | 19 (14/5) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 9.14 of 15 slots
- **Abstention accuracy**:  83.3% (5/6)
- **Failed with evidence present saying 'no info' / 'don't know'**: 2

Search latency: p50 373 ms, p95 1396 ms (local Docker Postgres, 246 memories per question).

Run token usage & cost: $0.1761
- gpt-4o-mini: $0.1761, 1,132,183 in / 10,388 out tokens, 200 calls
