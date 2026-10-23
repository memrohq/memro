# 0021. Temporal Ordering and Contradiction Resolution in Search
Date: 2026-10-23 · Status: Accepted

## Context
LongMemEval benchmark analysis (500 questions baseline and 100-question CI subset) revealed that while retrieval recall@15 is high (99%), the downstream answering step struggled with two persistent failure modes:

1. **Knowledge Updates & Stale Facts**: When an agent's memory contains both an initial state and an updated state (e.g., apartment lease duration changing from 6 months to 2 years in question `2133c1b5`, coin collection totals in `69fee5aa`, or tennis racket preferences in `f685340e`), both exchanges are retrieved. Because search results historically lacked chronological indicators and superseding metadata, the LLM answering step frequently selected the older, superseded fact.
2. **Calendar Arithmetic Hallucinations**: When answering temporal span questions ("how many days between X and Y?"), answer models hallucinated day counts across month boundaries despite having the correct raw dates.

Previously, `server/internal/engine/knowledge.go` implemented `DetectContradiction` and `server/internal/engine/ledger.go` implemented `BuildTemporalLedger`, but these capabilities were isolated to separate cognitive/MEE endpoints (`/mee/reason`, `/mee/resolve`) and were not integrated into the primary search pipeline (`GET /search` and `engine.Search`). Furthermore, `domain.SearchResult` omitted `event_date`, `document_date`, and `session_id`, leaving callers blind to the temporal state of retrieved memories.

## Decision

### 1. Extend `domain.SearchResult` and `domain.SearchQuery`
`domain.SearchResult` now includes temporal provenance and conflict resolution metadata:
```go
type SearchResult struct {
    MemoryID     string  `json:"memory_id"`
    Content      string  `json:"content"`
    Score        float32 `json:"score"`
    MemoryType   string  `json:"memory_type"`
    CreatedAt    string  `json:"created_at"`
    DocumentDate *string `json:"document_date,omitempty"`
    EventDate    *string `json:"event_date,omitempty"`
    SessionID    *string `json:"session_id,omitempty"`
    AssetURL     *string `json:"asset_url"`
    IsSuperseded bool    `json:"is_superseded,omitempty"`
    SupersededBy *string `json:"superseded_by,omitempty"`
}
```

`loadResults` in `server/internal/engine/engine.go` queries `document_date`, `event_date`, and `session_id` directly from the `memories` table alongside content and metadata.

### 2. Pairwise Contradiction Detection & Superseding in Search
In `Search`, when multiple candidate memories are returned:
1. **Explicit Relations**: The engine queries `memory_relations` for `updates` edges between returned memories. Any memory targeted by an update is flagged `IsSuperseded = true` and `SupersededBy = sourceID`.
2. **Dynamic Contradiction Detection**: Pairs of results are evaluated with `DetectContradiction`. When contradiction likelihood $\ge 0.7$, the engine compares `event_date`, `document_date`, and `created_at`. The older memory is flagged `IsSuperseded = true` and `SupersededBy = newerID`.
3. **Optional Score Demotion**: When `q.ResolveContradictions` is true (or query parameter `resolve_conflicts=true`), superseded memories have their ranking scores halved, ensuring the latest fact surfaces first.

### 3. Chronological Sequencing (`order`)
`SearchQuery` and `GET /search` accept an `order` parameter:
- `"score"` (default): Rank by hybrid fusion / similarity score.
- `"chronological"`: Ascending chronological order (oldest to newest) by event/document date, falling back to score on ties.
- `"chronological_desc"`: Descending chronological order (newest to oldest).

### 4. Deterministic Temporal Ledger (`include_ledger`)
When `include_ledger=true` is requested, the engine invokes `RenderLedgerFromResults(anchor, results)`:
- Computes exact relative day offsets (`X days ago`, `same day`, `in X days`) against `anchor_date` (defaulting to current time).
- Computes exact interval days between successive chronological events (`+N days`).
- Highlights any superseded memories (`[SUPERSEDED]`).
- Appends monthly sequence breakdowns to prevent LLM calendar calculation hallucinations.
- Returns the rendered markdown table in `SearchResponse.Ledger`.

### 5. SDK and API Support
- **HTTP API**: `GET /search` accepts `order`, `resolve_conflicts` / `resolve_contradictions`, `include_ledger`, `anchor_date`, and `temporal_weight`.
- **SDKs**: Go, Python, and TypeScript/JavaScript SDKs expose these parameters and fields across `search()`, `SearchResult`, and `SearchResponse`.

## Consequences
- **Positive**: Directly solves knowledge update conflicts on LongMemEval without requiring expensive external re-ranking models.
- **Positive**: Provides verifiable, deterministic arithmetic for temporal reasoning tasks.
- **Positive**: Fully backward-compatible; default search behavior remains unchanged when new parameters are omitted.
- **Positive**: Cache keys in `ResultCache` incorporate all new parameters, ensuring isolation across distinct ordering and conflict modes.
