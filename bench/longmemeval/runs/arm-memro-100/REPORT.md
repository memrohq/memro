# LongMemEval_S on Memro: arm-memro-100

Questions answered: 100  ·  graded (gpt-4o-mini): 100
Server commit: `6c1ed132855b41b2c7ab621a1a06f666748717c6`
Engine options: `{"top_k": 15, "baseline": "memro", "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 16 |  75.0% (12/16) | 100.0% (16/16) | 100.0% (16/16) | 100.0% (16/16) | 4 (4/0) |
| multi-session | 27 |  74.1% (20/27) | 100.0% (27/27) | 100.0% (27/27) | 100.0% (27/27) | 7 (6/1) |
| single-session-assistant | 11 | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 0 (0/0) |
| single-session-preference | 6 |  83.3% (5/6) | 100.0% (6/6) | 100.0% (6/6) | 100.0% (6/6) | 1 (1/0) |
| single-session-user | 14 | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% (19/26) |  96.2% (25/26) |  96.2% (25/26) |  96.2% (25/26) | 7 (4/3) |
| overall | 100 |  81.0% (81/100) |  99.0% (99/100) |  99.0% (99/100) |  99.0% (99/100) | 19 (15/4) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 9.13 of 15 slots
- **Abstention accuracy**: 100.0% (6/6)
- **Failed with evidence present saying 'no info' / 'don't know'**: 5

Search latency: p50 752 ms, p95 2227 ms (local Docker Postgres, 246 memories per question).

Run token usage & cost: $0.1715
- gpt-4o-mini: $0.1715, 1,101,372 in / 10,541 out tokens, 200 calls
