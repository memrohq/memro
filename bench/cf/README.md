# Cloudflare Benchmark & Memory-Engine Research Suite

This suite uses Memro's Cloudflare Startup Grant ($5,000 credits) as an experimental testbed for memory-engine research, keeping PostgreSQL/pgvector as the baseline and OpenAI models as neutral judges.

Refer to ADR [0021-cloudflare-benchmark-experiments](../../docs/decisions/0021-cloudflare-benchmark-experiments.md).

## Architecture

| Component | Role in Memro Benchmarks | Why |
|---|---|---|
| **Workers AI** | Bulk embeddings & reranking experiments | Tests open models (BGE-small, BGE-M3, Qwen) vs OpenAI baseline |
| **R2** | Benchmark artifact storage | Stores 250MB+ datasets, cached vectors, and run outputs outside application DB |
| **Vectorize** | Alternate vector-search backend | Evaluates serverless index latency, Recall@k, and cost vs PostgreSQL `pgvector` |
| **PostgreSQL / pgvector** | Comparison baseline | The self-hosted production standard for Memro |
| **OpenAI** | Independent evaluator | `gpt-4o-mini` / `gpt-4o` as impartial answerers and judges |
| **AI Gateway** | Routing & observability | Optional request monitoring and rate-limit buffering |

## Components

- `workers_ai_relay.py`: Local OpenAI-compatible HTTP relay for Workers AI embedding models.
- `r2_store.py`: Synchronizes benchmark datasets (`longmemeval_s_cleaned.json`, etc.) and results with R2.
- `vectorize_benchmark.py`: Ingestion and query latency/recall testing against Cloudflare Vectorize.
- `matrix_run.py`: Multi-configuration benchmark runner comparing embeddings and search backends.

## Environment Configuration

Create `bench/cf/.env` or set environment variables:

```bash
# Cloudflare Credentials
CLOUDFLARE_ACCOUNT_ID="your_account_id"
CLOUDFLARE_API_TOKEN="your_workers_ai_and_vectorize_token"

# R2 Configuration (S3-compatible API token)
R2_ACCESS_KEY_ID="your_r2_access_key"
R2_SECRET_ACCESS_KEY="your_r2_secret_key"
R2_BUCKET_NAME="memro-benchmarks"

# OpenAI Evaluator
OPENAI_API_KEY="your_openai_key"
```

## Quickstart

### 1. Start Workers AI Embedding Relay
```bash
python bench/cf/workers_ai_relay.py --model @cf/baai/bge-small-en-v1.5 --port 18201
```

### 2. Sync Datasets to/from R2
```bash
python bench/cf/r2_store.py pull-dataset longmemeval_s_cleaned.json
```

### 3. Run Benchmark Matrix
```bash
python bench/cf/matrix_run.py --subset bench/longmemeval/subset_100.json
```
