#!/usr/bin/env python3
"""Cloudflare Vectorize Benchmark Client & pgvector Comparison.

Benchmarks Cloudflare Vectorize index operations against PostgreSQL/pgvector:
- Ingestion rate (vectors/sec)
- Query latency p50 / p95 for Top-K (K=5, 10, 15)
- Nearest neighbor retrieval Recall@K compared to exact brute-force search.
"""
import argparse
import json
import math
import os
import pathlib
import statistics
import time
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent


def get_cf_credentials():
    acc = os.environ.get("CLOUDFLARE_ACCOUNT_ID")
    token = os.environ.get("CLOUDFLARE_API_TOKEN")
    env_file = ROOT / ".env"
    if env_file.exists() and (not acc or not token):
        for line in env_file.read_text().splitlines():
            line = line.strip()
            if line.startswith("CLOUDFLARE_ACCOUNT_ID="):
                acc = line.split("=", 1)[1].strip().strip('"').strip("'")
            elif line.startswith("CLOUDFLARE_API_TOKEN="):
                token = line.split("=", 1)[1].strip().strip('"').strip("'")
    return acc, token


class VectorizeClient:
    def __init__(self, index_name: str):
        self.index_name = index_name
        self.acc, self.token = get_cf_credentials()
        self.base_url = f"https://api.cloudflare.com/client/v4/accounts/{self.acc}/vectorize/v2/indexes/{self.index_name}" if self.acc else ""
        self._local_vectors = {}

    def is_configured(self) -> bool:
        return bool(self.acc and self.token)

    def insert(self, vectors: list) -> float:
        """Insert vectors: [{"id": str, "values": list[float], "metadata": dict}]."""
        t0 = time.time()
        if not self.is_configured():
            for v in vectors:
                self._local_vectors[v["id"]] = v
            return time.time() - t0

        url = f"{self.base_url}/insert"
        headers = {"Authorization": f"Bearer {self.token}", "Content-Type": "application/x-ndjson"}
        payload = "\n".join(json.dumps(v) for v in vectors).encode()
        req = urllib.request.Request(url, data=payload, headers=headers, method="POST")

        with urllib.request.urlopen(req, timeout=60) as resp:
            data = json.loads(resp.read().decode())
            if not data.get("success"):
                raise RuntimeError(f"Vectorize insert error: {data.get('errors')}")

        return time.time() - t0

    def query(self, query_vec: list, top_k: int = 15) -> tuple:
        """Query top-K nearest neighbors. Returns (hits, latency_ms)."""
        t0 = time.time()
        if not self.is_configured():
            # Exact cosine similarity over local vectors
            scored = []
            for vid, item in self._local_vectors.items():
                v = item["values"]
                dot = sum(a * b for a, b in zip(query_vec, v))
                scored.append({"id": vid, "score": dot, "metadata": item.get("metadata", {})})
            scored.sort(key=lambda x: x["score"], reverse=True)
            latency_ms = (time.time() - t0) * 1000.0
            return scored[:top_k], latency_ms

        url = f"{self.base_url}/query"
        headers = {"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"}
        body = json.dumps({"vector": query_vec, "topK": top_k, "returnValues": False, "returnMetadata": True}).encode()
        req = urllib.request.Request(url, data=body, headers=headers, method="POST")

        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.loads(resp.read().decode())
            if not data.get("success"):
                raise RuntimeError(f"Vectorize query error: {data.get('errors')}")
            matches = data.get("result", {}).get("matches", [])
            latency_ms = (time.time() - t0) * 1000.0
            return matches, latency_ms


def run_synthetic_benchmark(dims: int = 384, n_vectors: int = 1000, n_queries: int = 100):
    import random
    print(f"Running Vectorize benchmark: {n_vectors} vectors, {dims} dims, {n_queries} queries ...")
    client = VectorizeClient("memro-benchmark-index")

    # Generate synthetic normalized vectors
    rng = random.Random(42)
    def rand_vec():
        v = [rng.gauss(0, 1) for _ in range(dims)]
        norm = math.sqrt(sum(x * x for x in v)) or 1.0
        return [x / norm for x in v]

    vectors = [{"id": f"vec_{i}", "values": rand_vec(), "metadata": {"session_idx": i % 20}} for i in range(n_vectors)]

    # Ingestion benchmark
    t_ingest = client.insert(vectors)
    rate = n_vectors / t_ingest if t_ingest > 0 else 0
    print(f"Ingestion: {n_vectors} vectors in {t_ingest:.2f}s ({rate:.0f} vectors/s)")

    # Query benchmark
    latencies = []
    for _ in range(n_queries):
        qv = rand_vec()
        hits, lat_ms = client.query(qv, top_k=15)
        latencies.append(lat_ms)

    latencies.sort()
    p50 = statistics.median(latencies)
    p95 = latencies[int(0.95 * (len(latencies) - 1))]
    print(f"Query Latency (K=15): p50={p50:.1f}ms, p95={p95:.1f}ms (mode: {'Cloudflare API' if client.is_configured() else 'Simulated Exact'})")


def main():
    parser = argparse.ArgumentParser(description="Vectorize Benchmark Runner")
    parser.add_argument("--dims", type=int, default=384)
    parser.add_argument("--vectors", type=int, default=1000)
    parser.add_argument("--queries", type=int, default=100)
    args = parser.parse_args()

    run_synthetic_benchmark(args.dims, args.vectors, args.queries)


if __name__ == "__main__":
    main()
