# LongMemEval_S on Memro: temporal-C-1

Questions answered: 1  ·  graded (gpt-4o-mini): 1
Server commit: `894d14192c858f758737b0ce009377dec1a76c70`
Engine options: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| temporal-reasoning | 1 | 100.0% (1/1) | 100.0% (1/1) | 100.0% (1/1) | 100.0% (1/1) | 0 (0/0) |
| overall | 1 | 100.0% (1/1) | 100.0% (1/1) | 100.0% (1/1) | 100.0% (1/1) | 0 (0/0) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 10.00 of 15 slots
- **Abstention accuracy**: N/A
- **Failed with evidence present saying 'no info' / 'don't know'**: 0

Search latency: p50 823 ms, p95 823 ms (local Docker Postgres, 269 memories per question).

Run token usage & cost: $0.0014
- gpt-4o-mini: $0.0014, 8,947 in / 83 out tokens, 2 calls
