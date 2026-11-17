# Embedding comparison: 2026-10-23

LongMemEval_S, subset_100.json. Same Memro server, Postgres + pgvector and hybrid search; only the embedding model differs. Answers and judge: gpt-4o-mini.

Evidence recall counts the exact exchanges LongMemEval marks as holding the answer (session-level recall is ~99% for every model and doesn't separate them).

| Embedding model | n | QA accuracy | Evidence R@5 | Evidence R@15 | All evidence in top 15 | All evidence in prompt | Session R@5 | Session MRR | Search p50 | Embed wait | Embed errors |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `text-embedding-3-small` | 100 | 81.0% | 76.9% | 93.3% | 87.2% | 90.4% | 99.0% | 0.923 | 170 ms | 2.1 s | 0 |
| `@cf/baai/bge-m3` | 100 | 79.0% | 73.1% | 93.6% | 90.4% | 92.6% | 96.0% | 0.900 | 171 ms | 2.1 s | 0 |

Workers AI usage (embeddings not already cached):

- `@cf/baai/bge-m3`: 0 calls, 0 texts embedded, 24740 from cache, about 0 tokens (estimated)

With 100 questions, one question is 1 point of accuracy: confirm a winner on all 500 (`--subset` with the full ID list) before acting on it.
