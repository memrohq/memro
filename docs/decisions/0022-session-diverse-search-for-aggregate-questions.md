# 0022. Session-Diverse Search for Aggregate Questions
Date: 2026-10-23 · Status: Accepted (off by default)

## Context
On LongMemEval (100-question subset), 17 questions were wrong in two runs with identical retrieval; 9 of them lacked some of the exact answer-bearing exchanges in the answer prompt. Most are aggregate questions ("how many…", "total…", "list…") whose evidence sits in several conversations, while about 9 of the 15 results came from conversations already represented. Search already supports a per-conversation cap (`max_per_session`), but callers rarely set it.

## Decision
- `SEARCH_AGGREGATE_PER_SESSION=n` (default `0`, off): when a search sets no `max_per_session` and `engine.DetectQueryIntent` classifies the query as `aggregation` (an English keyword match for counts and totals: "how many", "how much", "total", "list all", "number of"…), results are capped at `n` per conversation, then filled with the remaining best results. A request's own `max_per_session` wins.
- Measured with exchange-level evidence, which is deterministic (no LLM), against the uncapped run. These runs used the first version's broader pattern (which also matched "list", "all the", "every", "both"); the shipped detector is `DetectQueryIntent`, so the numbers should be re-checked with it before turning the setting on:

| Variant | All evidence in prompt (94 q) | Retrieval targets fixed (of 9) | Questions losing evidence | Evidence recall@15 |
|---|---|---|---|---|
| none | 85 | 0 | – | 93.3% |
| cap 2, aggregate only | 87 | 2 | 0 | 90.4% |
| **cap 3, aggregate only** | **88** | **3** | **0** | **93.1%** |
| cap 2, every question | 87 | 2 | 0 | 78.9% |

- The server setting reproduced the client-side cap exactly (100/100 identical results) with the first version's pattern.

## Consequences
- Off by default; turned on per environment after an accuracy check with the paired test (`bench/longmemeval/compare.py`).
- It fixes 3 of the 9 retrieval targets. The rest need deeper candidates or reranking, tested the same way.
- The pattern is English-only; other languages see unchanged search.
