"""Relay tests against a stand-in Workers AI endpoint (no network)."""
import json
import pathlib
import sys
import threading
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
import workers_ai_relay as relay_mod  # noqa: E402


class Upstream:
    """Fake OpenAI-compatible endpoint: vector = [len(text), index]."""
    def __init__(self, fail_with=None, drop_one=False):
        self.calls, self.fail_with, self.drop_one = [], fail_with, drop_one
        up = self

        class H(BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                up.calls.append((self.path, self.headers.get("Authorization"), body))
                if up.fail_with:
                    self.send_response(up.fail_with)
                    self.end_headers()
                    self.wfile.write(b'{"errors":["nope"]}')
                    return
                data = [{"index": i, "embedding": [float(len(t)), float(i)]} for i, t in enumerate(body["input"])]
                if up.drop_one:
                    data = data[:-1]
                out = json.dumps({"data": data, "usage": {"prompt_tokens": 7}}).encode()
                self.send_response(200)
                self.send_header("Content-Length", str(len(out)))
                self.end_headers()
                self.wfile.write(out)

        self.srv = ThreadingHTTPServer(("127.0.0.1", 0), H)
        threading.Thread(target=self.srv.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.srv.server_port}/v1"


def serve(relay):
    srv = ThreadingHTTPServer(("127.0.0.1", 0), relay_mod.make_handler(relay))
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return f"http://127.0.0.1:{srv.server_port}"


def post(url, body):
    req = urllib.request.Request(url + "/v1/embeddings", data=json.dumps(body).encode(),
                                 headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, json.loads(r.read())
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read())


def test_forwards_caches_and_keeps_order(tmp_path):
    up = Upstream()
    r = relay_mod.Relay(up.url, "tok", tmp_path / "c.sqlite")
    url = serve(r)
    code, out = post(url, {"model": "@cf/baai/bge-m3", "input": ["a", "bbb"]})
    assert code == 200 and [d["embedding"][0] for d in out["data"]] == [1.0, 3.0]
    assert up.calls[0][0] == "/v1/embeddings" and up.calls[0][1] == "Bearer tok"
    assert up.calls[0][2]["model"] == "@cf/baai/bge-m3"
    # Second request: one cached, one new; only the new text goes upstream.
    code, out = post(url, {"model": "@cf/baai/bge-m3", "input": ["bbb", "cc"]})
    assert [d["embedding"][0] for d in out["data"]] == [3.0, 2.0]
    assert up.calls[1][2]["input"] == ["cc"]
    assert r.stats["hits"] == 1 and r.stats["prompt_tokens"] == 14


def test_cache_is_per_model(tmp_path):
    up = Upstream()
    url = serve(relay_mod.Relay(up.url, "tok", tmp_path / "c.sqlite"))
    post(url, {"model": "@cf/baai/bge-m3", "input": ["x"]})
    post(url, {"model": "@cf/qwen/qwen3-embedding-0.6b", "input": ["x"]})
    assert len(up.calls) == 2


def test_unknown_or_openai_model_is_refused_not_aliased(tmp_path):
    up = Upstream()
    url = serve(relay_mod.Relay(up.url, "tok", tmp_path / "c.sqlite"))
    for model in ("text-embedding-3-small", "bge-small", ""):
        code, out = post(url, {"model": model, "input": ["x"]})
        assert code == 400, model
    assert up.calls == []


@pytest.mark.parametrize("kw", [{"fail_with": 400}, {"drop_one": True}])
def test_upstream_problems_fail_and_cache_nothing(tmp_path, kw):
    up = Upstream(**kw)
    r = relay_mod.Relay(up.url, "tok", tmp_path / "c.sqlite")
    code, out = post(serve(r), {"model": "@cf/baai/bge-m3", "input": ["a", "b"]})
    assert code == 502 and out["error"]["type"] == "upstream_error"
    assert r.db.execute("SELECT count(*) FROM emb").fetchone()[0] == 0


def test_refuses_to_start_without_credentials(monkeypatch, tmp_path):
    monkeypatch.delenv("CLOUDFLARE_ACCOUNT_ID", raising=False)
    monkeypatch.delenv("CLOUDFLARE_API_TOKEN", raising=False)
    monkeypatch.setattr(relay_mod, "ROOT", tmp_path / "cf")
    monkeypatch.setattr(sys, "argv", ["relay"])
    with pytest.raises(SystemExit) as e:
        relay_mod.main()
    assert "required" in str(e.value)


def test_reads_bench_env(monkeypatch, tmp_path):
    monkeypatch.delenv("CLOUDFLARE_ACCOUNT_ID", raising=False)
    monkeypatch.delenv("CLOUDFLARE_API_TOKEN", raising=False)
    (tmp_path / "cf").mkdir()
    (tmp_path / ".env").write_text('export CLOUDFLARE_ACCOUNT_ID="acc"\nCLOUDFLARE_API_TOKEN=tok\n')
    monkeypatch.setattr(relay_mod, "ROOT", tmp_path / "cf")
    assert relay_mod.credentials() == ("acc", "tok")


def test_requests_stay_under_the_model_token_limit():
    # bge-small: 512 tokens -> 640-char budget per request.
    items = [(0, "a" * 300), (1, "b" * 300), (2, "c" * 300), (3, "d" * 2000), (4, "e" * 10)]
    groups = [[i for i, _ in g] for g in relay_mod.batches(items, 512)]
    assert groups == [[0, 1], [2], [3], [4]]
    # bge-m3: 60k tokens -> 75k chars; 50 texts of 2,000 chars split by size, not only count.
    many = [(i, "x" * 2000) for i in range(50)]
    assert [len(g) for g in relay_mod.batches(many, 60000)] == [37, 13]
