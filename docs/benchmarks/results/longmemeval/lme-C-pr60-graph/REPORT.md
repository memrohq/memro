# LongMemEval_S on Memro: lme-C-pr60-graph

Questions answered: 100  ·  graded (gpt-4o-mini): 100
Server commit: `894d14192c858f758737b0ce009377dec1a76c70`
Engine options: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% (14/16) | 100.0% (16/16) | 100.0% (16/16) | 100.0% (16/16) | 2 (2/0) |
| multi-session | 27 |  66.7% (18/27) | 100.0% (27/27) | 100.0% (27/27) | 100.0% (27/27) | 9 (7/2) |
| single-session-assistant | 11 | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 0 (0/0) |
| single-session-preference | 6 |  83.3% (5/6) | 100.0% (6/6) | 100.0% (6/6) | 100.0% (6/6) | 1 (1/0) |
| single-session-user | 14 | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 0 (0/0) |
| temporal-reasoning | 26 |  69.2% (18/26) |  96.2% (25/26) |  96.2% (25/26) |  96.2% (25/26) | 8 (5/3) |
| overall | 100 |  80.0% (80/100) |  99.0% (99/100) |  99.0% (99/100) |  99.0% (99/100) | 20 (15/5) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 9.14 of 15 slots
- **Abstention accuracy**:  83.3% (5/6)
- **Failed with evidence present saying 'no info' / 'don't know'**: 2

Search latency: p50 849 ms, p95 2366 ms (local Docker Postgres, 246 memories per question).

Run token usage & cost: $0.1735
- gpt-4o-mini: $0.1735, 1,114,691 in / 10,430 out tokens, 200 calls
