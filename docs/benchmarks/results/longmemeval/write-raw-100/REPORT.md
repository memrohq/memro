# LongMemEval_S on Memro: write-raw-100

Questions answered: 100  ·  graded (gpt-4o-mini): 100
Server commit: `07e60ba06d2e7ad895e80a741489cc7d7b333627`
Engine options: `{"top_k": 15, "baseline": "memro", "hide_superseded": false, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "token_budget": 0, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small", "write_mode": "raw"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% (14/16) | 100.0% (16/16) | 100.0% (16/16) | 100.0% (16/16) | 2 (2/0) |
| multi-session | 27 |  74.1% (20/27) | 100.0% (27/27) | 100.0% (27/27) | 100.0% (27/27) | 7 (5/2) |
| single-session-assistant | 11 | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 0 (0/0) |
| single-session-preference | 6 |  83.3% (5/6) | 100.0% (6/6) | 100.0% (6/6) | 100.0% (6/6) | 1 (1/0) |
| single-session-user | 14 | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 0 (0/0) |
| temporal-reasoning | 26 |  76.9% (20/26) |  96.2% (25/26) |  96.2% (25/26) |  96.2% (25/26) | 6 (4/2) |
| overall | 100 |  84.0% (84/100) |  99.0% (99/100) |  99.0% (99/100) |  99.0% (99/100) | 16 (12/4) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 9.13 of 15 slots
- **Abstention accuracy**:  83.3% (5/6)
- **Failed with evidence present saying 'no info' / 'don't know'**: 2

Search latency: p50 482 ms, p95 635 ms (local Docker Postgres, 246 memories per question).

Run token usage & cost: $0.1716
- gpt-4o-mini: $0.1716, 1,101,697 in / 10,602 out tokens, 200 calls
