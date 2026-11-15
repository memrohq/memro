# LongMemEval_S on Memro: arm-none-100

Questions answered: 100  ·  graded (gpt-4o-mini): 100
Server commit: `c5e9c83184c4ce440036629f6ab11532c9855a1c`
Engine options: `{"top_k": 15, "baseline": "none", "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "per_session_scope": "all", "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 16 |   0.0% (0/16) |   0.0% (0/16) |   0.0% (0/16) |   0.0% (0/16) | 16 (0/16) |
| multi-session | 27 |   7.4% (2/27) |   0.0% (0/27) |   0.0% (0/27) |   0.0% (0/27) | 25 (0/25) |
| single-session-assistant | 11 |   0.0% (0/11) |   0.0% (0/11) |   0.0% (0/11) |   0.0% (0/11) | 11 (0/11) |
| single-session-preference | 6 |   0.0% (0/6) |   0.0% (0/6) |   0.0% (0/6) |   0.0% (0/6) | 6 (0/6) |
| single-session-user | 14 |  28.6% (4/14) |   0.0% (0/14) |   0.0% (0/14) |   0.0% (0/14) | 10 (0/10) |
| temporal-reasoning | 26 |   0.0% (0/26) |   0.0% (0/26) |   0.0% (0/26) |   0.0% (0/26) | 26 (0/26) |
| overall | 100 |   6.0% (6/100) |   0.0% (0/100) |   0.0% (0/100) |   0.0% (0/100) | 94 (0/94) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 0.00 of 15 slots
- **Abstention accuracy**: 100.0% (6/6)
- **Failed with evidence present saying 'no info' / 'don't know'**: 0

Search latency: p50 0 ms, p95 0 ms (local Docker Postgres, 0 memories per question).

Run token usage & cost: $0.0121
- gpt-4o-mini: $0.0121, 60,356 in / 5,019 out tokens, 200 calls
