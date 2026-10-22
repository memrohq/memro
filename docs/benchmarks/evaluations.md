# Measurable Verification Benchmark & Evaluations

This document reports empirical evaluation results for Memro across four core memory capabilities:
1. **Identity Boundaries (Multi-Tenant Isolation)**
2. **Contradictory Facts**
3. **Stale vs. Updated Facts**
4. **Distractor Resistance & Out-of-Domain Filtering**

These tests are fully reproducible using [`bench/eval_suite.py`](../../bench/eval_suite.py). In accordance with Memro's transparency principles, all pass/fail outcomes, latencies, and failure modes are documented below without exaggeration.

---

## Evaluation Summary (2026-10-22)

**Target**: `https://api.memro.co` (also verified locally against PostgreSQL 16 + pgvector)  
**Overall Pass Rate**: 1 / 4 (25.0%) passing under standard baseline recall parameters.

| Test Suite | Scenario | Result | Latency | Key Finding |
|---|---|---|---|---|
| **Identity Boundaries** | Agent A private memory isolation from Agent B | **PASS (100%)** | 3,887 ms | Strict cryptographic isolation; Agent B receives 0 results. |
| **Contradictory Facts** | Consecutive conflicting branch updates | **FAIL** | 1,773 ms | Standard `recall()` without temporal decay ranked the original fact above the update. |
| **Stale/Updated Facts** | Database cluster endpoint migration | **FAIL** | 2,210 ms | Standard semantic search without `recall_temporal()` or `resolve_conflict()` does not automatically deprecate the stale record. |
| **Distractor Rejection** | Threshold rejection on out-of-domain query | **FAIL** | 2,048 ms | Out-of-domain query returned low-scoring matches rather than an empty set at `min_score=0.4`. |

---

## Methodology & Detailed Results

### Suite 1: Identity Boundaries (Multi-Tenant Isolation)
- **Objective**: Verify that sovereign agent keypairs cannot access private memories belonging to other agents, even when querying identical keywords.
- **Methodology**:
  1. Generate sovereign agent $A$ via Ed25519 keypair and register via proof-of-work.
  2. Generate sovereign agent $B$ via independent Ed25519 keypair.
  3. Agent $A$ stores private secret: `"Confidential audit token for Agent A: sec_tok_a_..."`.
  4. Agent $A$ recalls the secret $\rightarrow$ Expected: retrieved.
  5. Agent $B$ executes identical search query $\rightarrow$ Expected: 0 results returned.
- **Outcome**: **PASS**
  - Agent $A$ retrieved 1 result (`score: 0.500`).
  - Agent $B$ retrieved 0 results.
  - Zero cross-tenant leakage observed across all test iterations.

---

### Suite 2: Contradictory Facts
- **Objective**: Measure whether sequential updates to contradictory facts (e.g. configuration changes) naturally prioritize the newer assertion in top-1 retrieval.
- **Methodology**:
  1. Store Fact 1: `"The project deployment branch is set to release-v1"`.
  2. Store Fact 2 (0.5s later): `"The project deployment branch is updated to release-v2"`.
  3. Query: `"project deployment branch"`.
- **Outcome**: **FAIL**
  - Expected: Top result contains `release-v2`.
  - Actual: Top result was Fact 1 (`release-v1`).
- **Root Cause Analysis**:
  - Baseline `recall()` uses Reciprocal Rank Fusion (RRF) between semantic embeddings and full-text keyword search (`tsvector`).
  - Without explicit temporal weighting (via `recall_temporal(..., temporal_weight=0.7)`), dense similarity scores between `"set to release-v1"` and `"updated to release-v2"` are nearly identical ($\approx 0.89$), causing the earlier inserted row to tie or rank higher due to insertion ordering.
- **Remediation**:
  - Use `agent.recall_temporal(..., temporal_weight=0.6)` for time-sensitive queries.
  - Explicitly call `agent.resolve_conflict(old_id, new_id)` to mark superseded memories.

---

### Suite 3: Stale vs. Updated Facts
- **Objective**: Measure whether migrated operational endpoints automatically supersede legacy configuration records.
- **Methodology**:
  1. Ingest: `"Staging database cluster endpoint is postgres-legacy.internal:5432"`.
  2. Ingest update: `"Staging database cluster migrated to postgres-aurora-v2.internal:5432"`.
  3. Query: `"staging database cluster endpoint"`.
- **Outcome**: **FAIL**
  - Expected: Top result points to `postgres-aurora-v2`.
  - Actual: Top result was `postgres-legacy.internal`.
- **Root Cause Analysis**:
  - In a standard hybrid retrieval pass, the query contains `"staging database cluster endpoint"`, which has an exact lexical match in the legacy string (`"is postgres-legacy..."`), giving it a higher BM25/tsvector sparse rank than the migrated string (`"migrated to postgres-aurora..."`).
- **Remediation**:
  - Delete or supersede outdated memories via `agent.delete_memory(old_id)` when migrations are recorded, or query with temporal freshness filters.

---

### Suite 4: Distractor Rejection & Out-of-Domain Filtering
- **Objective**: Measure retrieval precision when querying against unrelated distractor memories and evaluate `min_score` cutoff effectiveness.
- **Methodology**:
  1. Insert 4 unrelated distractor memories (office pantry, visitor badges, team retreat, projector settings).
  2. Insert target memory: `"Production release freeze window starts December 15th at 18:00 UTC."`
  3. Test target query: `"production release freeze window date and time"`.
  4. Test unrelated negative query: `"quantum cryptography entanglement simulation algorithms"` with `min_score=0.4`.
- **Outcome**: **FAIL**
  - Expected: Target ranked #1; negative query returns 0 results.
  - Actual: Unrelated query returned 5 results with low dense-similarity scores below the semantic relevance threshold.
- **Root Cause Analysis**:
  - RRF fusion score normalization maps ranking positions rather than absolute cosine similarity scores. When a candidate list is non-empty, RRF yields non-zero reciprocal rank values even for distractor items.
- **Remediation**:
  - Set `mode="semantic"` with strict cosine distance thresholds when strict out-of-domain rejection is required, or inspect raw embedding distances before passing retrieved context to LLM prompts.

---

## How to Reproduce

Run the evaluation script against any live or local Memro instance:

```bash
# Against local self-hosted instance:
docker compose up -d
python bench/eval_suite.py http://localhost:8081

# Against Memro Cloud:
export MEMRO_API_KEY="memro_sk_..."
python bench/eval_suite.py https://api.memro.co
```

Results are saved to `bench/measurable_eval_results.json`.

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
