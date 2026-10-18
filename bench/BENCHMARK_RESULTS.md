# Memro Benchmark Results

**Generated:** 2026-03-18 01:35:46 UTC

**Duration:** 65.6s


## Executive Summary

| Metric | Value | Unit |

|--------|-------|------|

| P95 Query Latency | 367.45 | ms |

| Throughput | 118.65 | queries/sec |

| Latency Growth (per 10x data) | 56.89 | % |



## Detailed Test Results


### 1. Recall Accuracy


**Average Query Latency:** 390.69ms

**Median Query Latency:** 253.00ms

**P95 Latency:** 788.03ms


**Sample Queries:**

| Query | Retrieved | Latency (ms) | Top Score |

|-------|-----------|--------------|----------|

| What programming languages do ... | 3 | 911.51 | 1.000 |

| Tell me about your travels... | 0 | 241.88 | 0.000 |

| What are your interests?... | 10 | 252.96 | 0.100 |

| Coffee preferences... | 0 | 253.00 | 0.000 |

| Team meetings... | 0 | 294.10 | 0.000 |



### 2. Query Latency Distribution


**Benchmark:** 50 queries


| Percentile | Latency (ms) |

|------------|-------------|

| Mean | 45.88 |

| Median (P50) | 10.28 |

| P95 | 367.45 |

| P99 | 402.27 |

| Min | 5.54 |

| Max | 415.11 |

| Stdev | 105.34 |



### 3. Temporal Reasoning


**Temporal Queries Tested:** 4

**Average Results per Query:** 0.0

**Average Latency:** 361.09ms



### 4. Knowledge Chain Resolution


**Chain Queries:** 3

**Average Chain Coverage:** 88.9%

**Average Latency:** 273.83ms



### 5. Throughput Performance


**Test Duration:** 30s

**Total Queries:** 3560

**Queries per Second:** 118.65

**Error Rate:** 0.00%

**Average Latency:** 8.42ms

**P95 Latency:** 12.15ms



### 6. Memory Efficiency & Scaling


**Avg Latency Growth:** 56.89% per 10x data

**Max Latency Growth:** 345.02%


**Latency by Memory Count:**

| Memories | Latency (ms) |

|----------|-------------|

| 10 | 211.41 |

| 50 | 13.42 |

| 100 | 9.59 |

| 500 | 10.04 |

| 1,000 | 44.69 |



## System Information


- **Benchmark Version:** 1.0

- **Timestamp:** 2026-03-18T01:35:45.993318

- **Total Duration:** 65.6s
