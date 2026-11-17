# LongMemEval_S on Memro: flips-A-2

Questions answered: 3  ·  graded (gpt-4o-mini): 3
Server commit: `894d14192c858f758737b0ce009377dec1a76c70`
Engine options: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 1 |   0.0% (0/1) | 100.0% (1/1) | 100.0% (1/1) | 100.0% (1/1) | 1 (1/0) |
| multi-session | 2 | 100.0% (2/2) | 100.0% (2/2) | 100.0% (2/2) | 100.0% (2/2) | 0 (0/0) |
| overall | 3 |  66.7% (2/3) | 100.0% (3/3) | 100.0% (3/3) | 100.0% (3/3) | 1 (1/0) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 8.67 of 15 slots
- **Abstention accuracy**: 100.0% (1/1)
- **Failed with evidence present saying 'no info' / 'don't know'**: 0

Search latency: p50 614 ms, p95 614 ms (local Docker Postgres, 244 memories per question).

Run token usage & cost: $0.0059
- gpt-4o-mini: $0.0059, 37,371 in / 452 out tokens, 6 calls
