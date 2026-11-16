# Memro benchmarks

Everything here runs against the current stack: the Go server on PostgreSQL 16
with pgvector. Results and methods are written up in
[docs/benchmarks](../docs/benchmarks/README.md).

| Path | What it measures |
| --- | --- |
| [`longmemeval/`](longmemeval) | End-to-end accuracy and recall on LongMemEval_S (100-question subset or all 500), plus no-memory (`BASELINE=none`), plain-RAG (`BASELINE=rag`) and full-history (`BASELINE=full`) comparison arms, `TOKEN_BUDGET=N` to cap the prompt context, and `tokens.py` for accuracy against prompt tokens |
| [`eval_suite.py`](eval_suite.py) | Isolation, contradictory facts, stale facts and distractor rejection ([results](../docs/benchmarks/evaluations.md)) |
| [`cf/`](cf) | Embedding model comparison (ADR 0024) |
| [`locomo/`](locomo) | LoCoMo runner |

The evolution benchmark (updates, duplicates and independent facts) is a Go
test: `MEMRO_TEST_DATABASE_URL=... go test ./internal/api -run TestEvolutionBench -v`
in `server/`, run in CI as a gate.
