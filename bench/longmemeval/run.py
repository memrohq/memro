"""LongMemEval_S on Memro: ingest each question's chat history into a fresh
agent, retrieve with Memro's hybrid search, and answer with an LLM.

    python run.py --pilot            # 2 questions per type (12)
    python run.py --all              # all 500 (resumes; finished ids are skipped)

Writes runs/<name>/hyp.jsonl. Grade with judge.py, summarise with report.py.
"""
import argparse
import concurrent.futures as cf
import datetime as dt
import json
import os
import subprocess
import sys
import threading
import time

from common import RUNS, BENCH, BudgetExceeded, chat

sys.path.insert(0, os.environ.get("MEMRO_SDK", str(BENCH.parent / "memro" / "sdk" / "python" / "src")))
from memro import Agent, MemroClient  # noqa: E402
from memro.crypto import generate_keypair  # noqa: E402

DATA = BENCH / "data" / "longmemeval_s_cleaned.json"
API = os.environ.get("MEMRO_API", "http://127.0.0.1:18090")
DB_CONTAINER = os.environ.get("MEMRO_DB_CONTAINER", "memro-bench-db")
TOP_K = int(os.environ.get("TOP_K", "15"))
PER_ITEM_CHARS = int(os.environ.get("PER_ITEM_CHARS", "4000"))   # each retrieved memory, as shown to the answering model
MAX_MEMORY_CHARS = 9500  # Memro accepts up to 10,000 characters per memory
ANSWER_MODEL = os.environ.get("ANSWER_MODEL", "gpt-4o-mini")


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
    out = subprocess.run(["docker", "exec", DB_CONTAINER, "psql", "-U", "memro", "-d", "memro", "-tAc", sql],
                         capture_output=True, text=True, check=True)
    return out.stdout.strip()


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


def answer(q, hits, meta, session_memories):
    # Bounded Session Continuity Expansion:
    hit_ids = [h["memory_id"] for h in hits]

    # Identify top 3 unique sessions from hits
    top_sessions = []
    for h in hits:
        sid = meta[h["memory_id"]]["session"]
        if sid not in top_sessions:
            top_sessions.append(sid)
        if len(top_sessions) >= 3:
            break

    # Add sibling rounds for the top 3 sessions
    expanded_ids = list(hit_ids)
    for sid in top_sessions:
        for mid in session_memories.get(sid, []):
            if mid not in expanded_ids:
                expanded_ids.append(mid)

    # Sort chronologically by date
    ctx = []
    for mid in sorted(expanded_ids, key=lambda mid: meta[mid]["date"]):
        m = meta[mid]
        header = format_session_header(m["date"], q["question_date"])
        ctx.append(f"{header}\n{m['content'][:PER_ITEM_CHARS]}")

    prompt = ANSWER_PROMPT.format(now=q["question_date"], context="\n\n---\n\n".join(ctx) or "(nothing found)",
                                  question=q["question"])
    est = len(prompt) / 3.0 * 0.15 / 1e6 + 400 * 0.6 / 1e6
    return chat(ANSWER_MODEL, [{"role": "user", "content": prompt}], 400, est)


def run_one(q):
    t0 = time.time()
    client = MemroClient(API, api_key=None)
    agent = new_agent(client)
    meta = {}
    session_memories = {}
    try:
        n = 0
        for sid, date, session in zip(q["haystack_session_ids"], q["haystack_dates"], q["haystack_sessions"]):
            when = parse_date(date)
            session_memories[sid] = []
            for text in rounds(session):
                m = client.create_memory(agent.agent_id, agent._private_key, text, "episodic",
                                         event_date=when.strftime("%Y-%m-%dT%H:%M:%SZ"))
                meta[m["id"]] = {"session": sid, "date": date, "content": text}
                session_memories[sid].append(m["id"])
                n += 1
        ingest_s = time.time() - t0
        deadline = time.time() + 1800
        while (left := pending_embeddings(agent.agent_id)) and time.time() < deadline:
            time.sleep(2)
        if left:
            raise RuntimeError(f"{left} memories still not embedded")
        embed_s = time.time() - t0 - ingest_s
        t1 = time.time()
        hits = client.search(agent.agent_id, q["question"][:500], limit=TOP_K, private_key=agent._private_key, mode="hybrid")
        search_ms = (time.time() - t1) * 1000
        hyp = "(no LLM: dry run)" if os.environ.get("BENCH_NO_LLM") else answer(q, hits, meta, session_memories)
        return {
            "question_id": q["question_id"], "question_type": q["question_type"], "question": q["question"],
            "answer": q["answer"], "hypothesis": hyp,
            "retrieved_sessions": [meta[h["memory_id"]]["session"] for h in hits],
            "answer_session_ids": q["answer_session_ids"], "memories": n,
            "ingest_s": round(ingest_s, 1), "embed_wait_s": round(embed_s, 1), "search_ms": round(search_ms, 1),
        }
    finally:
        drop_agent(agent)


def main():
    ap = argparse.ArgumentParser()
    g = ap.add_mutually_exclusive_group(required=True)
    g.add_argument("--pilot", action="store_true", help="2 questions per type")
    g.add_argument("--all", action="store_true")
    ap.add_argument("--name", default=None)
    ap.add_argument("--workers", type=int, default=6)
    args = ap.parse_args()

    data = json.load(open(DATA))
    if args.pilot:
        by_type = {}
        for q in sorted(data, key=lambda q: q["question_id"]):
            by_type.setdefault(q["question_type"], [])
            if len(by_type[q["question_type"]]) < 2:
                by_type[q["question_type"]].append(q)
        todo = [q for qs in by_type.values() for q in qs]
    else:
        todo = data
    out_dir = RUNS / (args.name or ("pilot" if args.pilot else "full"))
    out_dir.mkdir(parents=True, exist_ok=True)
    hyp_file = out_dir / "hyp.jsonl"
    done = set()
    if hyp_file.exists():
        done = {json.loads(l)["question_id"] for l in hyp_file.read_text().splitlines() if l.strip()}
    todo = [q for q in todo if q["question_id"] not in done]
    print(f"{len(done)} done, {len(todo)} to run -> {hyp_file}", flush=True)

    lock = threading.Lock()
    stop = threading.Event()

    def work(q):
        if stop.is_set():
            return
        try:
            row = run_one(q)
        except BudgetExceeded as e:
            stop.set()
            print(f"STOP: {e}", flush=True)
            return
        except Exception as e:
            print(f"error {q['question_id']}: {str(e)[:200]}", flush=True)
            return
        with lock:
            with open(hyp_file, "a") as f:
                f.write(json.dumps(row) + "\n")
        print(f"ok {row['question_id']} {row['question_type']} mem={row['memories']} "
              f"ingest={row['ingest_s']}s embed={row['embed_wait_s']}s search={row['search_ms']}ms", flush=True)

    with cf.ThreadPoolExecutor(args.workers) as ex:
        list(ex.map(work, todo))


if __name__ == "__main__":
    main()
