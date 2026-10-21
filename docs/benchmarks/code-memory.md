# Code-memory benchmark (design)

Status: design · 2026-10-21 · For [ADR 0015](../decisions/0015-verified-code-memory.md)

**Question it answers:** when a coding agent recalls what earlier sessions
learned about a codebase, does Memro give it the *current* answer more often
than plain retrieval, and does it say when something may be out of date?

It reuses the LongMemEval harness in `memro-benchmark` (fresh agent per
scenario, cached embeddings, cost ledger, paired statistics). It adds a
dataset built from a real repository's history and three systems to compare.

## Dataset

**Source:** one open-source repository with a permissive license, at least two
years of history, and decisions explained in pull requests or commit messages
(design changes, reversals, renamed or removed modules). Shortlist, chosen by
checking those three criteria: PocketBase (Go, MIT), Hono (TypeScript, MIT),
Litestream (Go, Apache-2.0). Memro's own repository is a second, smaller
set: it has ADRs, but we wrote it, so it is reported separately.

**Scenarios.** Each scenario is a sequence of sessions at increasing commits:

1. **Learn (commit A):** memories a coding agent would save, written from the
   pull requests merged up to A. Each is one decision, convention or fix,
   anchored to the files the pull request touched: 300–500 memories in all.
2. **Change (commit B, months later):** the code moved on. Ground truth comes
   from history:
   - a decision reversed or replaced (a later PR changes the same files and
     says so);
   - files deleted or renamed;
   - files changed in unrelated ways (the decision still holds);
   - files untouched.
3. **Ask (at B):** 80–100 questions, each with a reference answer and the
   memory or memories that support it.

**Question types and target counts:**

| Type | Count | Correct behaviour |
|---|---|---|
| Still valid, files untouched | 20 | Recall it |
| Still valid, files changed for other reasons | 15 | Recall it (a `changed` label is acceptable) |
| Superseded by a later decision | 20 | Give the new decision; mention the old one only as history |
| About deleted or renamed code | 10 | Say it no longer applies |
| Needs two memories from different sessions | 10 | Combine them |
| Not in memory | 10 | Say it does not know |
| Distractor: similar wording, other module | 10 | Do not answer from the wrong module |

**Building it.**
- A script walks the repository history and proposes memories and questions
  with an LLM, from PR text and diffs.
- A person reviews every question and reference answer before use; nothing
  unreviewed is scored. Expect about 4 hours of review for 100 questions.
- The dataset, the script and the review notes are published with the
  results.

## Systems compared
Same answering model, prompt, context budget (top 10) and grader for all:

| System | What it gets |
|---|---|
| **A. No memory** | The question and the repository's README |
| **B. Plain retrieval** | Memro search over the same memories, no anchors or validity (today's behaviour) |
| **C. Memro verified** | Anchors and validity. The harness answers `check_memories` with the true blob IDs at commit B, as an agent running `git` would. Stale results are labeled and demoted. |
| **C′. Labels only** | As C without demotion, to separate the effect of labels from ranking |

The comparisons that matter are B vs C and C vs C′.

## Metrics
- **Correct answers** (official-style LLM grader; a sample of 50 cross-checked
  with a second model and by hand).
- **Stale answers:** answered with a superseded or deleted-code fact as if
  current. This is the headline metric.
- **Correct abstentions** on not-in-memory and deleted-code questions.
- **Flagging:** when a used memory was stale, did the answer say so?
- Context tokens per question, latency added by `check_memories`.

Statistics: paired McNemar exact tests per type, Holm correction, bootstrap
95% intervals, as in the LongMemEval search evaluation.

**Success criterion (set before running):** C has fewer stale answers than B
(Holm-adjusted p < 0.05) and no significant loss of correct answers on the
still-valid types.

## Cost and effort
- Embeddings for about 500 memories × 2 repositories: well under $0.10.
- Answering and grading with `gpt-4o-mini`, 4 systems × 100 questions: about
  $0.50 per repository.
- Building: dataset script about 1 day, review about 4 hours, harness changes
  about 1 day (once anchors and `check_memories` exist).

## Limitations to publish with it
- Memories are written from PRs, not captured from live agent sessions, so
  real agents may save less (or worse) than this.
- File-level anchors flag every change to a file; a decision about one
  function is labeled `changed` when another function changes.
- One or two repositories; results may not carry over to other codebases.
- The grader is an LLM; disagreements are reported.

## After the benchmark
Weeks 3–4 of the plan: 10–20 developers who use Claude Code, Cursor or Codex
daily use verified memory on their own repositories for two weeks. We measure
repeated explanations and stale-advice incidents they report, against a week
without it.
