# Benchmark Journal

Internal engineering journal tracking Memro's memory retrieval quality, benchmark iterations, and failure analyses over time.

> **Team Preference**: Every benchmark run gets a blog-style write-up covering what was run, what changed or improved, and what's next, so the team can track progress over time.

## Entries (Newest First)

- **[2026-11-22: GitHub History — Does the Connector Save an Agent Tokens?](2026-11-22-github-history.md)**: 30 questions about memrohq/memro's PRs and issues, answered by Claude Haiku with `gh` only, Memro recall over the GitHub connector's memories, recall over extracted facts, or nothing. Pre-registered bar not met: Memro reads about the same tokens as gh (median 16.2k vs 16.6k, fewer on 15/30) at 26/30 vs 29/30 correct (p = 0.25), but answers 2.5× faster. About 5.2k tokens per model call are the Memro MCP tool list. Facts lose detail (23/30, p = 0.03 vs gh). Follow-up with the MCP server listing only 3 tools (`MEMRO_MCP_TOOLS`): fewer tokens than gh on 26/30 (p = 0.00006), median 10.3k vs 15.2k, same accuracy (28/30 each), same dollar cost.

- **[2026-11-22: LongMemEval-100, Memories Written by a Model](2026-11-22-longmemeval-model-writes.md)**: Claude Haiku extracts one fact per memory from each session, against storing exchanges word for word. A tie: facts 80/100 and raw 84/100 (gpt-4o-mini, McNemar p = 0.50; claude-sonnet judge 79 vs 83), with 3.1k vs 10.8k prompt tokens per answer. Facts lose on single-session-assistant questions (6/11 vs 11/11) and gain on temporal ones (23/26 vs 20/26).

- **[2026-11-17: Relay Benchmark — Does Memory Stop a Fresh Agent Repeating Mistakes?](2026-11-17-relay-benchmark.md)**: agent A learns a fact, a fresh agent B must respect it; arms none / notes file / Memro. Pilot v1 traps were too easy (every arm 6/6: A wrote the fact into code comments); v2 tells A the fact during unrelated work: none 5/15, notes 15/15, Memro 15/15 on R1–R3. Memory is the whole difference; in one checkout a notes file ties Memro. Stage 2: a teammate's clone ties (10/10 each); with 300 earlier facts a notes file wins 10/10 to Memro's 6/10 (A didn't save, or B's search missed). With hooks that save the user's stated rules and show them every prompt: Memro 10/10, notes 9/10, Memro tools alone 8/10.

- **[2026-11-15: Baselines — No Memory, Plain RAG and Memro](2026-11-15-baselines-no-memory-and-plain-rag.md)**: on LongMemEval-100, no memory 6%, plain semantic top-15 81%, Memro 81% (p = 1.0 vs RAG); memory is the whole score, Memro's extras show no gain on this subset.

- **[2026-11-15: Evolution Engine, Save-Time Graph Edges and False Contradictions](2026-11-15-evolution-engine-and-contradictions.md)**: Evolution benchmark 4/9 → 0/9 unresolved updates; five LongMemEval-100 arms (78–80%, none significant); repeat runs showing answer noise; the negation rule's false contradictions (15 → 1 of 153 pairs).
- **[2026-10-23: LongMemEval So Far — Progress, Accuracy Plateau, and What's Next](2026-10-23-longmemeval-so-far.md)**: Comprehensive look at runs from the initial pilots to `baseline-100`, the failure diagnosis on 22 misses, and the roadmap forward.

---

## How to Write the Next Post

When completing a benchmark run (pilot, subset, or full ablation), create a new markdown file named `YYYY-MM-DD-<topic>.md` in this directory and link it at the top of the index above.

Copy the run's results (not server logs) into `docs/benchmarks/results/<benchmark>/<run-name>/`, replace local paths, and add a row to [its index](../results/README.md), so every number in a post can be checked.

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
