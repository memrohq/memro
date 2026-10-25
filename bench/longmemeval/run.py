"""LongMemEval_S on Memro: ingest each question's chat history into a fresh
agent, retrieve with Memro's hybrid search, and answer with an LLM.

    python run.py --pilot            # 2 questions per type (12)
    python run.py --subset           # 100-question stratified subset
    python run.py --all              # all 500 (resumes; finished ids are skipped)

Writes runs/<name>/hyp.jsonl. Grade with judge.py, summarise with report.py.
"""
import argparse
import concurrent.futures as cf
import datetime as dt
import json
import os
import pathlib
import subprocess
import sys
import threading
import time
import uuid

from common import BENCH, PRICES, ROOT, RUNS, BudgetExceeded, RunTracker, chat, set_run_tracker

sdk_path = os.environ.get("MEMRO_SDK")
if not sdk_path:
    for candidate in [
        BENCH.parent / "memro" / "sdk" / "python" / "src",
        BENCH.parent / "sdk" / "python" / "src",
        ROOT.parent.parent / "sdk" / "python" / "src",
    ]:
        if candidate.exists():
            sdk_path = str(candidate)
            break
if sdk_path:
    sys.path.insert(0, sdk_path)
from memro import Agent, MemroClient  # noqa: E402
from memro.crypto import generate_keypair  # noqa: E402

DATA = BENCH / "data" / "longmemeval_s_cleaned.json"
API = os.environ.get("MEMRO_API", "http://127.0.0.1:18090")
DB_CONTAINER = os.environ.get("MEMRO_DB_CONTAINER", "memro-bench-db")
TOP_K = int(os.environ.get("TOP_K", "15"))
PER_ITEM_CHARS = int(os.environ.get("PER_ITEM_CHARS", "4000"))   # each retrieved memory, as shown to the answering model
MAX_MEMORY_CHARS = 9500  # Memro accepts up to 10,000 characters per memory
ANSWER_MODEL = os.environ.get("ANSWER_MODEL", "gpt-4o-mini")
SESSION_EXPANSION = 3  # answer() adds every exchange from the top N retrieved sessions
NS_SESSION = uuid.UUID("6f1c2a8e-5b8f-4c4e-9d55-0a4f7d1c3b21")


def parse_date(s):
    """'2023/05/20 (Sat) 02:21' -> datetime."""
    return dt.datetime.strptime(s.split(" (")[0] + " " + s.split(") ")[1], "%Y/%m/%d %H:%M")


def rounds(session):
    """One memory per exchange: a user turn and the assistant reply after it."""
    out, i = [], 0
    while i < len(session):
        t = session[i]
        if t["role"] == "user":
            text = "User: " + t["content"]
            if i + 1 < len(session) and session[i + 1]["role"] == "assistant":
                text += "\nAssistant: " + session[i + 1]["content"]
                i += 1
        else:
            text = "Assistant: " + t["content"]
        out.append(text)
        i += 1
    pieces = []
    for text in out:
        for start in range(0, len(text), MAX_MEMORY_CHARS):
            pieces.append(text[start:start + MAX_MEMORY_CHARS])
    return pieces


def psql(sql):
    db_url = os.environ.get("MEMRO_DATABASE_URL")
    if db_url:
        out = subprocess.run(["psql", db_url, "-tAc", sql], capture_output=True, text=True, check=True)
        return out.stdout.strip()
    out = subprocess.run(["docker", "exec", DB_CONTAINER, "psql", "-U", "memro", "-d", "memro", "-tAc", sql],
                         capture_output=True, text=True, check=True)
    return out.stdout.strip()


def load_data():
    if os.environ.get("LONGMEMEVAL_DATA"):
        p = pathlib.Path(os.environ["LONGMEMEVAL_DATA"])
        if p.exists():
            return json.loads(p.read_text())
    candidates = [
        BENCH / "data" / "longmemeval_s_cleaned.json",
        ROOT / "data" / "longmemeval_s_cleaned.json",
        BENCH / "data" / "subset_100.json",
        ROOT / "data" / "subset_100.json",
        BENCH / "data" / "subset_100.json.gz",
        ROOT / "data" / "subset_100.json.gz",
        pathlib.Path.home() / "Documents/work/memro/memro-benchmark/data/longmemeval_s_cleaned.json",
        pathlib.Path.home() / "Documents/work/memro/memro-benchmark/data/subset_100.json",
    ]
    for p in candidates:
        if p.exists():
            if str(p).endswith(".gz"):
                import gzip
                with gzip.open(p, "rt") as f:
                    return json.load(f)
            return json.loads(p.read_text())
    raise FileNotFoundError(f"Could not find dataset in any of: {[str(p) for p in candidates]}")


def pending_embeddings(agent_id):
    return int(psql("SELECT count(*) FROM memories m LEFT JOIN memory_embeddings e ON e.memory_id = m.id "
                    f"WHERE m.agent_id = '{agent_id}' AND e.memory_id IS NULL"))


def new_agent(client):
    """A fresh agent for one question. Registered straight in the benchmark
    database: the API allows 20 new agents an hour per address, and this run
    needs 500. Requests are still signed with the agent's own key."""
    pub, priv = generate_keypair()
    psql(f"INSERT INTO agent_identities (id, tenant_id) VALUES ('{pub}', '00000000-0000-0000-0000-000000000000')")
    return Agent(agent_id=pub, private_key=priv, client=client)


def drop_agent(agent):
    psql(f"DELETE FROM agent_identities WHERE id = '{agent.agent_id}'")  # cascades to its memories


def get_server_commit():
    c = os.environ.get("MEMRO_COMMIT")
    if c:
        return c
    for path in [BENCH.parent / "memro", BENCH.parent, pathlib.Path.cwd()]:
        try:
            r = subprocess.run(["git", "rev-parse", "HEAD"], cwd=path, capture_output=True, text=True)
            if r.returncode == 0 and r.stdout.strip():
                return r.stdout.strip()
        except Exception:
            pass
    return "unknown"


def get_engine_options():
    return {
        "top_k": TOP_K,
        "mode": "hybrid",
        "answer_model": ANSWER_MODEL,
        "session_expansion": SESSION_EXPANSION,
        "format": os.environ.get("RECALL_FORMAT", "standard"),
        "write_time_supersession": os.environ.get("MEMRO_WRITE_TIME_SUPERSESSION", "false"),
        "per_item_chars": PER_ITEM_CHARS,
        "resolve_conflicts": os.environ.get("RESOLVE_CONFLICTS", "true").lower() == "true",
        "order": os.environ.get("SEARCH_ORDER", "score"),
    }


ANSWER_PROMPT = """You are a helpful assistant with a long-term memory of past chat sessions with the user.
Below are excerpts retrieved from that memory, each with the date of its session and its time relative to the current date. The current date is {now}.

{context}

Answer the user's question using the excerpts. Follow these rules carefully:
1. Personalization: If the user is asking for recommendations, advice, or suggestions, actively check the excerpts for the user's personal tastes, equipment, ingredients, preferences, or past experiences, and tailor your answer directly to them. Never claim you lack details if their personal context appears in the excerpts.
2. Temporal Reasoning: Pay close attention to dates and relative days. When calculating elapsed days between two dates (Date 1 and Date 2), calculate step-by-step: (days remaining in Month 1) + (days in Month 2). For example, between Jan 2 and Feb 1: January has 31 days, so 31 - 2 = 29 days in January, plus 1 day in February = 30 days.
3. Multi-Session Synthesis: When asked to count ('how many'), list, or aggregate events or items across sessions, inspect all excerpts and count each distinct event or item mentioned. Check dates carefully.
4. Knowledge Updates: If the question asks what was true initially vs what is true now, or if a count, role, status, preference, or fact changed over time, use the chronological session dates to distinguish the initial state from the newest updated state.
5. If the excerpts do not contain the needed information, say that you don't know.

Think briefly, step by step, especially about dates, user context, and which information is most recent, then end with a line "Answer: <your answer>".

Question: {question}"""


def format_session_header(session_date_str, now_str):
    try:
        s_dt = parse_date(session_date_str)
        n_dt = parse_date(now_str)
        delta_days = (n_dt.date() - s_dt.date()).days
        if delta_days == 0:
            rel = "same day as current date"
        elif delta_days == 1:
            rel = "1 day before current date"
        elif delta_days > 1:
            rel = f"{delta_days} days before current date"
        elif delta_days == -1:
            rel = "1 day after current date"
        else:
            rel = f"{-delta_days} days after current date"
        return f"[Session date: {session_date_str} ({rel})]"
    except Exception:
        return f"[Session date: {session_date_str}]"


def context_ids(hits, meta, session_memories):
    """Memory ids shown to the answering model, in prompt order: the hits plus
    every exchange from the top SESSION_EXPANSION sessions, sorted by date."""
    hit_ids = [h["memory_id"] for h in hits]

    # Identify the top unique sessions from hits
    top_sessions = []
    for h in hits:
        sid = meta[h["memory_id"]]["session"]
        if sid not in top_sessions:
            top_sessions.append(sid)
        if len(top_sessions) >= SESSION_EXPANSION:
            break

    # Add sibling rounds for those sessions
    expanded_ids = list(hit_ids)
    for sid in top_sessions:
        for mid in session_memories.get(sid, []):
            if mid not in expanded_ids:
                expanded_ids.append(mid)
    return sorted(expanded_ids, key=lambda mid: meta[mid]["date"])


def build_prompt(q, ids, meta, superseded_ids=None):
    if superseded_ids is None:
        superseded_ids = set()
    ctx = []
    for mid in ids:
        m = meta[mid]
        header = format_session_header(m["date"], q["question_date"])
        if mid in superseded_ids:
            header += " [SUPERSEDED by newer update]"
        ctx.append(f"{header}\n{m['content'][:PER_ITEM_CHARS]}")
    return ANSWER_PROMPT.format(now=q["question_date"], context="\n\n---\n\n".join(ctx) or "(nothing found)",
                                question=q["question"])


def ask(prompt, model=ANSWER_MODEL):
    pin, pout = PRICES[model]
    est = len(prompt) / 3.0 * pin / 1e6 + 400 * pout / 1e6
    return chat(model, [{"role": "user", "content": prompt}], 400, est)


def answer(q, hits, meta, session_memories):
    superseded_ids = {h["memory_id"] for h in hits if h.get("is_superseded")}
    return ask(build_prompt(q, context_ids(hits, meta, session_memories), meta, superseded_ids=superseded_ids))


def retrieve(q, client, agent):
    """Ingest q's haystack into agent, wait for embeddings, and search.
    Returns (hits, meta, session_memories, timings)."""
    t0 = time.time()
    meta = {}
    session_memories = {}
    for sid, date, session in zip(q["haystack_session_ids"], q["haystack_dates"], q["haystack_sessions"]):
        when = parse_date(date)
        session_memories[sid] = []
        sess_uuid = str(uuid.uuid5(NS_SESSION, f"{q['question_id']}:{sid}"))
        for text in rounds(session):
            m = client.create_memory(agent.agent_id, agent._private_key, text, "episodic",
                                     event_date=when.strftime("%Y-%m-%dT%H:%M:%SZ"),
                                     session_id=sess_uuid)
            meta[m["id"]] = {"session": sid, "date": date, "content": text}
            session_memories[sid].append(m["id"])
    ingest_s = time.time() - t0
    deadline = time.time() + 1800
    while (left := pending_embeddings(agent.agent_id)) and time.time() < deadline:
        time.sleep(2)
    if left:
        raise RuntimeError(f"{left} memories still not embedded")
    embed_s = time.time() - t0 - ingest_s
    t1 = time.time()
    resolve_conflicts = os.environ.get("RESOLVE_CONFLICTS", "true").lower() == "true"
    search_order = os.environ.get("SEARCH_ORDER", "score")
    hits = client.search(
        agent.agent_id,
        q["question"][:500],
        limit=TOP_K,
        private_key=agent._private_key,
        mode="hybrid",
        order=search_order,
        resolve_conflicts=resolve_conflicts,
    )
    search_ms = (time.time() - t1) * 1000
    return hits, meta, session_memories, {"ingest_s": round(ingest_s, 1), "embed_wait_s": round(embed_s, 1),
                                          "search_ms": round(search_ms, 1)}


def run_one(q):
    client = MemroClient(API, api_key=None)
    agent = new_agent(client)
    try:
        hits, meta, session_memories, timings = retrieve(q, client, agent)
        hyp = "(no LLM: dry run)" if os.environ.get("BENCH_NO_LLM") else answer(q, hits, meta, session_memories)
        return {
            "question_id": q["question_id"], "question_type": q["question_type"], "question": q["question"],
            "answer": q["answer"], "hypothesis": hyp,
            "retrieved_sessions": [meta[h["memory_id"]]["session"] for h in hits],
            "answer_session_ids": q["answer_session_ids"], "memories": len(meta),
            **timings,
        }
    finally:
        drop_agent(agent)


def main():
    ap = argparse.ArgumentParser()
    g = ap.add_mutually_exclusive_group(required=True)
    g.add_argument("--pilot", action="store_true", help="2 questions per type (12)")
    g.add_argument("--subset", nargs="?", const="subset_100.json", default=None, help="100-question stratified subset")
    g.add_argument("--all", action="store_true", help="all 500 questions")
    ap.add_argument("--name", default=None)
    ap.add_argument("--workers", type=int, default=6)
    args = ap.parse_args()

    data = load_data()
    if args.pilot:
        by_type = {}
        for q in sorted(data, key=lambda q: q["question_id"]):
            by_type.setdefault(q["question_type"], [])
            if len(by_type[q["question_type"]]) < 2:
                by_type[q["question_type"]].append(q)
        todo = [q for qs in by_type.values() for q in qs]
        default_name = "pilot"
    elif args.subset:
        subset_file = pathlib.Path(args.subset)
        if not subset_file.exists():
            for c in [ROOT / args.subset, BENCH / args.subset, ROOT / "subset_100.json"]:
                if c.exists():
                    subset_file = c
                    break
        subset_ids = set(json.loads(subset_file.read_text()))
        todo = [q for q in data if q["question_id"] in subset_ids]
        default_name = "baseline-100"
    else:
        todo = data
        default_name = "full"

    out_dir = RUNS / (args.name or default_name)
    out_dir.mkdir(parents=True, exist_ok=True)

    tracker = RunTracker(out_dir)
    tracker.update_metadata(
        name=out_dir.name,
        commit=get_server_commit(),
        engine_options=get_engine_options(),
        created_at=dt.datetime.utcnow().isoformat() + "Z",
        questions_total=len(todo),
    )
    set_run_tracker(tracker)

    hyp_file = out_dir / "hyp.jsonl"
    done = set()
    if hyp_file.exists():
        done = {json.loads(l)["question_id"] for l in hyp_file.read_text().splitlines() if l.strip()}
    todo = [q for q in todo if q["question_id"] not in done]
    print(f"{len(done)} done, {len(todo)} to run -> {hyp_file}", flush=True)

    lock = threading.Lock()
    stop = threading.Event()
    failed = []

    def work(q):
        if stop.is_set():
            return
        try:
            row = run_one(q)
        except BudgetExceeded as e:
            stop.set()
            failed.append(q["question_id"])
            print(f"STOP: {e}", flush=True)
            return
        except Exception as e:
            failed.append(q["question_id"])
            print(f"error {q['question_id']}: {str(e)[:200]}", flush=True)
            return
        with lock:
            with open(hyp_file, "a") as f:
                f.write(json.dumps(row) + "\n")
        print(f"ok {row['question_id']} {row['question_type']} mem={row['memories']} "
              f"ingest={row['ingest_s']}s embed={row['embed_wait_s']}s search={row['search_ms']}ms", flush=True)

    with cf.ThreadPoolExecutor(args.workers) as ex:
        list(ex.map(work, todo))

    # A run with missing answers isn't comparable; fail loudly (rerun resumes).
    if failed or stop.is_set():
        print(f"INCOMPLETE: {len(failed)} failed, stopped={stop.is_set()}; rerun the same command to resume", flush=True)
        sys.exit(1)


if __name__ == "__main__":
    main()
