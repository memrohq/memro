# LongMemEval_S on Memro: baseline-100

Questions answered: 100  ·  graded (gpt-4o-mini): 100
Server commit: `5d0ec7a38220c799bba65f64df3a822aef56f4d5`
Engine options: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "format": "standard", "write_time_supersession": "false", "per_item_chars": 4000}`

| Type | n | QA accuracy | Recall@5 | Recall@10 | Recall@15 | Failures (ctx/miss) |
|---|---|---|---|---|---|---|
| knowledge-update | 16 |  81.2% (13/16) | 100.0% (16/16) | 100.0% (16/16) | 100.0% (16/16) | 3 (3/0) |
| multi-session | 27 |  59.3% (16/27) | 100.0% (27/27) | 100.0% (27/27) | 100.0% (27/27) | 11 (9/2) |
| single-session-assistant | 11 | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 100.0% (11/11) | 0 (0/0) |
| single-session-preference | 6 |  83.3% (5/6) | 100.0% (6/6) | 100.0% (6/6) | 100.0% (6/6) | 1 (1/0) |
| single-session-user | 14 | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 100.0% (14/14) | 0 (0/0) |
| temporal-reasoning | 26 |  73.1% (19/26) |  96.2% (25/26) |  96.2% (25/26) |  96.2% (25/26) | 7 (4/3) |
| overall | 100 |  78.0% (78/100) |  99.0% (99/100) |  99.0% (99/100) |  99.0% (99/100) | 22 (17/5) |

## Retrieval & Synthesis Diagnostics
- **Average duplicate-session slots**: 9.14 of 15 slots
- **Abstention accuracy**:  83.3% (5/6)
- **Failed with evidence present saying 'no info' / 'don't know'**: 7

Search latency: p50 229 ms, p95 639 ms (local Docker Postgres, 246 memories per question).

Run token usage & cost: $0.1714
- gpt-4o-mini: $0.1714, 1,100,808 in / 10,453 out tokens, 200 calls
