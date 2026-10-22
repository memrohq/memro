# LongMemEval So Far: Progress, Accuracy Plateau, and What's Next

- **Date**: 2026-10-23
- **Author**: Platform & Memory Team
- **Scope**: LongMemEval_S benchmark evaluations from pilot to `baseline-100`

---

## 1. What We Measure and How

Memro is evaluated on **LongMemEval_S**, a benchmark suite of 500 challenge questions designed to test how well conversational memory engines retain, retrieve, and synthesize facts across long multi-session chat histories.

The benchmark spans six distinct challenge categories:
- **Knowledge Update** (78 questions): Facts that change over time (e.g. new apartment, new routine); the engine must return the current fact, not the stale one.
- **Multi-Session Synthesis** (133 questions): Information spread across multiple past conversations (e.g. total money earned across markets, hours played across games).
- **Single-Session User** (70 questions): User-stated personal facts within a single session.
- **Single-Session Assistant** (56 questions): Assistant-stated facts or suggestions.
- **Single-Session Preference** (30 questions): User tastes, hotel preferences, or dietary requirements.
- **Temporal Reasoning** (133 questions): Relative time questions (e.g. "what did I buy 10 days ago?", date intervals between events).

### The Evaluation Pipeline

1. **Ingestion**: For each question, all prior chat sessions (an average of 249 conversation turns) are ingested into an isolated Memro agent instance.
2. **Retrieval**: When the question is asked, Memro runs hybrid retrieval combining dense 1536-dimensional embeddings (`text-embedding-3-small`) with PostgreSQL full-text keyword search via Reciprocal Rank Fusion (RRF, $k=60$), retrieving the top 15 memories.
3. **Session Continuity Expansion**: The top 3 retrieved sessions are expanded to include contiguous conversation exchanges so the answering model sees dialogue context rather than isolated fragments.
4. **Answer Generation**: OpenAI `gpt-4o-mini` generates an answer using standard benchmark prompts.
5. **Independent Grading**: `gpt-4o-mini` acts as an independent judge, evaluating the answer against the official ground truth. In cross-validation samples, `gpt-4o-mini` agreed with `gpt-4o` on 49 out of 50 answers (98% agreement).
6. **Cost Tracking**: All API spend is tracked deterministically in `runs/ledger.json`. At standard list prices ($0.02 / 1M embedding tokens, $0.15 / $0.60 per 1M `gpt-4o-mini` prompt/completion tokens), a full 500-question pass costs ~$1.25 fresh or ~$0.43 with cached embeddings. The 100-question subset costs ~$0.17 per run.

---

## 2. Timeline of Runs

| Date | Run Name | n | QA Accuracy | Top K | Key Findings & Configuration |
|---|---|---|---|---|---|
| Oct 21 | `pilot-1` / `pilot-2` | 12 | *Invalid* | 10 | Early pilot runs were invalidated because fake-vector dry runs filled the Memro query cache. Once identified, caching was isolated and the server was restarted. |
| Oct 21 | `full` | 500 | **77.6%** (388/500) | 10 | First valid 500-question run. Recall@5 reached 98.6%. Search latency p50 was 564 ms. 73 of 112 failed questions had all evidence sessions in the top 10. |
| Oct 21 | `full-fixed` | 500 | **75.0%** (375/500) | 15 | Evaluated raw turn excerpts without conversation continuity context. Accuracy dipped 2.6 points, showing that models struggle when isolated turns lack context. |
| Oct 21 | `full-continuity` | 500 | **77.6%** (388/500) | 15 | Added bounded conversational turn continuity. Single-session preference jumped +10 points (60.0% -> 70.0%), temporal dipped −3 points, and search p50 improved from 564 ms to 401 ms. |
| Oct 21 | `answer_experiment` | 224 | **78.6%** (+0.8%) | 15 | Tested prompt engineering enhancements at the answering stage. The +0.8 point gain was within statistical noise, confirming prompt tweaks alone cannot resolve engine-level gaps. |
| Oct 23 | `baseline-100` | 100 | **78.0%** (78/100) | 15 | Stratified 100-question subset across all 6 categories (16 KU, 27 MS, 11 Asst, 6 Pref, 14 User, 26 TR, exactly 6 abstentions). Recall@5 was 99.0%, Recall@15 was 99.0%, run spend was $0.1714. Committed as the CI gate baseline. |
| Oct 23 | `failure-analysis` | 100 | — | 15 | Deep diagnostic pass replaying all 22 failures to isolate root causes. |

---

## 3. What Improved — and the Accuracy Plateau

Over the past week, infrastructure and measurement improved substantially:
- **Reproducible Measurement**: We moved from ad-hoc 500-question scripts to a standardized 100-question stratified subset that completes in ~2.5 minutes for $0.17.
- **Automated CI Gate**: The regression gate (`bench/longmemeval/compare.py --ci-gate`) now runs automatically on PRs labeled `bench`, failing if overall accuracy regresses by >2% or any category regresses by >5%.
- **Token & Cost Ledger**: Every token and dollar is tracked in `ledger.json` across embedding, answering, and judging stages.
- **Search Latency**: Retrieval p50 dropped from 564 ms in the initial run to 229 ms in the latest 100-question baseline.

### The Honest Truth: Accuracy Has Been Flat

Despite major improvements in testing speed, caching, and latency, **end-to-end QA accuracy has stayed flat at 77% to 78%**. 

Measuring retrieval by coarse session IDs produces high numbers (Recall@15 is 99%), but that metric gives a false sense of security. It tells us that an exchange *from the correct conversation* was retrieved, but not whether the *specific answer-bearing exchange* was present in the prompt.

---

## 4. Deep Dive: Baseline-100 Failure Analysis

On the 100-question baseline (`baseline-100`), Memro answered 78 correctly and failed 22. Using `bench/longmemeval/diagnose.py`, we analyzed the exact prompt text, retrieved memory IDs, and ground truth for all 22 failures.

The 22 failures fall into six distinct buckets:

```
Total Failures: 22
├── 1. Answer model limitations (gpt-4o solves on identical prompt): 8
├── 2. Retrieval gaps at exchange level (partial/missing evidence): 9
│   ├── Partial evidence in context: 7
│   └── Zero evidence in context: 2
├── 3. Knowledge-update errors (stale fact selected over new): 2
├── 4. False preference abstention ("I don't know"): 1
├── 5. Hallucinated answer on unanswerable question: 1
└── 6. Benchmark dataset quirk: 1
```

### Detailed Breakdown

1. **Answer Model Limitations (8 questions)**:
   - For 8 questions, all required evidence exchanges were in the top 15 prompt context, but `gpt-4o-mini` failed the multi-step arithmetic or multi-session deduction.
   - When we fed the *identical prompt* to `gpt-4o`, it answered all 8 correctly.
   - *(Note: `gpt-4o` produced 9 apparent fixes, but 1 was a judge error where `gpt-4o-mini` judge misgraded a valid answer).*
2. **Exchange-Level Retrieval Gaps (9 questions)**:
   - **7 questions had partial evidence**: For multi-session questions (e.g. counting total gaming hours across 5 sessions or counting baking instances across two weeks), Memro retrieved 3 or 4 of the sessions, but missed 1 critical session turn. Because the answer requires a sum, partial retrieval leads to an incorrect total.
   - **2 questions had zero evidence**: The specific exchange holding the fact was not in the top 15 slots (e.g. question `gpt4_8279ba03` asking for a kitchen appliance bought 10 days ago).
3. **Knowledge-Update Errors (2 questions)**:
   - Both the old fact and the new fact were retrieved into the prompt (e.g. previous tennis frequency vs current tennis frequency in `f685340e`, apartment living duration in `2133c1b5`).
   - The answering model conflated the timeline and picked the stale information because Memro stored both as active memories without explicit supersession relations.
4. **False Preference Abstention (1 question)**:
   - On question `0edc2aef` ("Can you suggest a hotel for my upcoming trip to Miami?"), the user's preference history was present in context, but the model responded with a generic "I don't know" rather than extracting the user's stylistic preferences.
5. **Failed Abstention on Unanswerable Question (1 question)**:
   - On question `gpt4_372c3eed_abs`, the prompt did not contain the years spent on the Master's degree. The model hallucinated "10 years" instead of properly abstaining.
6. **Dataset Quirk (1 question)**:
   - Minor expectation discrepancy in the benchmark ground truth.

---

## 5. What's Next: The Measured Road Ahead

We are not going to make ad-hoc prompt changes. Every proposed change will be implemented as its own PR, evaluated on the 100-question subset, checked by the CI regression gate, and finally verified against all 500 questions.

Our planned order of work:

1. **Retrieve Complete Evidence for Multi-Part Questions**:
   - Address the 9 exchange-level retrieval misses.
   - When a query asks for totals or multi-session items, expand retrieval to pull all turns linked to that entity or topic rather than relying on independent top-K slots.
2. **Engine-Level Memory Lifecycle & Temporal Reasoning**:
   - Add write-time supersession (`status = 'superseded'`, `superseded_by UUID`, and `conflicts_with` / `supersedes` relations).
   - Resolve the knowledge-update failures by filtering out superseded memories at query time.
   - Use `event_date` exclusively for temporal indexing rather than ingestion timestamps (`created_at`).
3. **Answer-Stage Context Synthesis in Memro**:
   - Rather than relying on each client application to format and synthesize raw memory excerpts, introduce a dedicated Memro context synthesis endpoint.
   - Provide structured timelines, entity profiles, and deduplicated facts directly in the API.
4. **Cloudflare Experimental Track (PR #38)**:
   - As a separate, isolated research track, evaluate Cloudflare Workers AI embeddings (`bge-small`, `bge-m3`) and Vectorize index performance against our PostgreSQL + pgvector baseline.
   - Keep PostgreSQL + pgvector as the primary production engine under test.
