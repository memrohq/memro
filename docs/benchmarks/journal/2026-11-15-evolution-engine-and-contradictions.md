# 2026-11-15: Evolution engine, save-time graph edges and false contradictions

- **Date**: 2026-11-15
- **Server commits**: `48fa10f` (main before PR #60), `894d141` (PR #60), `a4a9257` (PR #63, negation fix).
  Each run's `meta.json` records the harness checkout (`894d141`), not the server binary; the table below gives the binary.
- **Run directories**: `bench/longmemeval/runs/lme-{A..E}-*`, `flips-{A,B}-{1..3}`, `temporal-C-{1..3}` (local, not committed)
- **Dataset / subset**: LongMemEval_S, `bench/longmemeval/subset_100.json`; plus the evolution benchmark (`TestEvolutionBench`, 18 scenarios)
- **Engine options**: `{"top_k": 15, "mode": "hybrid", "answer_model": "gpt-4o-mini", "session_expansion": 3, "per_session": 0, "format": "standard", "per_item_chars": 4000, "resolve_conflicts": true, "order": "score", "embedding_model": "text-embedding-3-small"}`
- **Cost**: $0.87 for the five 100-question arms (~$0.17 each), $0.04 for the repeat runs

## 1. Goal & hypothesis

Make memory *evolve*: a newer fact should replace the older one, restatements
should collapse, and unrelated facts must be left alone. PR #60 sharpened the
contradiction rules and let the save-time linker (`GRAPH_WRITE_EDGES`) find
candidates by shared words. Hypotheses: (1) the evolution benchmark goes to
zero unresolved updates without false updates; (2) LongMemEval doesn't regress,
and save-time edges may help.

## 2. Evolution benchmark (`TestEvolutionBench`, CI gate)

9 updates, 2 duplicates, 7 independent pairs (5 of them traps). Lower is better.

| Server | Linking | Unresolved updates | Duplicates left | False updates |
|---|---|---|---|---|
| main before #60 (`48fa10f`) | off / on | 4/9 | 2/2 | 0/7 |
| PR #60 (`894d141`) | off / on | **0/9** | **0/2** | 0/7 |
| PR #63 (`a4a9257`) | off / on | 0/9 | 0/2 | 0/7 |
| PR #62, #64 (audit, agent edits) | off / on | 0/9 | 0/2 | 0/7 |

The gate fails CI if any of the three is above zero.

## 3. LongMemEval-100, five arms on fresh databases

| Arm | Server | Graph edges | Overall | Knowledge-update | Multi-session | Temporal | "No info" with evidence | Search p50 / p95 | Cost |
|---|---|---|---|---|---|---|---|---|---|
| A | main before #60 | off | 79% | 81.2% | 63.0% | 73.1% | 4 | 1037 / 1940 ms | $0.175 |
| B | PR #60 | off | 78% | 87.5% | 55.6% | 73.1% | 6 | 1004 / 1748 ms | $0.175 |
| C | PR #60 | on | 80% | 87.5% | 66.7% | 69.2% | 2 | 849 / 2366 ms | $0.174 |
| D | PR #63 | off | **80%** | 81.2% | **70.4%** | 69.2% | **2** | 1298 / 2791 ms | $0.172 |
| E | PR #63 | on | 78% | 75.0% | 63.0% | 73.1% | 4 | 1122 / 4478 ms | $0.171 |

Single-session categories were 100% (assistant, user) and 83.3% (preference)
in every arm. Evidence recall@15 was 93.3% in every arm: retrieval didn't
change, only what the answer model was shown and in which order.

Paired tests (McNemar, questions that flipped):

| Comparison | Fixed | Regressed | p |
|---|---|---|---|
| A → B (PR #60) | 1 | 2 | 1.00 |
| B → C (graph on) | 3 | 1 | 0.63 |
| B → D (negation fix) | 4 | 2 | 0.69 |
| C → E (negation fix, graph on) | 2 | 4 | 0.69 |

**None is significant.** At n = 100 one question is one point, and the
answer model is not fully deterministic (next section).

D and E were interrupted by OpenAI embedding rate limits (1M tokens/min) at
99 and 91 of 100 questions and resumed; each question uses its own agent, so
resuming doesn't change the others. Arms A–C also hit rate limits (retried by
the indexer backfill).

## 4. Noise check: re-running the flipped questions

- **A vs B flips** (`184da446`, `2b8f3739`, `gpt4_372c3eed_abs`), 3 runs each:
  A scored 3/3, 2/3, 1/3; B scored 1/3, 1/3, 2/3. The same server gives
  different answers between runs.
- **Temporal regression in C** (`gpt4_483dd43c`), 3 runs on C: correct 3/3.
  The "temporal drop" in C was noise, not the graph.

## 5. Root cause found: false contradictions from the negation rule

The negation rule called two statements contradicting when one had a negative
word and they shared any word over three letters. On one question's chat
sessions it marked **15 of 153** message pairs as contradicting, so search
demoted real evidence, and with save-time linking arm C wrote **~1,870**
bogus `updates` edges. PR #63 requires the same claim (content-word Jaccard ≥
0.5 once negative/sentiment words are removed) and matches "never use X" vs
"use X" directly: **1 of 153** pairs.

That's the likely reason "no information while evidence present" fell from 6
to 2 (B → D) and multi-session rose 55.6% → 70.4%, though not significantly.
With graph edges on (E) the gain didn't hold (78%), so save-time linking stays
off by default.

## 6. Takeaways & next steps

- PR #60 is justified by the evolution benchmark, not by LongMemEval.
- PR #63 removes a real source of wrong demotions; LongMemEval is neutral to
  slightly positive.
- `GRAPH_WRITE_EDGES` stays off: no reliable LongMemEval gain (C 80%, E 78%)
  and a worse p95.
- Rules keep needing traps; ADR 0032 moves contradictions to structured facts
  (single- vs multi-valued attributes) with an LLM judge only for unclear pairs.
- To tell real gains from noise, run the 500-question set or repeat arms
  (≥3 runs) before claiming a LongMemEval change.
- CI's LongMemEval job needs the `OPENAI_API_KEY` repository secret; these
  runs were local.
