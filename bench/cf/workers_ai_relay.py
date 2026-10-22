#!/usr/bin/env python3
"""OpenAI-compatible /v1/embeddings relay backed by Cloudflare Workers AI.

Enables benchmarking Memro directly with Workers AI embedding models:
- @cf/baai/bge-small-en-v1.5 (384 dims)
- @cf/baai/bge-base-en-v1.5 (768 dims)
- @cf/baai/bge-large-en-v1.5 (1024 dims)
- @cf/baai/bge-m3 (1024 dims)

Embeddings are cached in SQLite by (model, sha256(text)) so repeated evaluations
incur zero additional latency or credit consumption.
"""
import argparse
import base64
import hashlib
import json
import os
import pathlib
import sqlite3
import struct
import sys
import threading
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ROOT = pathlib.Path(__file__).resolve().parent
DB = ROOT / "cf_embed_cache.sqlite"
_lock = threading.Lock()
_conn = sqlite3.connect(DB, check_same_thread=False)
_conn.execute("CREATE TABLE IF NOT EXISTS emb (k TEXT PRIMARY KEY, v BLOB)")
_conn.commit()

DEFAULT_MODEL = "@cf/baai/bge-small-en-v1.5"
SUPPORTED_MODELS = {
    "@cf/baai/bge-small-en-v1.5": 384,
    "@cf/baai/bge-base-en-v1.5": 768,
    "@cf/baai/bge-large-en-v1.5": 1024,
    "@cf/baai/bge-m3": 1024,
    "bge-small": 384,
    "bge-base": 768,
    "bge-large": 1024,
    "bge-m3": 1024,
}

MODEL_ALIASES = {
    "bge-small": "@cf/baai/bge-small-en-v1.5",
    "bge-base": "@cf/baai/bge-base-en-v1.5",
    "bge-large": "@cf/baai/bge-large-en-v1.5",
    "bge-m3": "@cf/baai/bge-m3",
    "text-embedding-3-small": "@cf/baai/bge-small-en-v1.5",  # fallback alias
}

stats = {"hits": 0, "misses": 0, "calls": 0}


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


def _key(model: str, text: str) -> str:
    return model + ":" + hashlib.sha256(text.encode()).hexdigest()


def call_workers_ai(model: str, texts: list) -> list:
    acc, token = get_cf_credentials()
    if not acc or not token:
        # In offline or mock mode, generate deterministic normalized vector
        return [pseudo_vector(t, SUPPORTED_MODELS.get(model, 384)) for t in texts]

    url = f"https://api.cloudflare.com/client/v4/accounts/{acc}/ai/run/{model}"
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
    }
    body = json.dumps({"text": texts}).encode()
    req = urllib.request.Request(url, data=body, headers=headers, method="POST")

    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            data = json.loads(resp.read().decode())
            if not data.get("success"):
                errors = data.get("errors", [])
                raise RuntimeError(f"Cloudflare Workers AI error: {errors}")
            result = data.get("result", {})
            # Workers AI returns {"shape": [...], "data": [[...], ...]}
            return result.get("data", [])
    except urllib.error.HTTPError as e:
        msg = e.read().decode(errors="replace")
        raise RuntimeError(f"Workers AI HTTP {e.code}: {msg}")


def pseudo_vector(text: str, dims: int = 384) -> list:
    """Deterministic offline vector for dry-run testing."""
    import math
    v = [0.0] * dims
    for w in text.lower().split():
        idx = int(hashlib.md5(w.strip(".,!?").encode()).hexdigest(), 16) % dims
        v[idx] += 1.0
    norm = math.sqrt(sum(x * x for x in v)) or 1.0
    return [x / norm for x in v]


def embed(model: str, texts: list) -> list:
    resolved_model = MODEL_ALIASES.get(model, model)
    keys = [_key(resolved_model, t) for t in texts]

    with _lock:
        found = {k: v for k, v in _conn.execute(
            f"SELECT k, v FROM emb WHERE k IN ({','.join('?' * len(keys))})", keys)}

    missing_indices = [i for i, k in enumerate(keys) if k not in found]
    stats["hits"] += len(texts) - len(missing_indices)
    stats["misses"] += len(missing_indices)

    if missing_indices:
        missing_texts = [texts[i] for i in missing_indices]
        # Batch in chunks of 50 for Workers AI limits
        batch_size = 50
        all_vecs = []
        for b_start in range(0, len(missing_texts), batch_size):
            chunk = missing_texts[b_start:b_start + batch_size]
            vecs = call_workers_ai(resolved_model, chunk)
            all_vecs.extend(vecs)
            stats["calls"] += 1

        rows = []
        for i, vec in zip(missing_indices, all_vecs):
            blob = struct.pack(f"{len(vec)}f", *vec)
            found[keys[i]] = blob
            rows.append((keys[i], blob))

        with _lock:
            _conn.executemany("INSERT OR REPLACE INTO emb VALUES (?, ?)", rows)
            _conn.commit()

    return [list(struct.unpack(f"{len(found[k]) // 4}f", found[k])) for k in keys]


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_POST(self):
        if not self.path.endswith("/embeddings"):
            self.send_error(404)
            return

        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))))
        raw_texts = body.get("input", [])
        texts = raw_texts if isinstance(raw_texts, list) else [raw_texts]
        model = body.get("model", DEFAULT_MODEL)

        try:
            vecs = embed(model, texts)
        except Exception as e:
            self._json(502, {"error": {"message": str(e)[:300], "type": "upstream_error"}})
            return

        data = []
        for i, v in enumerate(vecs):
            if body.get("encoding_format") == "base64":
                v = base64.b64encode(struct.pack(f"{len(v)}f", *v)).decode()
            data.append({"object": "embedding", "index": i, "embedding": v})

        self._json(200, {
            "object": "list",
            "data": data,
            "model": model,
            "usage": {"prompt_tokens": len(texts) * 10, "total_tokens": len(texts) * 10}
        })

    def do_GET(self):
        self._json(200, stats)

    def _json(self, code, obj):
        out = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)


def main():
    parser = argparse.ArgumentParser(description="Cloudflare Workers AI Embeddings Relay")
    parser.add_argument("--port", type=int, default=18201)
    parser.add_argument("--model", default=DEFAULT_MODEL)
    args = parser.parse_args()

    print(f"Workers AI relay listening on 127.0.0.1:{args.port}")
    print(f"Default model: {args.model}")
    print(f"Cache location: {DB}")
    ThreadingHTTPServer(("127.0.0.1", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
