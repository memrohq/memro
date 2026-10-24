# Benchmark Journal

Internal engineering journal tracking Memro's memory retrieval quality, benchmark iterations, and failure analyses over time.

> **Team Preference**: Every benchmark run gets a blog-style write-up covering what was run, what changed or improved, and what's next, so the team can track progress over time.

## Entries (Newest First)

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
