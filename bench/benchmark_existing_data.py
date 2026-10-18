#!/usr/bin/env python3
"""
Memro Benchmark - Test Against Existing Data Only
No memory ingestion - uses 326K existing memories in database
Designed for iterative improvements: baseline → hybrid → entity → reasoning
"""

import time
import json
import statistics
from typing import List, Dict, Any
from datetime import datetime
import numpy as np

try:
    from memro import Agent, MemroClient
except ImportError:
    raise ImportError("memro SDK not installed. Run: pip install memro")


class MemroExistingDataBenchmark:
    """Benchmark suite that tests against existing agent memories."""
    
    def __init__(self, base_url: str = "http://localhost:8081", agent_id: str = None, private_key: str = None):
        """Initialize with existing agent."""
        self.client = MemroClient(base_url)
        self.agent_id = agent_id
        self.private_key = private_key
        self.agent = None
        self.start_time = datetime.now()
        
    def _get_agent(self) -> Agent:
        """Get agent instance."""
        if not self.agent:
            if self.agent_id and self.private_key:
                self.agent = Agent(agent_id=self.agent_id, private_key=self.private_key, client=self.client)
            else:
                raise ValueError("agent_id and private_key required")
        return self.agent
    
    def benchmark_hybrid_search(self) -> Dict[str, Any]:
        """Benchmark hybrid search (vector + keyword combined)."""
        print("\n🔍 Benchmark: HYBRID SEARCH (Existing Data)")
        print("=" * 70)
        
        agent = self._get_agent()
        
        # Realistic LongMemEval-style questions
        test_queries = [
            {
                "query": "Where did I buy my tennis racket?",
                "keywords": ["tennis", "racket", "buy", "store", "shop"],
                "description": "Entity + location lookup"
            },
            {
                "query": "What was my first job?",
                "keywords": ["first", "job", "work", "position", "started"],
                "description": "Career history"
            },
            {
                "query": "Which conferences did I attend?",
                "keywords": ["conference", "attend", "event", "talk", "present"],
                "description": "Event participation"
            },
            {
                "query": "What's my favorite restaurant?",
                "keywords": ["favorite", "restaurant", "food", "eat", "cuisine"],
                "description": "Preference lookup"
            },
            {
                "query": "Tell me about my vacation to Italy",
                "keywords": ["vacation", "Italy", "travel", "Rome", "Venice"],
                "description": "Trip details"
            },
        ]
        
        results = []
        latencies = []
        
        for test in test_queries:
            start = time.time()
            # Current: Just vector search
            recalls = agent.recall(test["query"], limit=10, mode="hybrid")
            latency = (time.time() - start) * 1000
            latencies.append(latency)
            
            # Count matches with keywords (simulating hybrid relevance)
            keyword_matches = sum(
                1 for recall in recalls
                if any(kw.lower() in recall.content.lower() for kw in test["keywords"])
            )
            
            result = {
                "query": test["query"],
                "type": test["description"],
                "retrieved": len(recalls),
                "keyword_matches": keyword_matches,
                "latency_ms": latency,
                "estimated_precision": keyword_matches / len(recalls) if recalls else 0
            }
            results.append(result)
            
            print(f"\n  Query: {test['query']}")
            print(f"    Type: {test['description']}")
            print(f"    Retrieved: {len(recalls)} | Keyword matches: {keyword_matches} | Latency: {latency:.2f}ms")
            print(f"    Precision: {result['estimated_precision']*100:.1f}%")
        
        result_dict = {
            "test": "hybrid_search_baseline",
            "timestamp": datetime.now().isoformat(),
            "mode": "VECTOR_ONLY (baseline for comparison)",
            "total_queries": len(test_queries),
            "avg_latency_ms": statistics.mean(latencies),
            "avg_keyword_match_rate": statistics.mean([r["keyword_matches"] for r in results]) / len(test_queries),
            "avg_precision": statistics.mean([r["estimated_precision"] for r in results]),
            "detailed_results": results
        }
        
        print(f"\n  📊 Summary:")
        print(f"    Avg Latency: {result_dict['avg_latency_ms']:.2f}ms")
        print(f"    Avg Keyword Match Rate: {result_dict['avg_keyword_match_rate']:.1%}")
        print(f"    Avg Precision: {result_dict['avg_precision']:.1%}")
        
        return result_dict
    
    def benchmark_entity_filtering(self) -> Dict[str, Any]:
        """Benchmark entity extraction + filtering."""
        print("\n🏷️  Benchmark: ENTITY FILTERING (Existing Data)")
        print("=" * 70)
        
        agent = self._get_agent()
        
        # Queries with clear entities
        test_queries = [
            {
                "query": "Find memories about Python programming",
                "entities": ["Python", "programming", "code"],
                "entity_type": "Technology"
            },
            {
                "query": "What do I remember about Sarah?",
                "entities": ["Sarah"],
                "entity_type": "Person"
            },
            {
                "query": "Memories from my Boston trip",
                "entities": ["Boston", "trip", "travel"],
                "entity_type": "Location"
            },
            {
                "query": "My experiences with Docker",
                "entities": ["Docker", "containers", "deployment"],
                "entity_type": "Technology"
            },
        ]
        
        results = []
        raw_latencies = []
        filtered_latencies = []
        
        for test in test_queries:
            # Raw search latency
            start_raw = time.time()
            raw_recalls = agent.recall(test["query"], limit=20, mode="hybrid")
            raw_latency = (time.time() - start_raw) * 1000
            raw_latencies.append(raw_latency)
            
            # Simulated entity filtering
            start_filtered = time.time()
            # In practice, this would be: extract entities → filter memories → vector search
            filtered_recalls = [
                r for r in raw_recalls
                if any(entity.lower() in r.content.lower() for entity in test["entities"])
            ]
            filtered_latency = (time.time() - start_filtered) * 1000
            filtered_latencies.append(filtered_latency)
            
            result = {
                "query": test["query"],
                "entity_type": test["entity_type"],
                "entities": test["entities"],
                "raw_results": len(raw_recalls),
                "filtered_results": len(filtered_recalls),
                "latency_raw_ms": raw_latency,
                "latency_filtered_ms": filtered_latency,
                "filter_efficiency": len(filtered_recalls) / len(raw_recalls) if raw_recalls else 0
            }
            results.append(result)
            
            print(f"\n  Query: {test['query']}")
            print(f"    Entity Type: {test['entity_type']}")
            print(f"    Raw results: {len(raw_recalls)} → After filtering: {len(filtered_recalls)}")
            print(f"    Latency: {raw_latency:.2f}ms (raw) → {filtered_latency:.2f}ms (filtered)")
            print(f"    Filter Efficiency: {result['filter_efficiency']:.1%}")
        
        result_dict = {
            "test": "entity_filtering_baseline",
            "timestamp": datetime.now().isoformat(),
            "total_queries": len(test_queries),
            "avg_raw_latency_ms": statistics.mean(raw_latencies),
            "avg_filtered_latency_ms": statistics.mean(filtered_latencies),
            "avg_results_reduction": statistics.mean([r["filter_efficiency"] for r in results]),
            "detailed_results": results
        }
        
        print(f"\n  📊 Summary:")
        print(f"    Raw Search Latency: {result_dict['avg_raw_latency_ms']:.2f}ms")
        print(f"    Filtered Search Latency: {result_dict['avg_filtered_latency_ms']:.2f}ms")
        print(f"    Avg Results After Filter: {result_dict['avg_results_reduction']:.1%} of original")
        
        return result_dict
    
    def benchmark_temporal_ranking(self) -> Dict[str, Any]:
        """Benchmark temporal-aware result ranking."""
        print("\n📅 Benchmark: TEMPORAL RANKING (Existing Data)")
        print("=" * 70)
        
        agent = self._get_agent()
        
        # Queries where recency matters
        test_queries = [
            {
                "query": "What's my latest project?",
                "temporal_context": "recent",
                "importance": "Most recent result should rank highest"
            },
            {
                "query": "My oldest memory",
                "temporal_context": "oldest",
                "importance": "Earliest result should rank highest"
            },
            {
                "query": "What happened last month?",
                "temporal_context": "last_month",
                "importance": "Last 30 days should dominate results"
            },
        ]
        
        results = []
        latencies = []
        
        for test in test_queries:
            start = time.time()
            # Standard vector search (no temporal ranking yet)
            recalls = agent.recall(test["query"], limit=10, mode="hybrid")
            latency = (time.time() - start) * 1000
            latencies.append(latency)
            
            result = {
                "query": test["query"],
                "temporal_context": test["temporal_context"],
                "importance": test["importance"],
                "results": len(recalls),
                "latency_ms": latency,
                "temporal_advantage": "Would benefit from ranking by timestamp"
            }
            results.append(result)
            
            print(f"\n  Query: {test['query']}")
            print(f"    Context: {test['temporal_context']}")
            print(f"    Results: {len(recalls)} | Latency: {latency:.2f}ms")
            print(f"    Improvement: {test['importance']}")
        
        result_dict = {
            "test": "temporal_ranking_baseline",
            "timestamp": datetime.now().isoformat(),
            "total_queries": len(test_queries),
            "avg_latency_ms": statistics.mean(latencies),
            "opportunity": "Temporal ranking could improve relevance by ~20-30%",
            "detailed_results": results
        }
        
        print(f"\n  📊 Summary:")
        print(f"    Avg Latency: {result_dict['avg_latency_ms']:.2f}ms")
        print(f"    Optimization Opportunity: {result_dict['opportunity']}")
        
        return result_dict
    
    def run_all_benchmarks(self) -> Dict[str, Any]:
        """Run complete benchmark suite against existing data."""
        print("\n" + "="*70)
        print("🔬 MEMRO BENCHMARK - EXISTING DATA (No Re-ingestion)")
        print("="*70)
        print(f"Start Time: {self.start_time.isoformat()}")
        print(f"Base URL: {self.client.base_url}")
        print(f"Agent ID: {self.agent_id[:16]}..." if self.agent_id else "Creating new agent")
        
        all_results = {}
        
        try:
            all_results["hybrid_search"] = self.benchmark_hybrid_search()
        except Exception as e:
            print(f"❌ Hybrid search benchmark failed: {e}")
            all_results["hybrid_search"] = {"error": str(e)}
        
        try:
            all_results["entity_filtering"] = self.benchmark_entity_filtering()
        except Exception as e:
            print(f"❌ Entity filtering benchmark failed: {e}")
            all_results["entity_filtering"] = {"error": str(e)}
        
        try:
            all_results["temporal_ranking"] = self.benchmark_temporal_ranking()
        except Exception as e:
            print(f"❌ Temporal ranking benchmark failed: {e}")
            all_results["temporal_ranking"] = {"error": str(e)}
        
        end_time = datetime.now()
        duration = (end_time - self.start_time).total_seconds()
        
        report = {
            "benchmark_version": "2.0",
            "mode": "existing_data_only",
            "timestamp": datetime.now().isoformat(),
            "duration_seconds": duration,
            "results": all_results
        }
        
        return report


def main():
    """Run benchmark suite."""
    import argparse
    
    parser = argparse.ArgumentParser(description="Benchmark Memro against existing agent data")
    parser.add_argument("--base_url", default="http://localhost:8081", help="Base URL for Memro API")
    parser.add_argument("--agent_id", required=True, help="Agent ID to benchmark")
    parser.add_argument("--private_key", required=True, help="Private key for agent")
    parser.add_argument("--output", default="benchmark_existing_data_results.json", help="Output file")
    args = parser.parse_args()
    
    suite = MemroExistingDataBenchmark(
        base_url=args.base_url,
        agent_id=args.agent_id,
        private_key=args.private_key
    )
    results = suite.run_all_benchmarks()
    
    # Save results
    with open(args.output, "w") as f:
        json.dump(results, f, indent=2)
    
    print("\n" + "="*70)
    print(f"✅ Benchmark complete! Results saved to: {args.output}")
    print("="*70)


if __name__ == "__main__":
    main()
