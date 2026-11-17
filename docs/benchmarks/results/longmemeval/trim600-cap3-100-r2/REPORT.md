# LongMemEval_S on Memro: trim600-cap3-100-r2

Questions answered: 100  ·  graded (gpt-4o-mini): 100
Server commit: `f71c5d99f590ec5c3f6b8dd2c5cd1a237725be5c`
Engine options: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "assistant_chars": 600, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% (14/16) | 100.0% (16/16) | 100.0% (16/16) | 100.0% (16/16) | 2 (2/0) |
| multi-session | 27 |  63.0% (17/27) | 100.0% (27/27) | 100.0% (27/27) | 100.0% (27/27) | 10 (10/0) |
| single-session-assistant | 11 | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 0 (0/0) |
| single-session-preference | 6 |  66.7% (4/6) | 100.0% (6/6) | 100.0% (6/6) | 100.0% (6/6) | 2 (2/0) |
| single-session-user | 14 | 100.0% (14/14) |  85.7% (12/14) | 100.0% (14/14) | 100.0% (14/14) | 0 (0/0) |
| temporal-reasoning | 26 |  76.9% (20/26) |  88.5% (23/26) |  96.2% (25/26) |  96.2% (25/26) | 6 (4/2) |
| overall | 100 |  80.0% (80/100) |  95.0% (95/100) |  99.0% (99/100) |  99.0% (99/100) | 20 (18/2) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 8.49 of 15 slots
- **Abstention accuracy**:  83.3% (5/6)
- **Failed with evidence present saying 'no info' / 'don't know'**: 5

Search latency: p50 120 ms, p95 250 ms (local Docker Postgres, 246 memories per question).

Run token usage & cost: $0.0684
- gpt-4o-mini: $0.0684, 415,711 in / 10,018 out tokens, 200 calls
