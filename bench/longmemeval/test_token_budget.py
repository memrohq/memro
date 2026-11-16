"""Offline checks for the TOKEN_BUDGET and BASELINE=full arms (no server, no OpenAI).

    uv run --with pytest --with requests --with pynacl --with pydantic pytest -q test_token_budget.py
"""
import os
import sys

os.environ["TOKEN_BUDGET"] = "100"
sys.path.insert(0, os.path.dirname(__file__))
import run  # noqa: E402


def test_budget_skips_superseded_duplicates_and_oversized():
    contents = {"a": "x" * 40, "b": "X " * 20, "c": "y" * 4000, "d": "Z" * 20, "e": "z" * 20}
    meta = {k: {"content": v, "date": f"2023/05/0{i}", "session": k} for i, (k, v) in enumerate(contents.items(), 1)}
    hits = [{"memory_id": "c"}, {"memory_id": "a"}, {"memory_id": "b", "is_superseded": True},
            {"memory_id": "d"}, {"memory_id": "e"}]
    # c is too big, b superseded, e a duplicate of d once lowercased
    assert run.context_ids(hits, meta, {}) == ["a", "d"]


def test_full_baseline_puts_whole_history_in_prompt(monkeypatch):
    q = {"question_id": "q", "question_type": "t", "question": "?", "answer": "a",
         "question_date": "2023/06/01 (Thu) 10:00", "answer_session_ids": [],
         "haystack_session_ids": ["s1"], "haystack_dates": ["2023/05/01 (Mon) 10:00"],
         "haystack_sessions": [[{"role": "user", "content": "hi", "has_answer": True},
                                {"role": "assistant", "content": "yo"}]]}
    prompts = []

    def ask(prompt, usage=None):
        prompts.append(prompt)
        usage.update(prompt_tokens=123)
        return "ok"

    monkeypatch.setattr(run, "BASELINE", "full")
    monkeypatch.setattr(run, "ask", ask)
    row = run.run_one(q)
    assert "User: hi\nAssistant: yo" in prompts[0]
    assert (row["memories"], row["evidence_in_context"], row["prompt_tokens"]) == (1, 1, 123)
