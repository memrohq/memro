# 0021. Cloudflare Startup Grant: Benchmark & Memory-Engine Research Architecture

- **Status**: Proposed
- **Date**: 2026-10-23
- **Deciders**: Engineering, Platform

## Context

Memro has received a $5,000 Cloudflare startup grant. Memory systems require intensive experimental iterations: embedding multi-gigabyte evaluation datasets, evaluating vector similarity search vs hybrid RRF retrieval, measuring recall quality, and comparing retrieval recall against cost and latency.

Rather than migrating production workloads or coupling the core memory engine to vendor-specific infrastructure, this decision establishes an experimental architecture leveraging the Cloudflare grant to expand research bandwidth while preserving Memro's source-available, PostgreSQL-centric core.

## Principles

1. **Credits Buy Experimental Freedom, Not Vendor Lock-In**:
   Core Memro remains self-hostable with standard PostgreSQL + pgvector. Cloudflare infrastructure is treated as an experiment testbed to evaluate alternative embedding and retrieval architectures.
2. **Separation of Concerns**:
   - **Compute (Workers AI)**: High-throughput embedding and reranking experiments across models (BGE-small, BGE-M3, Qwen).
   - **Storage (R2)**: Persistent storage for benchmark datasets, serialized embeddings, run logs, and diagnostic traces—keeping bulk experiment data out of PostgreSQL.
   - **Alternative Search Backend (Vectorize)**: Benchmarked directly against PostgreSQL/pgvector to evaluate latency, recall, and cost trade-offs.
   - **Independent Evaluator (OpenAI)**: Maintained strictly separate from embedding generation to avoid circular evaluation bias; models such as `gpt-4o-mini` and `gpt-4o` remain neutral judges.
   - **Observability (AI Gateway)**: Evaluated for request routing, caching, and rate-limit mitigation if the observability gains justify latency overhead.
3. **Controlled Single-Variable Methodology**:
   To isolate true performance drivers, every benchmark run varies exactly one parameter at a time:
   - Fixed dataset: LongMemEval_S (standard cleaned 500 questions and stratified 100-question subset).
   - Embedding models are never mixed in the same vector space. Re-embed the corpus for each candidate model and evaluate independently.
   - Metrics: Recall@5, Recall@10, Recall@15, MRR, nDCG@k, p50/p95 latency, and cost per 1,000 operations.

## Architecture

```
                  ┌─────────────────────────────────────┐
                  │       LongMemEval Test Corpus       │
                  │ (Questions, Sessions, Ground Truth) │
                  └──────────────────┬──────────────────┘
                                     │
                 ┌───────────────────┴───────────────────┐
                 │                                       │
                 ▼                                       ▼
     [Configuration A (Baseline)]           [Configuration B (Workers AI)]
       OpenAI text-embedding-3-small          Workers AI (BGE / BGE-M3 / Qwen)
                 │                                       │
                 ├───────────────────┬───────────────────┤
                 ▼                   ▼                   ▼
          [PostgreSQL pgvector] [Vectorize Index] [Hybrid RRF Engine]
                 │                   │                   │
                 └───────────────────┼───────────────────┘
                                     ▼
                      ┌──────────────────────────────┐
                      │    Neutral Answerer/Judge    │
                      │    (OpenAI gpt-4o-mini)      │
                      └──────────────┬───────────────┘
                                     ▼
                      ┌──────────────────────────────┐
                      │  Evaluation & Cost Reports   │
                      │ (Pushed to R2 Bucket / Git)  │
                      └──────────────────────────────┘
```

## Implementation Components

1. **Workers AI Embedding Relay (`bench/cf/workers_ai_relay.py`)**:
   - Implements an OpenAI-compatible `/v1/embeddings` endpoint bridging to Cloudflare Workers AI.
   - Supports model comparison:
     - `@cf/baai/bge-small-en-v1.5` (384 dims, fast, lightweight)
     - `@cf/baai/bge-base-en-v1.5` (768 dims)
     - `@cf/baai/bge-large-en-v1.5` (1024 dims)
     - `@cf/baai/bge-m3` (1024 dims, multi-lingual / long-context)
   - Features local SQLite and optional R2 backing cache for embedding deduplication.

2. **R2 Artifact Manager (`bench/cf/r2_store.py`)**:
   - S3-compatible client targeting Cloudflare R2 (`https://<accountid>.r2.cloudflarestorage.com`).
   - Manages large evaluation assets without cluttering Git or local databases:
     - LongMemEval raw & cleaned datasets.
     - Pre-computed embedding matrices per model.
     - Longitudinal benchmark results and query trace archives.

3. **Vectorize Comparison Harness (`bench/cf/vectorize_benchmark.py`)**:
   - Benchmarks Vectorize indices against PostgreSQL `pgvector` on identical corpora.
   - Compares:
     - Ingestion throughput (vectors/sec).
     - Query latency at $k=5, 10, 15$ (p50, p95).
     - Nearest neighbor recall vs exact brute-force ground truth.

4. **Matrix Runner (`bench/cf/matrix_run.py`)**:
   - Executes multi-model, multi-backend benchmark matrices with automated markdown reporting.

## Consequences

- Memro maintains reproducible, independent benchmarks without coupling production server code to vendor services.
- The $5,000 grant is utilized for high-throughput embedding generation and vector search evaluation.
- R2 provides durable storage for large benchmark corpora.
- Results provide objective empirical data on whether open embedding models (BGE, Qwen) achieve comparable recall to proprietary APIs for agent memory.
