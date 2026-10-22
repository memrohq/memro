# Memro Benchmarks & Evaluations

Reproducible, verified benchmarks evaluating Memro's retrieval precision, fact consistency, and latency.

- [LongMemEval Benchmark Report](longmemeval.md): 500-question evaluation on LongMemEval_S with fixed models (`gpt-4o-mini`, `text-embedding-3-small`), disclosed API costs ($2.8664 ledger total, ~$1.25 per 500-question pass), category breakdown, and disclosed limitations.
- [Measurable Verification Benchmark & Evaluations](evaluations.md): Empirical test suite across identity boundaries (100% pass), contradictory facts, stale facts, and distractor rejection (`bench/eval_suite.py`).
- [Code-Memory Benchmark Design](code-memory.md): Design specification for measuring historical codebase decision memory across commits.
- [Web Summary](https://memro.co/benchmarks): Interactive benchmarks page on memro.co.
