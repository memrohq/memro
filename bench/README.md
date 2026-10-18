# Memro Comprehensive Benchmark Suite

Complete benchmark suite for evaluating Memro performance and comparing against Supermemory methodology.

## Overview

This benchmark suite measures:

1. **Recall Accuracy** - Semantic search quality
2. **Query Latency** - Response time distribution (mean, median, P95, P99)
3. **Temporal Reasoning** - Accuracy of time-based memory grounding
4. **Knowledge Chain Resolution** - Depth and accuracy of relationship traversal
5. **Throughput** - Queries per second under load
6. **Memory Efficiency** - Scaling behavior with data size

## Quick Start

### Prerequisites

```bash
# Install dependencies
pip install numpy

# Ensure Memro backend is running
# The benchmarks expect API at http://localhost:8081
```

### Run Full Benchmark Suite

```bash
# Run all benchmarks (includes 30s throughput test)
python run_benchmarks.py

# Quick mode (skips throughput test, ~5 min)
python run_benchmarks.py --quick

# Custom API URL
python run_benchmarks.py --base_url http://your-api:8081
```

## Benchmark Details

### 1. Recall Accuracy

**What it measures:** Quality of semantic search results

**Test:** 5 diverse queries searching across 15 test memories

**Output:**
- Average latency per query
- Median latency
- P95 latency
- Top result score

**Supermemory equivalent:** Semantic search quality from LongMemEval

### 2. Query Latency

**What it measures:** Response time distribution under typical load

**Test:** 50 sequential queries with diverse patterns

**Output:**
- Mean latency
- Median latency (P50)
- P95 latency (95th percentile - typical user experience)
- P99 latency (99th percentile - worst case)
- Standard deviation
- Min/max latency

**Target:** P95 < 100ms (typical), P99 < 200ms (worst case)

### 3. Temporal Reasoning

**What it measures:** Ability to ground memories in time and query temporally

**Test:** 4 temporal queries across 5 time-distributed memories

**Output:**
- Number of temporally-relevant results
- Average latency for temporal queries

**Supermemory equivalent:** Temporal grounding accuracy

### 4. Knowledge Chain Resolution

**What it measures:** Depth of knowledge chains and relationship traversal

**Test:** 3 multi-hop relationship queries

**Output:**
- Chain depth coverage (% of expected depth)
- Number of related memories retrieved
- Average latency

**Example:** Query "Tell me about Alice and what she works on" should traverse:
- Alice → Person
- Alice → Works at → Company
- Alice → Published → Research

### 5. Throughput

**What it measures:** System throughput under sustained load

**Test:** 30-second load test with 5 diverse queries

**Output:**
- Queries per second (QPS)
- Total queries completed
- Error rate
- Average and P95 latency under load

**Target:** >50 QPS for single instance

### 6. Memory Efficiency

**What it measures:** How latency scales with data size

**Test:** Query latency at 10, 50, 100, 500, 1000 memories

**Output:**
- Latency at each scale
- Growth rate between steps
- Scaling model (linear vs sublinear)

**Target:** Sublinear growth (<30% per 10x data increase)

## Output Files

### benchmark_results.json
Raw benchmark data in JSON format. Contains:
- Individual test results
- Detailed query-by-query data
- Timestamps and metadata

### BENCHMARK_RESULTS.md
Human-readable report with:
- Executive summary
- Detailed results for each test
- Tables and metrics
- Performance analysis

### benchmark_report.json
Structured report with:
- Extracted key metrics
- Performance tier classification
- Optimization recommendations
- Raw results reference

### OPTIMIZATION_COMPARISON.md
Comparison table showing:
- Before/after optimization gains
- Expected improvement from P0 and P1 optimizations
- Target performance levels

## Performance Tiers

### Latency Classification
- **EXCELLENT:** P95 < 100ms
- **GOOD:** P95 100-500ms
- **ACCEPTABLE:** P95 500-1000ms
- **NEEDS_IMPROVEMENT:** P95 > 1000ms

### Throughput Classification
- **EXCELLENT:** >100 QPS
- **GOOD:** 50-100 QPS
- **ACCEPTABLE:** 10-50 QPS
- **NEEDS_IMPROVEMENT:** <10 QPS

### Scaling Classification
- **EXCELLENT:** <10% latency growth per 10x data
- **GOOD:** <30% growth (sublinear)
- **NEEDS_IMPROVEMENT:** >30% growth (linear or worse)

## Supermemory Comparison

This benchmark suite uses similar methodology to Supermemory's published benchmarks:

| Aspect | Supermemory | Memro |
|--------|-------------|-------|
| Recall Quality | LongMemEval dataset | 15-memory synthetic dataset + diverse queries |
| Latency Percentiles | P50, P95, P99 | P50, P95, P99 |
| Temporal Reasoning | Event timeline accuracy | Temporal query accuracy |
| Knowledge Graphs | Relationship depth | Knowledge chain depth |
| Throughput | QPS under load | QPS (30s sustained) |
| Scaling | Various data sizes | 10-1000 memories |

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Benchmark

on: [push, pull_request]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    services:
      memro:
        image: ghcr.io/memrohq/memro:latest
        ports:
          - 8081:8081
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-python@v2
        with:
          python-version: '3.11'
      - run: pip install -r bench/requirements.txt
      - run: |
          cd bench
          python run_benchmarks.py --quick
          python scripts/compare_results.py
```

## Advanced Usage

### Custom Benchmarks

Create a new benchmark by extending `MemroBenchmarkSuite`:

```python
from comprehensive_benchmark import MemroBenchmarkSuite

class MyBenchmark(MemroBenchmarkSuite):
    def benchmark_custom_feature(self):
        """Test custom feature."""
        print("Testing custom feature...")
        # Your test code
        return {"test": "custom", "result": "value"}

suite = MyBenchmark()
results = suite.benchmark_custom_feature()
```

### Comparing Versions

```bash
# Run benchmark before optimization
python run_benchmarks.py > before.json

# Apply optimization
# ...

# Run benchmark after optimization
python run_benchmarks.py > after.json

# Compare
python scripts/compare_results.py before.json after.json
```

### Continuous Benchmarking

```bash
# Save results with version tag
python run_benchmarks.py --output results_v${VERSION}.json

# Track performance over releases
git log --oneline | head -20 | while read commit msg; do
  git checkout $commit
  python run_benchmarks.py --output results_${commit:0:7}.json
done
```

## Interpreting Results

### Example Benchmark Results

```json
{
  "query_latency": {
    "avg_latency_ms": 45.3,
    "p95_latency_ms": 78.2,
    "p99_latency_ms": 125.0,
    "queries": 50
  },
  "throughput": {
    "queries_per_second": 67.5,
    "error_rate": 0.02,
    "total_queries": 2000
  }
}
```

**Interpretation:**
- ✅ Average query takes 45ms
- ✅ 95% of queries complete within 78ms (good)
- ⚠️  1% of queries take >125ms (acceptable but monitor)
- ✅ System handles 67.5 queries/second
- ✅ <2% error rate is good

### Optimization Impact

After implementing P0 optimizations, expect:
- Query latency: -40% (P95 from 130ms → 78ms)
- Throughput: +150% (from 25 QPS → 62 QPS)
- Memory scaling: Improved to sublinear

After P1 optimizations:
- Query latency: Additional -50% (P95 → 40ms)
- Throughput: +100% more (from 62 QPS → 120+ QPS)
- Cache hits: 70-80% for typical workloads

## Troubleshooting

### Connection Refused

```
Error: Connection refused at http://localhost:8081
```

**Solution:** Ensure Memro backend is running:
```bash
cd memro
cargo run --release
```

### Slow Benchmarks

If benchmarks take >10 minutes:
- Use `--quick` flag to skip throughput test
- Reduce test dataset size in `comprehensive_benchmark.py`
- Ensure system isn't under other load

### Missing Dependencies

```bash
pip install numpy  # Required for percentile calculations
```

## Performance Targets

### Target Metrics for Production

| Metric | Target | Priority |
|--------|--------|----------|
| Query Latency P95 | <100ms | Critical |
| Query Latency P99 | <200ms | High |
| Throughput | >50 QPS | High |
| Error Rate | <1% | Critical |
| Memory Scaling | Sublinear | Medium |
| Temporal Accuracy | >90% | Medium |

## Publishing Results

### Generated Report Includes

1. **Executive Summary** - Key metrics at a glance
2. **Detailed Results** - Full breakdown of each benchmark
3. **Performance Classification** - Tier assignment based on metrics
4. **Recommendations** - Specific optimization suggestions
5. **Comparison Tables** - Before/after optimization impact

### Publication Format

Reports are saved as:
- `BENCHMARK_RESULTS.md` - Shareable markdown
- `benchmark_report.json` - Machine-readable format
- `OPTIMIZATION_COMPARISON.md` - Impact analysis

## Further Reading

- [Supermemory Benchmark Methodology](https://supermemory.com/docs/benchmarks)
- [LongMemEval Dataset](https://github.com/supermemoryai/longmemeval)
- [Performance Optimization Guide](../doc/OPTIMIZATION_COMPLETE.md)

---

**Last Updated:** 2026-03-17
**Benchmark Version:** 1.0
**Compatible With:** Memro v0.1+
