# Benchmark Journal

Internal engineering journal tracking Memro's memory retrieval quality, benchmark iterations, and failure analyses over time.

> **Team Preference**: Every benchmark run gets a blog-style write-up covering what was run, what changed or improved, and what's next, so the team can track progress over time.

## Entries (Newest First)

- **[2026-11-17: Relay Benchmark — Does Memory Stop a Fresh Agent Repeating Mistakes?](2026-11-17-relay-benchmark.md)**: agent A learns a fact, a fresh agent B must respect it; arms none / notes file / Memro. Pilot v1 traps were too easy (every arm 6/6: A wrote the fact into code comments); v2 tells A the fact during unrelated work: none 5/15, notes 15/15, Memro 15/15 on R1–R3. Memory is the whole difference; in one checkout a notes file ties Memro. Stage 2: a teammate's clone ties (10/10 each); with 300 earlier facts a notes file wins 10/10 to Memro's 6/10 (A didn't save, or B's search missed).

- **[2026-11-15: Baselines — No Memory, Plain RAG and Memro](2026-11-15-baselines-no-memory-and-plain-rag.md)**: on LongMemEval-100, no memory 6%, plain semantic top-15 81%, Memro 81% (p = 1.0 vs RAG); memory is the whole score, Memro's extras show no gain on this subset.

- **[2026-11-15: Evolution Engine, Save-Time Graph Edges and False Contradictions](2026-11-15-evolution-engine-and-contradictions.md)**: Evolution benchmark 4/9 → 0/9 unresolved updates; five LongMemEval-100 arms (78–80%, none significant); repeat runs showing answer noise; the negation rule's false contradictions (15 → 1 of 153 pairs).
- **[2026-10-23: LongMemEval So Far — Progress, Accuracy Plateau, and What's Next](2026-10-23-longmemeval-so-far.md)**: Comprehensive look at runs from the initial pilots to `baseline-100`, the failure diagnosis on 22 misses, and the roadmap forward.

---

## How to Write the Next Post

When completing a benchmark run (pilot, subset, or full ablation), create a new markdown file named `YYYY-MM-DD-<topic>.md` in this directory and link it at the top of the index above.

### Post Template

```markdown
# YYYY-MM-DD: [Title of Run or Experiment]

- **Date**: YYYY-MM-DD
- **Server Commit**: `<git-sha>`
- **Run Directory**: `bench/longmemeval/runs/<run-name>`
- **Dataset / Subset**: LongMemEval_S (e.g. 100-question subset / 500 full)
- **Engine Options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", ...}`
- **Cost**: $X.XXXX

## 1. Goal & Hypothesis

What single change was being tested, and what was the expected outcome?

## 2. Configuration & Changes

What changed in the engine, ranking, prompt, or retrieval parameters? (Reference PRs or commits).

## 3. Results & Comparison

Include comparison table against previous baseline using \`compare.py\`:

| Category | n | Acc (Prev) | Acc (New) | Delta | R@5 | R@15 |
|---|---|---|---|---|---|---|
| knowledge-update | ... | ... | ... | ... | ... | ... |
| multi-session | ... | ... | ... | ... | ... | ... |
| temporal-reasoning | ... | ... | ... | ... | ... | ... |
| overall | ... | ... | ... | ... | ... | ... |

- **Search Latency (p50 / p95)**: X ms / Y ms
- **Duplicate Session Slots**: X.X / 15
- **Abstentions / False Abstentions**: ...

## 4. Analysis & Failure Drilldown

- Which questions flipped from wrong to right? Why?
- Did any questions regress from right to wrong?
- Break down remaining failures by exact evidence availability (in-context vs missed).

## 5. What's Next

What is the immediate next single-variable experiment?
```
