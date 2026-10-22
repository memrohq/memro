#!/usr/bin/env python3
"""A caching /v1/embeddings relay to Cloudflare Workers AI, for evaluation.

Memro's server is pointed at it (EMBEDDING_BASE_URL, EMBEDDING_MODEL=@cf/...)
so the rest of the engine (pgvector, hybrid search) is unchanged and only the
embedding model differs. Benchmark-only: nothing in Memro Cloud uses it.

    python bench/cf/workers_ai_relay.py --port 18301

It forwards to Workers AI's OpenAI-compatible endpoint and caches every vector
in SQLite by (model, sha256(text)), so reruns are free. It never makes up a
vector: without Cloudflare credentials it refuses to start, an unknown model
is a 400, and an upstream error is passed back so the run fails visibly.

Credentials: CLOUDFLARE_ACCOUNT_ID and CLOUDFLARE_API_TOKEN (Workers AI read),
from the environment, bench/cf/.env or bench/.env (both gitignored).
"""
import argparse
import hashlib
import json
import os
import pathlib
import sqlite3
import struct
import sys
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ROOT = pathlib.Path(__file__).resolve().parent
CACHE = ROOT / "cache" / "workers_ai_embeddings.sqlite"

# Workers AI text-embedding models worth testing for Memro's memories (up to
# ~2,400 tokens each). max_tokens: Cloudflare model pages (embeddinggemma:
# Google model card). Prices are Cloudflare's list prices per 1M input tokens
# where published; None means not published on the model page.
MODELS = {
    "@cf/baai/bge-m3": {"max_tokens": 60000, "usd_per_m": 0.0118},
    "@cf/qwen/qwen3-embedding-0.6b": {"max_tokens": 8192, "usd_per_m": 0.0118},
    "@cf/google/embeddinggemma-300m": {"max_tokens": 2048, "usd_per_m": None},
    "@cf/baai/bge-large-en-v1.5": {"max_tokens": 512, "usd_per_m": None},
    "@cf/baai/bge-base-en-v1.5": {"max_tokens": 512, "usd_per_m": 0.0666},
    "@cf/baai/bge-small-en-v1.5": {"max_tokens": 512, "usd_per_m": None},
}
BATCH = 50  # texts per upstream call


def credentials():
    acc, token = os.environ.get("CLOUDFLARE_ACCOUNT_ID"), os.environ.get("CLOUDFLARE_API_TOKEN")
    for env in (ROOT / ".env", ROOT.parent / ".env"):  # bench/cf/.env, then bench/.env
        if not env.exists():
            continue
        for line in env.read_text().splitlines():
            k, _, v = line.strip().removeprefix("export ").partition("=")
            v = v.strip().strip('"').strip("'")
            if k == "CLOUDFLARE_ACCOUNT_ID" and not acc:
                acc = v
            elif k == "CLOUDFLARE_API_TOKEN" and not token:
                token = v
    return acc, token


class Relay:
    def __init__(self, upstream: str, token: str, cache_path: pathlib.Path = CACHE):
        self.upstream = upstream.rstrip("/") + "/embeddings"
        self.token = token
        cache_path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(cache_path, check_same_thread=False)
        self.db.execute("CREATE TABLE IF NOT EXISTS emb (k TEXT PRIMARY KEY, v BLOB)")
        self.db.commit()
        self.lock = threading.Lock()
        self.stats = {"hits": 0, "misses": 0, "upstream_calls": 0, "prompt_tokens": 0}

    @staticmethod
    def key(model, text):
        return model + ":" + hashlib.sha256(text.encode()).hexdigest()

    def _post(self, body: dict) -> dict:
        data = json.dumps(body).encode()
        delay = 2.0
        for attempt in range(6):
            req = urllib.request.Request(self.upstream, data=data, method="POST", headers={
                "Authorization": f"Bearer {self.token}", "Content-Type": "application/json",
                "User-Agent": "memro-bench-relay/1"})
            try:
                with urllib.request.urlopen(req, timeout=120) as r:
                    return json.loads(r.read())
            except urllib.error.HTTPError as e:
                msg = e.read().decode(errors="replace")[:300]
                if e.code in (429, 500, 502, 503, 504) and attempt < 5:
                    time.sleep(delay)
                    delay = min(delay * 2, 30)
                    continue
                raise RuntimeError(f"Workers AI HTTP {e.code}: {msg}")
            except urllib.error.URLError:
                if attempt < 5:
                    time.sleep(delay)
                    delay = min(delay * 2, 30)
                    continue
                raise
        raise RuntimeError("unreachable")

    def embed(self, model: str, texts: list) -> list:
        keys = [self.key(model, t) for t in texts]
        with self.lock:
            found = dict(self.db.execute(
                f"SELECT k, v FROM emb WHERE k IN ({','.join('?' * len(keys))})", keys)) if keys else {}
        todo = [i for i, k in enumerate(keys) if k not in found]
        self.stats["hits"] += len(texts) - len(todo)
        self.stats["misses"] += len(todo)
        for start in range(0, len(todo), BATCH):
            chunk = todo[start:start + BATCH]
            out = self._post({"model": model, "input": [texts[i] for i in chunk]})
            data = sorted(out.get("data", []), key=lambda d: d.get("index", 0))
            if len(data) != len(chunk):
                raise RuntimeError(f"Workers AI returned {len(data)} embeddings for {len(chunk)} inputs")
            self.stats["upstream_calls"] += 1
            self.stats["prompt_tokens"] += int((out.get("usage") or {}).get("prompt_tokens") or 0)
            rows = []
            for i, d in zip(chunk, data):
                vec = d["embedding"]
                if not vec:
                    raise RuntimeError("Workers AI returned an empty embedding")
                blob = struct.pack(f"{len(vec)}f", *vec)
                found[keys[i]] = blob
                rows.append((keys[i], blob))
            with self.lock:  # only real vectors are ever cached
                self.db.executemany("INSERT OR REPLACE INTO emb VALUES (?, ?)", rows)
                self.db.commit()
        return [list(struct.unpack(f"{len(found[k]) // 4}f", found[k])) for k in keys]


def make_handler(relay: Relay):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def _json(self, code, obj):
            out = json.dumps(obj).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(out)))
            self.end_headers()
            self.wfile.write(out)

        def do_GET(self):
            self._json(200, relay.stats)

        def do_POST(self):
            if not self.path.rstrip("/").endswith("/embeddings"):
                return self._json(404, {"error": {"message": "only /v1/embeddings"}})
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
            model = body.get("model", "")
            if model not in MODELS:
                return self._json(400, {"error": {"message": f"model {model!r} is not a Workers AI model this "
                                                             f"relay evaluates; use one of {sorted(MODELS)}"}})
            texts = body.get("input", [])
            texts = texts if isinstance(texts, list) else [texts]
            try:
                vecs = relay.embed(model, texts)
            except Exception as e:  # surface upstream failures; never substitute vectors
                return self._json(502, {"error": {"message": str(e)[:300], "type": "upstream_error"}})
            self._json(200, {"object": "list", "model": model,
                             "data": [{"object": "embedding", "index": i, "embedding": v} for i, v in enumerate(vecs)]})
    return Handler


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--port", type=int, default=18301)
    args = ap.parse_args()
    acc, token = credentials()
    if not (acc and token):
        sys.exit("CLOUDFLARE_ACCOUNT_ID and CLOUDFLARE_API_TOKEN are required (env, bench/cf/.env or bench/.env)")
    relay = Relay(f"https://api.cloudflare.com/client/v4/accounts/{acc}/ai/v1", token)
    print(f"Workers AI relay on 127.0.0.1:{args.port}, cache {CACHE}", flush=True)
    ThreadingHTTPServer(("127.0.0.1", args.port), make_handler(relay)).serve_forever()


if __name__ == "__main__":
    main()
