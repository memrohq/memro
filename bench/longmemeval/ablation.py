"""Controlled 4-arm ablation runner for graph retrieval & temporal conflict resolution.

Evaluates 4 arms on identical questions, dataset snapshots, models, prompts, and server settings:
  - Arm A (baseline):        Hybrid only (graph=None, resolve_conflicts=False)
  - Arm B (legacy graph):    Hybrid + legacy graph traversal (graph=legacy, resolve_conflicts=False)
  - Arm C (bounded graph):   Hybrid + PR #43 bounded traversal (graph=bounded, resolve_conflicts=False)
  - Arm D (graph + temporal): Hybrid + PR #43 bounded traversal + temporal resolution (graph=bounded, resolve_conflicts=True)

Usage:
    python ablation.py --pilot            # 12 questions (2 per category)
    python ablation.py --subset           # 100-question stratified subset
    python ablation.py --subset --no-llm  # dry-run retrieval only (measure recall & latency)
"""
import argparse
import concurrent.futures as cf
import datetime as dt
import json
import os
import pathlib
import sys
import threading
import time

from common import BENCH, ROOT, RUNS, RunTracker, set_run_tracker
import run as base_run

ALL_ARMS = [
    {
        "id": "arm-a",
        "name": "ablation-arm-a",
        "desc": "Arm A: Baseline Hybrid (no graph, no conflict resolution)",
        "graph": None,
        "resolve_conflicts": False,
    },
    {
        "id": "arm-b",
        "name": "ablation-arm-b",
        "desc": "Arm B: Hybrid + Legacy Graph (pre-PR #43, drops short dead ends)",
        "graph": "legacy",
        "resolve_conflicts": False,
    },
    {
        "id": "arm-c",
        "name": "ablation-arm-c",
        "desc": "Arm C: Hybrid + PR #43 Bounded Graph (preserves short terminal paths)",
        "graph": "bounded",
        "resolve_conflicts": False,
    },
    {
        "id": "arm-d",
        "name": "ablation-arm-d",
        "desc": "Arm D: PR #43 Bounded Graph + Temporal Resolution",
        "graph": "bounded",
        "resolve_conflicts": True,
    },
    {
        "id": "arm-e",
        "name": "ablation-arm-e",
        "desc": "Arm E: Graph Degree Reranking (zero new injected nodes)",
        "graph": "rerank",
        "resolve_conflicts": False,
    },
    {
        "id": "arm-f",
        "name": "ablation-arm-f",
        "desc": "Arm F: Selective Graph (query-intent gated expansion & rerank)",
        "graph": "selective",
        "resolve_conflicts": False,
    },
    {
        "id": "arm-g",
        "name": "ablation-arm-g",
        "desc": "Arm G: Graph Rerank + Adaptive Session Diversity (PR 22)",
        "graph": "rerank",
        "adaptive_diversity": True,
        "resolve_conflicts": False,
    },
    {
        "id": "arm-h",
        "name": "ablation-arm-h",
        "desc": "Arm H: Selective Graph + Adaptive Session Diversity (PR 22)",
        "graph": "selective",
        "adaptive_diversity": True,
        "resolve_conflicts": False,
    },
]
ARMS = ALL_ARMS


def setup_arm_dirs(base_dir: pathlib.Path, arms=None):
    if arms is None:
        arms = ARMS
    arm_dirs = {}
    for arm in arms:
        d = base_dir / arm["name"]
        d.mkdir(parents=True, exist_ok=True)
        meta = {
            "name": arm["name"],
            "description": arm["desc"],
            "arm_id": arm["id"],
            "graph": arm["graph"],
            "resolve_conflicts": arm["resolve_conflicts"],
            "adaptive_diversity": arm.get("adaptive_diversity", False),
            "commit": base_run.get_server_commit(),
            "created_at": dt.datetime.now(dt.timezone.utc).isoformat(),
        }
        (d / "meta.json").write_text(json.dumps(meta, indent=2))
        arm_dirs[arm["id"]] = d
    return arm_dirs


def load_finished(arm_dirs):
    finished = {}
    for arm_id, d in arm_dirs.items():
        hyp_file = d / "hyp.jsonl"
        done = set()
        if hyp_file.exists():
            for line in hyp_file.read_text().splitlines():
                if line.strip():
                    try:
                        done.add(json.loads(line)["question_id"])
                    except Exception:
                        pass
        finished[arm_id] = done
    # A question is considered done only if all 4 arms have completed it
    all_done = set.intersection(*finished.values()) if finished else set()
    return all_done, finished


def run_question_all_arms(q, client, no_llm=False):
    agent = base_run.new_agent(client)
    t0 = time.time()
    meta = {}
    session_memories = {}
    try:
        # 1. Ingest haystack once for all arms
        for sid, date, session in zip(q["haystack_session_ids"], q["haystack_dates"], q["haystack_sessions"]):
            when = base_run.parse_date(date)
            session_memories[sid] = []
            sess_uuid = str(base_run.uuid.uuid5(base_run.NS_SESSION, f"{q['question_id']}:{sid}"))
            for text in base_run.rounds(session):
                m = client.create_memory(
                    agent.agent_id,
                    agent._private_key,
                    text,
                    "episodic",
                    event_date=when.strftime("%Y-%m-%dT%H:%M:%SZ"),
                    session_id=sess_uuid,
                )
                meta[m["id"]] = {"session": sid, "date": date, "content": text}
                session_memories[sid].append(m["id"])
        ingest_s = time.time() - t0

        # Wait for embeddings
        deadline = time.time() + 1800
        while (left := base_run.pending_embeddings(agent.agent_id)) and time.time() < deadline:
            time.sleep(1.5)
        if left:
            raise RuntimeError(f"{left} memories still not embedded")
        embed_s = time.time() - t0 - ingest_s

        # 2. Query each arm against identical agent & edge state
        arm_results = {}
        for arm in ARMS:
            t_search = time.time()
            resp = client.search(
                agent.agent_id,
                q["question"][:500],
                limit=base_run.TOP_K,
                private_key=agent._private_key,
                mode="hybrid",
                order="score",
                resolve_conflicts=arm["resolve_conflicts"],
                graph=arm["graph"],
                adaptive_diversity=arm.get("adaptive_diversity", False),
                return_response=True,
            )
            search_ms = (time.time() - t_search) * 1000
            hits = resp.get("results", [])
            graph_diag = resp.get("graph")

            if no_llm:
                hyp = "(no LLM: retrieval dry-run)"
            else:
                hyp = base_run.answer(q, hits, meta, session_memories)

            retrieved_sessions = [meta[h["memory_id"]]["session"] for h in hits if h["memory_id"] in meta]

            arm_results[arm["id"]] = {
                "question_id": q["question_id"],
                "question_type": q["question_type"],
                "question": q["question"],
                "answer": q["answer"],
                "hypothesis": hyp,
                "retrieved_sessions": retrieved_sessions,
                "answer_session_ids": q["answer_session_ids"],
                "memories": len(meta),
                "ingest_s": round(ingest_s, 1),
                "embed_wait_s": round(embed_s, 1),
                "search_ms": round(search_ms, 1),
                "graph_diag": graph_diag,
                "graph_result_count": sum(1 for h in hits if h.get("graph") or h.get("from_graph")),
            }

        return arm_results

    finally:
        base_run.drop_agent(agent)


def sample_created_edges(sample_size=20):
    """Inspect created edges across recently created memories for precision audit."""
    sql = (
        "SELECT r.relation_type, m1.content, m2.content "
        "FROM memory_relations r "
        "JOIN memories m1 ON r.source_memory_id = m1.id "
        "JOIN memories m2 ON r.target_memory_id = m2.id "
        f"ORDER BY r.created_at DESC LIMIT {sample_size}"
    )
    try:
        raw = base_run.psql(sql)
        lines = [l for l in raw.splitlines() if l.strip()]
        return lines
    except Exception as e:
        return [f"Error sampling edges: {e}"]


def main():
    global ARMS
    parser = argparse.ArgumentParser(description="Controlled Multi-Arm Ablation for Memro Graph Retrieval")
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--pilot", action="store_true", help="12 questions (2 per category)")
    group.add_argument("--subset", nargs="?", const="subset_100.json", default=None, help="100-question subset")
    group.add_argument("--all", action="store_true", help="All 500 questions")
    parser.add_argument("--arms", default="a,b,c,d,e,f", help="Comma-separated arm letters, e.g. 'a,c,e,f'")
    parser.add_argument("--workers", type=int, default=6, help="Concurrent worker count (default: 6)")
    parser.add_argument("--no-llm", action="store_true", help="Skip answer LLM (test retrieval & latency only)")
    parser.add_argument("--output-dir", default=str(RUNS), help="Base output directory for runs")
    args = parser.parse_args()

    arm_letters = set(x.strip().lower() for x in args.arms.split(",") if x.strip())
    ARMS = [a for a in ALL_ARMS if a["id"].split("-")[1] in arm_letters]
    if not ARMS:
        print(f"ERROR: No valid arms matched --arms={args.arms}", file=sys.stderr)
        sys.exit(1)

    data = base_run.load_data()
    if args.pilot:
        by_type = {}
        for q in sorted(data, key=lambda q: q["question_id"]):
            by_type.setdefault(q["question_type"], [])
            if len(by_type[q["question_type"]]) < 2:
                by_type[q["question_type"]].append(q)
        questions = [q for qs in by_type.values() for q in qs]
    elif args.subset:
        subset_file = pathlib.Path(args.subset)
        if not subset_file.exists():
            for c in [ROOT / args.subset, BENCH / args.subset, ROOT / "subset_100.json", BENCH / "subset_100.json"]:
                if c.exists():
                    subset_file = c
                    break
        subset_ids = set(json.loads(subset_file.read_text()))
        questions = [q for q in data if q["question_id"] in subset_ids]
    else:
        questions = data

    base_dir = pathlib.Path(args.output_dir)
    arm_dirs = setup_arm_dirs(base_dir, arms=ARMS)
    all_done, _ = load_finished(arm_dirs)
    todo = [q for q in questions if q["question_id"] not in all_done]

    print(f"=== Memro Graph Ablation Benchmark ===")
    print(f"Total questions: {len(questions)} | Already completed across active arms: {len(all_done)} | To run: {len(todo)}")
    print(f"Target arms: {[a['id'] for a in ARMS]}")
    print(f"Concurrency: {args.workers} workers | No-LLM mode: {args.no_llm}")
    print()

    client = base_run.MemroClient(base_run.API, api_key=None)
    lock = threading.Lock()

    def process_q(q):
        t_start = time.time()
        results = run_question_all_arms(q, client, no_llm=args.no_llm)
        elapsed = time.time() - t_start

        with lock:
            for arm in ARMS:
                arm_id = arm["id"]
                hyp_path = arm_dirs[arm_id] / "hyp.jsonl"
                with open(hyp_path, "a") as f:
                    f.write(json.dumps(results[arm_id]) + "\n")

            lat_strs = " ".join(f"{arm['id'][-1].upper()}={results[arm['id']]['search_ms']:4.0f}ms" for arm in ARMS)
            intent = ""
            for arm in ARMS:
                if results[arm["id"]].get("graph_diag", {}).get("query_intent"):
                    intent = f"| intent={results[arm['id']]['graph_diag']['query_intent']} "
                    break

            print(
                f"[{q['question_id'][:8]}] {q['question_type']:22s} "
                f"mem={results[ARMS[0]['id']]['memories']:3d} "
                f"search: {lat_strs} {intent}| total={elapsed:4.1f}s",
                flush=True,
            )

    if todo:
        with cf.ThreadPoolExecutor(max_workers=args.workers) as pool:
            futures = [pool.submit(process_q, q) for q in todo]
            for f in cf.as_completed(futures):
                try:
                    f.result()
                except Exception as e:
                    print(f"ERROR: {e}", file=sys.stderr, flush=True)

    print()
    print("=== Ablation Run Complete ===")
    print("Arm directories written:")
    for arm in ARMS:
        hyp_file = arm_dirs[arm["id"]] / "hyp.jsonl"
        count = len([l for l in hyp_file.read_text().splitlines() if l.strip()]) if hyp_file.exists() else 0
        print(f"  - {arm['name']}: {count} questions -> {hyp_file}")

    print()
    print("Edge Sample Precision Audit:")
    samples = sample_created_edges(10)
    for s in samples:
        print(f"  {s[:120]}")


if __name__ == "__main__":
    main()
