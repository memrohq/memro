# 🚀 Performance Improvement Quick Start

**Goal:** Increase recall from 0.335 → 0.8+ through three phases  
**Timeline:** 10-15 days  
**Status:** Baseline measured ✅ → Ready for P1 implementation

---

## 📋 One-Time Setup

```bash
# Activate venv (if not already done)
source .venv/bin/activate

# Navigate to benchmark directory
cd bench

# Verify Docker containers running
docker ps | grep memro  # Should show: backend, postgres, qdrant, neo4j, frontend-developer
```

---

## 🎯 Phase 1: Hybrid BM25+Vector Search (1-2 days)

### Step 1: Backend Implementation

Edit `server/src/search/hybrid.rs`

**Add PostgreSQL FTS Index:**
```sql
-- Run in postgres:
CREATE INDEX idx_memories_content_fts ON memories 
USING GIN(to_tsvector('english', content));
```

**Create Hybrid Search Function:**
```rust
// In backend/src/search/hybrid.rs
pub async fn hybrid_search(
    pool: &PgPool,
    agent_id: Uuid,
    query: &str,
    embedding: Vec<f32>,
    vector_weight: f32,
    keyword_weight: f32,
    limit: i32,
) -> Result<Vec<SearchResult>> {
    // 1. Get vector results (60% weight)
    // 2. Get keyword results (40% weight)  
    // 3. Combine and deduplicate
    // 4. Return top K
}
```

### Step 2: API Endpoint

Add to `backend/src/api/search.rs`:
```rust
#[post("/search/hybrid")]
async fn search_hybrid(
    Extension(pool): Extension<PgPool>,
    Json(request): Json<HybridSearchRequest>,
) -> Result<Json<Vec<SearchResult>>> {
    // Call hybrid_search function
    // Return results
}
```

### Step 3: Python SDK Update

Edit `sdk/python/src/memro/`:
```python
def recall(self, query: str, use_hybrid: bool = False, **kwargs) -> List[Memory]:
    if use_hybrid:
        # Call /search/hybrid endpoint
        return self._client.post(
            "/search/hybrid",
            query=query,
            vector_weights=kwargs.get("vector_weight", 0.6),
            keyword_weights=kwargs.get("keyword_weight", 0.4)
        )
    else:
        # Original vector search
        return self._client.post("/search", query=query)
```

### Step 4: Test P1 Implementation

```bash
# Run baseline benchmark
cp benchmark_report.json p0_baseline.json

# Run P1 tests (non-destructive, using existing 326K memories)
python benchmark_existing_data.py \
  --agent_id ab7a24f744520625fb44fda13e3c5e7704cd80e15806cf4b1eada850fc9d79ca \
  --private_key REDACTED_TEST_KEY \
  --enable-hybrid-search \
  --output p1_results.json

# Compare results
python compare_benchmarks.py p0_baseline.json p1_results.json --output p1_comparison.json
```

### Step 5: Verify Success Criteria

✅ **P1 Success = All 3 Met:**
```
1. Recall >  0.40  (baseline 0.335 → target 0.40-0.50)
2. Precision > 0.50 (keyword filtering reduces false positives)
3. Latency P95 < 50ms (baseline 23.6ms + ~5-10ms overhead = OK)
```

---

## 📊 After Each Phase

### Copy Results to Tracker
```bash
# Document in IMPROVEMENT_TRACKER.md:
# 1. Update Progress Tracking Table
# 2. Record actual recall/precision/latency
# 3. Update status from ⏳ to ✅ or note any gaps
```

### Generate Report
```bash
echo "## P1 Results - $(date)" >> IMPLEMENTATION_LOG.md
python compare_benchmarks.py baseline.json results_p1.json >> IMPLEMENTATION_LOG.md
```

---

## 🔍 Phase 2: Entity Extraction + Filtering (3-5 days)

*After P1 validation, proceed with:*

1. Add `memories_entities` junction table (tracks: person, place, org, concept)
2. Build entity extractor service (LLM or transformer-based)
3. Pre-filter memories by extracted entities before vector search
4. Target: 0.6-0.7 recall

---

## ⏰ Phase 3: Temporal Ranking (5-7 days)

*After P2 validation, proceed with:*

1. Add temporal metadata (`event_date`, `temporal_tags`)
2. Build temporal decay scorer (recent = higher score)
3. Support temporal queries ("my latest project", "yesterday's notes")
4. Target: 0.8+ recall

---

## 📈 Expected Results Timeline

```
Day 1-2:   P1 Implementation + Testing    → Recall 0.40-0.50
Day 5-7:   P2 Implementation + Testing    → Recall 0.60-0.70  
Day 12-14: P3 Implementation + Testing    → Recall 0.80+
Day 15:    Full validation + Documentation
```

---

## 🛠️ Debugging Commands

```bash
# View latest benchmark report
cat benchmark_report.json | jq '.metrics'

# Compare two specific results
python compare_benchmarks.py benchmark_report.json p1_results.json

# Run single test
python benchmark_existing_data.py --test-only hybrid_search

# Check database memory count
psql -U postgres -h localhost -c "SELECT COUNT(*) FROM memories WHERE agent_id='ab7a24f744520625fb44fda13e3c5e7704cd80e15806cf4b1eada850fc9d79ca';"

# View implementation roadmap
cat IMPROVEMENT_ROADMAP.md

# View progress tracker
cat IMPROVEMENT_TRACKER.md
```

---

## 📝 Key Reminders

✅ **Do:** 
- Test against existing 326K memories (no re-ingestion)
- Keep latency P95 < 100ms
- Run comparison after each phase
- Document results in IMPROVEMENT_TRACKER.md
- Commit working code regularly

❌ **Don't:**
- Re-ingest memories unnecessarily
- Break agent isolation (scoped searches required)
- Exceed 100ms P95 latency
- Skip testing before proceeding to next phase

---

## 🚀 Ready to Start P1?

```bash
# Confirm environment
source .venv/bin/activate
cd bench

# Verify baseline exists
ls -la benchmark_report.json
ls -la compare_benchmarks.py
ls -la benchmark_existing_data.py

# You're ready!
echo "✅ Ready for P1 implementation"
```

**Next command to run (after P1 code is ready):**
```bash
python benchmark_existing_data.py \
  --agent_id ab7a24f744520625fb44fda13e3c5e7704cd80e15806cf4b1eada850fc9d79ca \
  --private_key REDACTED_TEST_KEY \
  --enable-hybrid-search \
  --output p1_results.json
```

---

## 📞 Support Files

- **Baseline:** [benchmark_report.json](benchmark_report.json)
- **Comparison Tool:** [compare_benchmarks.py](compare_benchmarks.py)
- **P1 Testable:** [benchmark_existing_data.py](benchmark_existing_data.py)
- **Progress Tracker:** [IMPROVEMENT_TRACKER.md](IMPROVEMENT_TRACKER.md)
- **Roadmap:** [IMPROVEMENT_ROADMAP.md](IMPROVEMENT_ROADMAP.md)

---

**Now proceed with P1 implementation! Let me know when code is ready to test.** 🚀
