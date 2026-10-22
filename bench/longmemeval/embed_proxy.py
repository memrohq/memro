"""A caching OpenAI-compatible /v1/embeddings relay for the benchmark.

Memro's server is pointed at it (EMBEDDING_BASE_URL). Every text's embedding
is cached in SQLite by (model, sha256(text)), so the same chat session shared
by several LongMemEval questions, and every rerun, is embedded once. Each
upstream call goes through the cost ledger and is refused past the budget.
Results are unchanged: the same model returns the same vector for the same
text.
"""
import base64
import hashlib
import json
import os
import sqlite3
import struct
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from common import RUNS, BudgetExceeded, check_budget, openai_post, record

DB = RUNS / "embed_cache.sqlite"
_lock = threading.Lock()
_conn = sqlite3.connect(DB, check_same_thread=False)
_conn.execute("CREATE TABLE IF NOT EXISTS emb (k TEXT PRIMARY KEY, v BLOB)")
_conn.commit()
stats = {"hits": 0, "misses": 0}


def _key(model, text):
    return model + ":" + hashlib.sha256(text.encode()).hexdigest()


def fake_vector(text, dims=1536):
    """Offline dry runs (BENCH_FAKE_EMBED=1): a bag-of-words vector, no API call."""
    import math
    v = [0.0] * dims
    for w in text.lower().split():
        v[int(hashlib.md5(w.strip(".,!?").encode()).hexdigest(), 16) % dims] += 1
    n = math.sqrt(sum(x * x for x in v)) or 1.0
    return [x / n for x in v]


def embed(model, texts):
    if os.environ.get("BENCH_FAKE_EMBED"):
        return [fake_vector(t) for t in texts]
    if os.environ.get("BENCH_LOG_QUERIES"):
        with open(RUNS / "relay_queries.log", "a") as f:
            for t in texts:
                if len(t) < 300:
                    f.write(repr(t) + "\n")
    keys = [_key(model, t) for t in texts]
    with _lock:
        found = {k: v for k, v in _conn.execute(
            f"SELECT k, v FROM emb WHERE k IN ({','.join('?' * len(keys))})", keys)}
    missing = [i for i, k in enumerate(keys) if k not in found]
    stats["hits"] += len(texts) - len(missing)
    stats["misses"] += len(missing)
    if missing:
        est = sum(len(texts[i]) for i in missing) / 3.0 * 0.02 / 1e6  # generous token estimate
        check_budget(est)
        r = openai_post("/v1/embeddings", {"model": model, "input": [texts[i] for i in missing]})
        record(model, r.get("usage", {}).get("prompt_tokens", 0))
        rows = []
        for item, i in zip(sorted(r["data"], key=lambda d: d["index"]), missing):
            vec = item["embedding"]
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
        texts = body["input"] if isinstance(body["input"], list) else [body["input"]]
        try:
            vecs = embed(body["model"], texts)
        except BudgetExceeded as e:
            self._json(402, {"error": {"message": str(e), "type": "budget"}})
            return
        except Exception as e:  # upstream failure: Memro retries on backfill
            self._json(502, {"error": {"message": str(e)[:300]}})
            return
        data = []
        for i, v in enumerate(vecs):
            if body.get("encoding_format") == "base64":
                v = base64.b64encode(struct.pack(f"{len(v)}f", *v)).decode()
            data.append({"object": "embedding", "index": i, "embedding": v})
        self._json(200, {"object": "list", "data": data, "model": body["model"],
                         "usage": {"prompt_tokens": 0, "total_tokens": 0}})

    def do_GET(self):
        self._json(200, stats)

    def _json(self, code, obj):
        out = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 18200
    print(f"embedding relay on 127.0.0.1:{port}, cache {DB}", flush=True)
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
