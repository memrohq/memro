# Embedding-model evaluation with Workers AI

Benchmark-only tooling (ADR 0024). It answers one question: **would another
embedding model retrieve Memro's memories as well as OpenAI
`text-embedding-3-small`?** Everything else stays fixed: the same Memro
server, Postgres + pgvector, hybrid search, questions, answer model and judge.
Nothing here ships in Memro Cloud. Cloudflare's startup credits pay for the
Workers AI calls.

## Setup

1. Postgres with pgvector, e.g. the `memro-bench-db` container on port 55434
   (`MEMRO_DATABASE_URL`, default `postgres://memro:memro@127.0.0.1:55434/memro`).
2. Go, to build the server once (`--build`).
3. `OPENAI_API_KEY` in `memro-benchmark/.env` or the environment (answers, judge,
   and the OpenAI baseline).
4. `bench/.env` or `bench/cf/.env` (both gitignored) with a Cloudflare API token that has Workers AI
   Read:
   ```
   CLOUDFLARE_ACCOUNT_ID=...
   CLOUDFLARE_API_TOKEN=...
   ```

## Run

```bash
# Retrieval only: embeddings, no OpenAI chat cost
python bench/cf/eval_embeddings.py --retrieval-only --build

# Full: + answers and gpt-4o-mini judge (~$0.20 per model on 100 questions)
python bench/cf/eval_embeddings.py

# Pick models
python bench/cf/eval_embeddings.py --models text-embedding-3-small @cf/qwen/qwen3-embedding-0.6b
```

Each model gets a fresh server (its query cache must not carry over) and a run
in `bench/longmemeval/runs/emb-<model>/`. The table goes to
`bench/longmemeval/runs/EMBEDDINGS_<date>.md`. Rerunning resumes, and Workers
AI vectors are cached in `bench/cf/cache/`, so repeats are free.

## Reading the results

- **Evidence R@k** is the share of the exact answer-bearing exchanges
  (LongMemEval `has_answer`) in the top k. This is the metric that separates
  models. Session-level recall is about 99% for all of them.
- **All evidence in prompt**: the answering model saw every exchange it needed.
- **QA accuracy** moves 1 point per question on 100 questions. Confirm a
  winner on all 500 before acting on it.
- Models with a 512-token input limit (`bge-base`, `bge-large`, `bge-small`)
  only see the start of longer exchanges, so they start at a disadvantage.

## Models

`workers_ai_relay.py` lists the ones it accepts. Defaults: `bge-m3` (60k
tokens) and `qwen3-embedding-0.6b` (8k tokens), against OpenAI. Memro embeds
queries and memories the same way, so instruction-tuned models are tested the
way Memro would use them.

## Write it up

Every run gets a post in `docs/benchmarks/journal/`: what was run, the table,
what changed, and what's next.
