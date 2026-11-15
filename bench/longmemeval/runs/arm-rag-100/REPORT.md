# LongMemEval_S on Memro: arm-rag-100

Questions answered: 100  ·  graded (gpt-4o-mini): 100
Server commit: `c5e9c83184c4ce440036629f6ab11532c9855a1c`
Engine options: `{"top_k": 15, "baseline": "rag", "mode": "semantic", "answer_model": "gpt-4o-mini", "session_expansion": 0, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": false, "order": "score", "embedding_model": "text-embedding-3-small"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 16 |  87.5% (14/16) | 100.0% (16/16) | 100.0% (16/16) | 100.0% (16/16) | 2 (2/0) |
| multi-session | 27 |  70.4% (19/27) | 100.0% (27/27) | 100.0% (27/27) | 100.0% (27/27) | 8 (6/2) |
| single-session-assistant | 11 | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 0 (0/0) |
| single-session-preference | 6 |  66.7% (4/6) | 100.0% (6/6) | 100.0% (6/6) | 100.0% (6/6) | 2 (2/0) |
| single-session-user | 14 | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% (19/26) |  96.2% (25/26) |  96.2% (25/26) |  96.2% (25/26) | 7 (4/3) |
| overall | 100 |  81.0% (81/100) |  99.0% (99/100) |  99.0% (99/100) |  99.0% (99/100) | 19 (14/5) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 9.14 of 15 slots
- **Abstention accuracy**:  83.3% (5/6)
- **Failed with evidence present saying 'no info' / 'don't know'**: 2

Search latency: p50 1074 ms, p95 3616 ms (local Docker Postgres, 246 memories per question).

Run token usage & cost: $0.1332
- gpt-4o-mini: $0.1332, 844,595 in / 10,924 out tokens, 200 calls
