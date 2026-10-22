#!/usr/bin/env python3
"""Compare embedding models on LongMemEval with everything else held fixed.

For each model it starts a Memro server whose only difference is the embedding
model (EMBEDDING_BASE_URL/EMBEDDING_MODEL), runs the same questions through
bench/longmemeval/run.py, optionally grades them, and writes one comparison
table. Storage and search stay Postgres + pgvector + Memro's hybrid search.
Benchmark-only: nothing here ships in Memro Cloud.

    python bench/cf/eval_embeddings.py --retrieval-only          # embeddings only, no LLM cost
    python bench/cf/eval_embeddings.py                           # + answers and judge (~$0.20/model)
    python bench/cf/eval_embeddings.py --models @cf/baai/bge-m3 text-embedding-3-small

Needs: Postgres with pgvector (MEMRO_DATABASE_URL, default the memro-bench-db
container on 55434), Go to build the server, OPENAI_API_KEY (answers, judge,
and the OpenAI baseline), and Cloudflare credentials for @cf/ models.
"""
import argparse
import datetime as dt
import json
import os
import pathlib
import re
import socket
import statistics
import subprocess
import sys
import time
import urllib.parse
import urllib.request

CF = pathlib.Path(__file__).resolve().parent
BENCH = CF.parent
LME = BENCH / "longmemeval"
REPO = BENCH.parent
sys.path.insert(0, str(LME))
sys.path.insert(0, str(CF))
from common import RUNS, openai_key  # noqa: E402

DEFAULT_MODELS = ["text-embedding-3-small", "@cf/baai/bge-m3", "@cf/qwen/qwen3-embedding-0.6b"]
EMBED_PORT, API_PORT = 18300, 18390


def slug(model):
    return re.sub(r"[^a-z0-9.]+", "-", model.lower().removeprefix("@cf/")).strip("-")


def port_free(port):
    with socket.socket() as s:
        return s.connect_ex(("127.0.0.1", port)) != 0


def wait_http(url, seconds=60):
    deadline = time.time() + seconds
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=3):
                return
        except Exception:
            time.sleep(1)
    raise RuntimeError(f"{url} did not come up")


def get_json(url):
    with urllib.request.urlopen(url, timeout=10) as r:
        return json.loads(r.read())


def build_server(force):
    out = CF / ".bin" / "memro-api"
    if force or not out.exists():
        out.parent.mkdir(exist_ok=True)
        subprocess.run(["go", "build", "-o", str(out), "./cmd/memro-api"], cwd=REPO / "server", check=True)
    return out


def start(cmd, log, env=None, cwd=None):
    return subprocess.Popen(cmd, stdout=open(log, "w"), stderr=subprocess.STDOUT, env=env, cwd=cwd)


def stop(*procs):
    for p in procs:
        if p and p.poll() is None:
            p.terminate()
            try:
                p.wait(10)
            except subprocess.TimeoutExpired:
                p.kill()


def prefill(model, subset_file):
    """Embed every memory and question for these questions into the relay's
    cache first, straight from Workers AI with no server deadline. Slow models
    (qwen3 took ~4 s a call) otherwise time out the server's 30 s embedding
    call, and the run is full of retries. The run then reads the cache."""
    import concurrent.futures as cfut
    import workers_ai_relay as w
    from run import load_data, rounds
    ids = set(json.loads(pathlib.Path(subset_file).read_text()))
    texts = []
    for q in load_data():
        if q["question_id"] in ids:
            texts.append(q["question"][:500])
            for session in q["haystack_sessions"]:
                texts.extend(rounds(session))
    texts = list(dict.fromkeys(texts))
    acc, token = w.credentials()
    if not (acc and token):
        raise SystemExit("Cloudflare credentials are required to prefill")
    relay = w.Relay(f"https://api.cloudflare.com/client/v4/accounts/{acc}/ai/v1", token)
    chunks = [texts[i:i + 200] for i in range(0, len(texts), 200)]
    done = 0
    with cfut.ThreadPoolExecutor(4) as ex:
        for n in ex.map(lambda c: len(relay.embed(model, c)), chunks):
            done += n
            print(f"  prefill {model}: {done}/{len(texts)}", end="\r", flush=True)
    print(f"  prefill {model}: {len(texts)} texts, {relay.stats['misses']} new, {relay.stats['hits']} cached")


def fresh_database(admin_url, name):
    """Create an empty database for one model's run. Each memory keeps a single
    embedding, and a server re-embeds every memory lacking one for its model,
    so models must never share a database."""
    subprocess.run(["psql", admin_url, "-qc", f"DROP DATABASE IF EXISTS {name}", "-c", f"CREATE DATABASE {name}"],
                   check=True, capture_output=True)
    url = urllib.parse.urlparse(admin_url)._replace(path="/" + name).geturl()
    subprocess.run(["psql", url, "-qc", "CREATE EXTENSION IF NOT EXISTS vector"], check=True, capture_output=True)
    return url


def drop_database(admin_url, name):
    subprocess.run(["psql", admin_url, "-qc", f"DROP DATABASE IF EXISTS {name} WITH (FORCE)"], capture_output=True)


def run_model(model, args, server_bin):
    name = f"emb-{slug(model)}{args.suffix}"
    run_dir = RUNS / name
    run_dir.mkdir(parents=True, exist_ok=True)
    for port in (EMBED_PORT, API_PORT):
        if not port_free(port):
            raise SystemExit(f"port {port} is in use; stop whatever is on it first")

    cloudflare = model.startswith("@cf/")
    embed_cmd = ([sys.executable, str(CF / "workers_ai_relay.py"), "--port", str(EMBED_PORT)] if cloudflare
                 else [sys.executable, str(LME / "embed_proxy.py"), str(EMBED_PORT)])
    embed = api = None
    db_name = "memro_emb_" + re.sub(r"[^a-z0-9]+", "_", slug(model))
    db_url = fresh_database(args.db_url, db_name)
    try:
        embed = start(embed_cmd, run_dir / "embed.log", cwd=LME)
        time.sleep(1.5)
        if embed.poll() is not None:
            raise SystemExit(f"embedding service exited:\n{(run_dir / 'embed.log').read_text()[-500:]}")
        env = {**os.environ,
               "DATABASE_URL": db_url + ("&" if "?" in db_url else "?") + "sslmode=disable",
               "PORT": str(API_PORT), "RATE_LIMIT_MAX": "50000",
               "FILE_STORAGE_PATH": str(CF / ".bin" / "files"),
               "EMBEDDING_PROVIDER": "openai", "EMBEDDING_BASE_URL": f"http://127.0.0.1:{EMBED_PORT}",
               "EMBEDDING_MODEL": model, "OPENAI_API_KEY": openai_key()}
        # A fresh server per model: its query-embedding cache must not carry over.
        api = start([str(server_bin)], run_dir / "api.log", env=env)
        wait_http(f"http://127.0.0.1:{API_PORT}/health")

        run_env = {**os.environ, "MEMRO_API": f"http://127.0.0.1:{API_PORT}",
                   "MEMRO_DATABASE_URL": db_url, "BENCH_EMBEDDING_MODEL": model}
        if args.retrieval_only:
            run_env["BENCH_NO_LLM"] = "1"
        hyp = run_dir / "hyp.jsonl"
        before = len(hyp.read_text().splitlines()) if hyp.exists() else 0
        subprocess.run([sys.executable, str(LME / "run.py"), "--subset", str(args.subset), "--name", name,
                        "--workers", str(args.workers)], env=run_env, cwd=LME, check=True)
        ran = len(hyp.read_text().splitlines()) - before
        if cloudflare and ran:  # a resumed, already-complete run embeds nothing
            stats = get_json(f"http://127.0.0.1:{EMBED_PORT}/")
            if stats["hits"] + stats["misses"] == 0:
                raise SystemExit(f"{model}: the server never called the relay; results would be mislabeled")
            (run_dir / "embed_stats.json").write_text(json.dumps(stats, indent=2) + "\n")
    finally:
        stop(api, embed)
        drop_database(args.db_url, db_name)
    # The server retries failed embedding batches, so a run can finish with
    # errors along the way; count them so the report can't look clean.
    errors = (run_dir / "api.log").read_text().count("embedding batch failed")
    (run_dir / "server_embed_errors.txt").write_text(f"{errors}\n")
    if not args.retrieval_only:
        subprocess.run([sys.executable, str(LME / "judge.py"), str(run_dir)], cwd=LME, check=True)
    return run_dir


def metrics(run_dir):
    rows = [json.loads(l) for l in (run_dir / "hyp.jsonl").read_text().splitlines() if l.strip()]
    ev = [r for r in rows if r.get("evidence_total")]
    def recall_at(k):
        return statistics.mean(sum(r["hit_evidence"][:k]) / r["evidence_total"] for r in ev) if ev else 0.0
    def mrr(r):
        return next((1 / (i + 1) for i, s in enumerate(r["retrieved_sessions"]) if s in r["answer_session_ids"]), 0.0)
    gold = [r for r in rows if r["answer_session_ids"]]
    eval_file = run_dir / "eval-gpt-4o-mini.jsonl"
    labels = {json.loads(l)["question_id"]: json.loads(l)["label"]
              for l in eval_file.read_text().splitlines() if l.strip()} if eval_file.exists() else {}
    stats_file = run_dir / "embed_stats.json"
    return {
        "n": len(rows),
        "accuracy": (sum(labels.get(r["question_id"], False) for r in rows) / len(rows)) if labels else None,
        "ev_r5": recall_at(5), "ev_r15": recall_at(15),
        "all_ev_15": sum(sum(r["hit_evidence"]) == r["evidence_total"] for r in ev) / len(ev) if ev else 0.0,
        "all_ev_ctx": sum(r["evidence_in_context"] == r["evidence_total"] for r in ev) / len(ev) if ev else 0.0,
        "sess_r5": sum(any(s in r["answer_session_ids"] for s in r["retrieved_sessions"][:5]) for r in gold) / len(gold),
        "mrr": statistics.mean(mrr(r) for r in gold),
        "p50_ms": statistics.median(r["search_ms"] for r in rows),
        "embed_wait_s": statistics.mean(r["embed_wait_s"] for r in rows),
        "embed_stats": json.loads(stats_file.read_text()) if stats_file.exists() else None,
        "embed_errors": int((run_dir / "server_embed_errors.txt").read_text()) if (run_dir / "server_embed_errors.txt").exists() else 0,
    }


def report(results, args):
    pct = lambda x: "–" if x is None else f"{x * 100:.1f}%"
    lines = [f"# Embedding comparison: {dt.date.today()}", "",
             f"LongMemEval_S, {args.subset.name}. Same Memro server, Postgres + pgvector and hybrid search; "
             "only the embedding model differs. Answers and judge: gpt-4o-mini.", "",
             "Evidence recall counts the exact exchanges LongMemEval marks as holding the answer "
             "(session-level recall is ~99% for every model and doesn't separate them).", "",
             "| Embedding model | n | QA accuracy | Evidence R@5 | Evidence R@15 | All evidence in top 15 "
             "| All evidence in prompt | Session R@5 | Session MRR | Search p50 | Embed wait | Embed errors |",
             "|---|---|---|---|---|---|---|---|---|---|---|---|"]
    for model, m in results.items():
        lines.append(f"| `{model}` | {m['n']} | {pct(m['accuracy'])} | {pct(m['ev_r5'])} | {pct(m['ev_r15'])} | "
                     f"{pct(m['all_ev_15'])} | {pct(m['all_ev_ctx'])} | {pct(m['sess_r5'])} | {m['mrr']:.3f} | "
                     f"{m['p50_ms']:.0f} ms | {m['embed_wait_s']:.1f} s | {m['embed_errors']} |")
    lines += ["", "Workers AI usage (embeddings not already cached):", ""]
    for model, m in results.items():
        if m["embed_stats"]:
            s = m["embed_stats"]
            lines.append(f"- `{model}`: {s['upstream_calls']} calls, {s['misses']} texts embedded, "
                         f"{s['hits']} from cache, about {s.get('est_tokens', 0):,} tokens (estimated)")
    lines += ["", "With 100 questions, one question is 1 point of accuracy: confirm a winner on all 500 "
              "(`--subset` with the full ID list) before acting on it."]
    return "\n".join(lines) + "\n"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--models", nargs="+", default=DEFAULT_MODELS)
    ap.add_argument("--subset", type=pathlib.Path, default=LME / "subset_100.json")
    ap.add_argument("--retrieval-only", action="store_true", help="skip answering and judging (no OpenAI chat cost)")
    ap.add_argument("--suffix", default="", help="appended to run names, e.g. -r2 for a repeat")
    ap.add_argument("--db-url", default=os.environ.get("MEMRO_DATABASE_URL",
                                                       "postgres://memro:memro@127.0.0.1:55434/memro"),
                    help="admin connection; each model gets its own fresh database next to it")
    ap.add_argument("--workers", type=int, default=6)
    ap.add_argument("--build", action="store_true", help="rebuild the server binary")
    ap.add_argument("--server-bin", help="use this memro-api binary (e.g. built from another branch)")
    ap.add_argument("--prefill", action="store_true", help="embed everything into the Workers AI cache before each run")
    args = ap.parse_args()

    server_bin = pathlib.Path(args.server_bin) if args.server_bin else build_server(args.build)
    results = {}
    for model in args.models:
        print(f"== {model}", flush=True)
        if args.prefill and model.startswith("@cf/"):
            prefill(model, args.subset)
        results[model] = metrics(run_model(model, args, server_bin))
    text = report(results, args)
    out = RUNS / f"EMBEDDINGS_{dt.date.today()}{args.suffix}.md"
    out.write_text(text)
    print(text)
    print(f"-> {out}")


if __name__ == "__main__":
    main()
