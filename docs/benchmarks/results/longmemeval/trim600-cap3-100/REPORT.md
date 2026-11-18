# LongMemEval_S on Memro: trim600-cap3-100

Questions answered: 100  ·  graded (gpt-4o-mini): 100
Server commit: `f71c5d99f590ec5c3f6b8dd2c5cd1a237725be5c`
Engine options: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "assistant_chars": 600, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% (13/16) | 100.0% (16/16) | 100.0% (16/16) | 100.0% (16/16) | 3 (3/0) |
| multi-session | 27 |  70.4% (19/27) | 100.0% (27/27) | 100.0% (27/27) | 100.0% (27/27) | 8 (8/0) |
| single-session-assistant | 11 | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 0 (0/0) |
| single-session-preference | 6 |  83.3% (5/6) | 100.0% (6/6) | 100.0% (6/6) | 100.0% (6/6) | 1 (1/0) |
| single-session-user | 14 | 100.0% (14/14) |  85.7% (12/14) | 100.0% (14/14) | 100.0% (14/14) | 0 (0/0) |
| temporal-reasoning | 26 |  80.8% (21/26) |  88.5% (23/26) |  96.2% (25/26) |  96.2% (25/26) | 5 (3/2) |
| overall | 100 |  83.0% (83/100) |  95.0% (95/100) |  99.0% (99/100) |  99.0% (99/100) | 17 (15/2) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 8.49 of 15 slots
- **Abstention accuracy**: 100.0% (6/6)
- **Failed with evidence present saying 'no info' / 'don't know'**: 5

Search latency: p50 98 ms, p95 165 ms (local Docker Postgres, 246 memories per question).

Run token usage & cost: $0.0685
- gpt-4o-mini: $0.0685, 415,927 in / 10,235 out tokens, 200 calls
