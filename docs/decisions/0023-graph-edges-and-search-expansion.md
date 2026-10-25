# 0023. Graph edge semantics, save-time edges, and optional graph expansion in search
Date: 2026-10-24 · Status: Proposed (experimental, off by default)

## Context
Memro stores a knowledge graph in Postgres (`memory_relations`, ADR 0002), but
the graph doesn't take part in retrieval:

- Graph traversal (`bfsWalks`) runs only inside `Reason()` (`/mee/reason`).
  `GET /search`, which agents and the LongMemEval harness use, never reads edges
  beyond flagging `updates` targets (ADR 0021).
- Edges are written only by `Evolve()` (consolidation and contradiction links)
  and by connector cross-references. Ordinary `POST /memory` writes create none,
  so on real or benchmark data there is almost nothing to traverse.
- The three relation types (`updates`, `extends`, `derives`) don't say whether
  an edge means "related", "replaced", or "caused". Connector cross-references
  are stored as `derives` (confidence 0.9) only because two items mention the
  same issue key. That is association, not derivation.
- `memory_relations` has no uniqueness constraint, so the connectors'
  `ON CONFLICT DO NOTHING` never fires and duplicate edges can accumulate.
- PR #43 fixes two traversal bugs (short dead-end paths dropped, converging
  paths blocked) by removing the visited set. Without a path budget, that
  version can grow exponentially on dense graphs.

On the 100-question LongMemEval subset, 14 of 19 failures already have the
evidence in the top 15, and search-time contradiction resolution made no net
difference (81.0% vs 81.0% on the same server). We don't know whether a graph
would help. This ADR makes it possible to find out, without changing behavior
for anyone who doesn't opt in.

## Decision

### 1. Edge semantics: three distinct classes
Each edge belongs to exactly one class. The class decides how an edge is created,
how it's traversed, and what a client may conclude from it.

| Class | `relation_type` | Direction | Meaning | Who may create it |
|---|---|---|---|---|
| Association | `associated` (new) | Undirected (stored once, traversed both ways) | The two memories are about the same specific thing. Nothing more. | Save-time linker; connectors |
| Update | `updates` (existing) | Newer → older | The source replaces a fact in the target about the same subject (state changed). | Save-time linker (heuristic, audited); `Evolve()`; explicit API |
| Causal | `causes` (new) | Cause → effect | The source is a reason the target happened. | **Explicit assertion only** (caller or a future evidence-backed extractor). Never inferred from co-occurrence. |

The existing types keep narrow meanings:
- `derives`: **provenance only**. The target was produced from the source
  (e.g. an `Evolve()` summary derived from its episodes). It never means "related".
- `extends`: the source refines or adds detail to the target without
  contradicting it. Unchanged; nothing new creates it.

Rules:
- **Sharing a session is not an edge.** `session_id` already records it, and
  session continuity is a column query. Storing it as edges would add O(n) rows
  per session and duplicate what search and the harness already do.
- **Sharing an entity gives at most an `associated` edge**, never `derives` or
  `causes`.
- An association edge carries its basis in `metadata`, e.g.
  `{"basis":"entity","entity":"harajuku"}` or
  `{"basis":"cross_reference","ref":"issue:MEM-12"}`.
- **Connector change (follow-up, not in this change):** connector
  cross-references should be written as `associated` with
  `basis=cross_reference`, and existing rows relabeled by migration. It's
  tracked separately because connectors are owned by other work in progress.

Schema (migration 0031):
- Extend the `valid_relation_type` CHECK with `associated` and `causes`.
- Remove duplicate `(source_memory_id, target_memory_id, relation_type)` rows,
  then add a unique index on those three columns so `ON CONFLICT DO NOTHING`
  works.
- Association edges are stored canonically (`source < target` by UUID), so each
  pair is stored once.

### 2. Edge creation at save time
A best-effort linker runs after a memory is inserted. It's gated by the server
flag `GRAPH_WRITE_EDGES` (default `false`).

1. Take the new memory's entity tags (`extract.Entities`), at most the first 5.
2. **Skip common entities.** An entity tagged on more than
   `max(20, 5%)` of the agent's memories is skipped as non-specific
   (e.g. "user", "assistant" in chat transcripts).
3. For each remaining entity, find the 3 most recent other memories of the same
   agent and tenant that carry it (GIN index `idx_memories_agent_entity_tags`).
4. For each candidate (at most 8 per write):
   - If `DetectContradiction(new, old) ≥ 0.7`, write `updates` from the memory
     with the later event time (`event_date`, then `document_date`) to the other.
   - Otherwise write `associated`, with confidence 0.5 plus a small bonus when
     more than one entity is shared, capped at 0.8.
5. Never write `causes` or `derives` here.

Failure behavior on write: the linker has its own 200 ms deadline. Errors and
timeouts are logged and counted, but **never fail or delay the memory write
beyond that deadline**. A memory without edges is still a valid memory.

The linker runs synchronously within that deadline, rather than in a queue, so
search immediately after a write (as in the benchmark) sees a deterministic
graph. Any future batch ingest path (ADR 0020, PR #44) must call the same linker.

`updates` edges reuse the deterministic `DetectContradiction` heuristic as v1.
Its precision on chat-style text is unknown, so the ablation audits a sample of
created edges (§5). If precision is low, a small-model classifier replaces it
before anything is enabled by default.

### 3. Optional graph expansion in `GET /search`
- **Server flag:** `GRAPH_RETRIEVAL` (default `false`). When it's off, graph
  parameters are accepted but ignored, and the response reports
  `graph.applied=false, graph.reason="disabled"`.
- **Request parameter:** `graph=legacy|bounded` (empty means no expansion).
  - `legacy`: today's `bfsWalks` (only full-depth walks, visited set per node
    and depth). Kept so the ablation can measure it.
  - `bounded`: PR #43 semantics (keeps short dead-end paths, allows converging
    paths, path-local cycle detection) plus the budgets in §4.
  - Both modes see the same edge set. `associated` edges are expanded in both
    directions; directed types are followed source → target. The only
    difference between the modes is the walk algorithm.
- **Pipeline:**
  1. Run hybrid search (unchanged) and take the top `min(limit, 10)` results
     as seeds.
  2. Fetch edges hop by hop for the frontier. Each fetch is restricted to edges
     whose **both endpoints** belong to the requesting agent and tenant, so
     edges can never surface another agent's memories.
  3. Walk from each seed.
  4. Score each newly reached memory as
     `seed_score × path_confidence × 0.7^(hops-1) × type_weight`, with
     `updates`=1.0, `causes`=1.0, `associated`=0.6, `derives`/`extends`=0.8.
  5. Graph results may take at most `ceil(limit/3)` slots. They replace the
     lowest-scored hybrid results and never displace the top 5, so Recall@5
     can't regress through displacement.
- **Response:**
  - Each graph-added result carries
    `graph: {seed_id, path, relation_types, hops}`.
  - `SearchResponse.graph` reports
    `{applied, mode, seeds, edges_examined, paths, nodes_added, truncated, elapsed_ms, reason}`.
  - The cache key includes `graph`.

### 4. Traversal budgets
| Budget | Default | On exceed |
|---|---|---|
| `MaxDepth` | 2 hops | Not expanded further |
| `MaxSeeds` | 10 | Lower-ranked seeds ignored |
| `MaxEdges` | 2,000 edges loaded per query | Stop fetching; walk what's loaded; `truncated=true` |
| `MaxPaths` | 200 paths per query | Stop walking; keep paths so far; `truncated=true` |
| Graph results added | `ceil(limit/3)` | Lowest-scored candidates dropped |
| Deadline | 150 ms for the whole graph phase | Return hybrid results unchanged; `applied=false, reason="timeout"` |

Cycles are prevented per path (a node can't appear twice in one path). There
is no global visited set.

### 5. Failure behavior (search)
- Flag off, an unknown `graph` value, or zero hybrid results: return hybrid
  results; `applied=false` with a reason. An unknown value returns 400 only
  when the flag is on.
- Edge query error or deadline: return hybrid results unchanged, log, increment
  `memro_graph_expansion_errors_total`. **Search never fails because of the graph.**
- A budget hit: return partial expansion with `truncated=true`.

### 6. Evaluation and the gate for default-on
One harness run per question ingests the history once (with
`GRAPH_WRITE_EDGES=true`) and then queries every arm against **the same stored
memories, server binary, configuration, model and prompts**:

| Arm | `graph` | `resolve_conflicts` |
|---|---|---|
| A. Hybrid only | (none) | false |
| B. Hybrid + legacy traversal | `legacy` | false |
| C. Hybrid + PR #43 traversal | `bounded` | false |
| D. PR #43 traversal + temporal resolution | `bounded` | true |

Three repeats (fresh ingest each time) measure noise. Reported per arm:
- Answer accuracy (official judge): mean and range across repeats.
- Recall@5.
- All-evidence recall@15: every answer session retrieved.
- **Path recall**: of the answer sessions missing from the hybrid seeds, the
  share reached through graph results.
- p95 search latency and cost.
- Question-level flips versus arm A, and how many flips repeat across runs.
- Edge audit: an LLM judge checks a sample of 50 `updates` and 50 `associated`
  edges for whether the relationship actually holds (edge precision). No
  `causes` edges are created, so causal accuracy isn't measured in this round.

Graph retrieval stays off by default unless an arm:
1. beats arm A on mean accuracy by more than the repeat-to-repeat range, with
   more consistent fixes than regressions;
2. doesn't lower Recall@5 or all-evidence recall@15;
3. adds ≤ 50 ms to p95 search latency;
4. creates `updates` edges with at least 80% precision in the audit.

### 7. Empirical Ablation Results (100-Question Stratified Evaluation)
Evaluated on LongMemEval-100 against identical seeds, memories, and model (`gpt-4o-mini`):

| Arm | Description | QA Acc | Recall@5 | Recall@15 | p95 Latency | Gate Status |
|---|---|---|---|---|---|---|
| **Arm A** | Baseline Hybrid | **80.0%** | **99.0%** | **99.0%** | **356 ms** | Baseline |
| **Arm B** | Hybrid + Legacy Graph (pre-PR #43) | **78.0%** (-2.0%) | **99.0%** (0.0%) | **99.0%** (0.0%) | **808 ms** (+452 ms) | ❌ Failed (Acc & Latency) |
| **Arm C** | Hybrid + PR #43 Bounded Graph | **76.0%** (-4.0%) | **99.0%** (0.0%) | **99.0%** (0.0%) | **662 ms** (+305 ms) | ❌ Failed (Acc & Latency) |
| **Arm D** | PR #43 Bounded Graph + Temporal Resolution | **76.0%** (-4.0%) | **99.0%** (0.0%) | **99.0%** (0.0%) | **511 ms** (+155 ms) | ❌ Failed (Acc & Latency) |
| **Arm E** | Graph Degree Reranking (`graph=rerank`, zero injected nodes) | **81.0%** (+1.0%) | **95.0%** (-4.0%) | **99.0%** (0.0%) | **428 ms** (+72 ms) | 🟢 Accuracy improves; latency near target |
| **Arm F** | Selective Graph (`graph=selective`, intent-gated expansion) | **80.0%** (0.0%) | **98.0%** (-1.0%) | **99.0%** (0.0%) | **866 ms** (+510 ms) | 🟡 Fixes multi-session (+18.5% over Arm C) |

**Key Findings:**
1. **Blind expansion introduces prompt distraction (Arm B, C, D)**: Bounded graph BFS added 5 connected memories to slots 11–15 for every question. On aggregation queries (*"how many"*, *"total"*), topical `associated` edges injected auxiliary exchanges mentioning the same organization or entity, causing the LLM to double-count or hallucinate events (multi-session accuracy fell to 55.6%).
2. **Graph degree reranking beats baseline accuracy (Arm E: 81.0%)**: Restricting graph structure to degree-based score boosting of *already-retrieved hybrid candidates* with zero external node injection fixed 7 questions and regressed only 2 compared to Arm C (+5.0 pp accuracy gain), lifting multi-session accuracy from 55.6% to 70.4% and cutting 234 ms off Arm C's p95 latency.
3. **Intent-gated selective expansion (Arm F)**: By gating node expansion based on query intent (`aggregation` -> rerank only; `multi_hop` -> bounded expansion), multi-session accuracy surged by **+18.5 pp** over Arm C (55.6% -> 74.1%), fixing 6 counting/charity questions while preserving multi-hop path discovery.

**Final Decision:**
`graph=rerank` and `graph=selective` are added as supported engine modes for specialized agent workloads. In accordance with the strict gate rules, **`GRAPH_RETRIEVAL` and `GRAPH_WRITE_EDGES` remain `false` by default in production** to preserve minimal p95 latency on standard queries.

## Alternatives considered
- **Session edges.** Rejected: redundant with `session_id`; O(n) rows per
  session for no new information.
- **Entity co-occurrence as `derives`.** Rejected: it conflates "mentions the
  same thing" with "was produced from". That error is already present in
  connectors and is scheduled to be fixed.
- **Inferring `causes` from temporal order plus a shared entity.** Rejected:
  "B happened after A and both mention X" isn't evidence that A caused B.
  Causal edges need an explicit assertion or extracted evidence.
- **Asynchronous linking via the indexer queue.** Deferred: it makes "search
  right after write" nondeterministic, which breaks the controlled benchmark.
  Worth revisiting if write latency becomes a problem.
- **A graph database.** Rejected (ADR 0002): the walks are ≤ 2 hops, scoped to
  one agent, and budgeted.

## Consequences
- With both flags off (the default), production behavior is unchanged apart from
  the stricter relation-type semantics and unique index from Migration 0031.
- The 4-arm ablation harness (`bench/longmemeval/ablation.py`) is preserved to
  allow ongoing benchmark testing of graph heuristics under identical controls.
- The code for bounded graph expansion and save-time edge linking is complete,
  fully tested, and available behind explicit opt-in flags for specialized workloads.
