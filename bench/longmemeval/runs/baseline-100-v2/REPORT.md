# LongMemEval_S on Memro: baseline-100-v2

Questions answered: 100  ·  graded (gpt-4o-mini): 100
Server commit: `d400b55cec29e82cea06fae44deef94b47118865`
Engine options: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000, "embedding_model": "text-embedding-3-small"}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 16 |  75.0% (12/16) | 100.0% (16/16) | 100.0% (16/16) | 100.0% (16/16) | 4 (4/0) |
| multi-session | 27 |  77.8% (21/27) | 100.0% (27/27) | 100.0% (27/27) | 100.0% (27/27) | 6 (5/1) |
| single-session-assistant | 11 | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 0 (0/0) |
| single-session-preference | 6 |  83.3% (5/6) | 100.0% (6/6) | 100.0% (6/6) | 100.0% (6/6) | 1 (1/0) |
| single-session-user | 14 | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 0 (0/0) |
| temporal-reasoning | 26 |  69.2% (18/26) |  96.2% (25/26) |  96.2% (25/26) |  96.2% (25/26) | 8 (5/3) |
| overall | 100 |  81.0% (81/100) |  99.0% (99/100) |  99.0% (99/100) |  99.0% (99/100) | 19 (15/4) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 9.14 of 15 slots
- **Abstention accuracy**: 100.0% (6/6)
- **Failed with evidence present saying 'no info' / 'don't know'**: 3

Search latency: p50 170 ms, p95 276 ms (local Docker Postgres, 246 memories per question).

Run token usage & cost: $0.1716
- gpt-4o-mini: $0.1716, 1,101,129 in / 10,771 out tokens, 200 calls
