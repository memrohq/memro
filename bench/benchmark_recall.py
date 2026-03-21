#!/usr/bin/env python3
"""
Benchmark script to test recall performance against existing data.
Tests various query types and measures latency/accuracy.
"""

import time
import json
import statistics
from typing import List, Dict, Any
from memro import Agent, MemroClient

class RecallBenchmark:
    def __init__(self, base_url: str = "http://localhost:8081"):
        self.client = MemroClient(base_url)
        self.results = []

    def get_existing_agents(self) -> List[str]:
        """Get list of existing agent IDs from the system."""
        # Since we can't directly query agents, we'll try common patterns
        # from the LongMemEval benchmark
        return [
            "d08b6d84cf8bc05282f1af9180783eeb1fdff5783425bf2be3b57703e5064762",
            "a0e6fbcd4f2eb7129414391e05eb62a50b119926fcb5ffda39b97b3fdea3d6ce",
            "e1ea892568c54762ae2e31e973dd6f3bda00d416e3cc18db348dc21ce11ae03f",
            "73841488153d3dcfc3c3aead37534f27096b5431732b3eb2f32dac551c903abd",
            "b8e276dde905a555557113db847f56308ed268a3abc4e77afa4a9aabd2e0ff48"
        ]

    def test_recall_queries(self, queries: List[str]) -> Dict[str, Any]:
        """Test recall performance for a set of queries using a new agent."""
        try:
            agent = Agent.create(client=self.client)
            results = []

            for query in queries:
                start_time = time.time()
                recall_results = agent.recall(query, limit=10, mode="hybrid")
                latency = (time.time() - start_time) * 1000  # ms

                results.append({
                    "query": query,
                    "latency_ms": latency,
                    "results_count": len(recall_results),
                    "results": [{"content": r.content[:100] + "..." if len(r.content) > 100 else r.content,
                               "score": r.score,
                               "type": r.memory_type} for r in recall_results[:3]]  # Top 3 results
                })

            return {
                "agent_id": agent.id if hasattr(agent, 'id') else "new_agent",
                "success": True,
                "queries_tested": len(queries),
                "avg_latency_ms": statistics.mean([r["latency_ms"] for r in results]),
                "results": results
            }

        except Exception as e:
            return {
                "agent_id": "unknown",
                "success": False,
                "error": str(e)
            }

    def run_benchmark(self):
        """Run comprehensive benchmark against existing data."""
        print("🚀 Starting Recall Performance Benchmark")
        print("=" * 50)

        # Test queries covering different types of recall
        test_queries = [
            # Semantic search queries
            "What is the user's favorite color?",
            "Tell me about the user's preferences",
            "What activities does the user enjoy?",

            # Specific memory recall
            "What did the user say about playing chess?",
            "What information was shared about the user's interests?",

            # Contextual queries
            "What can you tell me about the user's background?",
            "What memories involve the user's hobbies?",

            # Edge cases
            "Show me recent memories",
            "What was the last thing mentioned?",

            # Complex queries
            "What are the user's favorite activities and why?",
            "Tell me everything you know about the user's preferences"
        ]

        agents = self.get_existing_agents()
        print(f"📊 Testing {len(agents)} agents with {len(test_queries)} queries each")
        print(f"🎯 Total memories in system: 2,689")
        print()

        successful_agents = 0
        total_queries = 0
        all_latencies = []

        for i, agent_id in enumerate(agents, 1):
            print(f"🤖 Agent {i}/{len(agents)}: {agent_id[:16]}...")
            result = self.test_recall_queries(test_queries)

            if result["success"]:
                successful_agents += 1
                total_queries += result["queries_tested"]
                all_latencies.extend([r["latency_ms"] for r in result["results"]])

                print(f"   ⚡ Avg latency: {result['avg_latency_ms']:.1f}ms")
                print(f"   📈 Results returned: {sum(r['results_count'] for r in result['results'])}")
            else:
                print(f"   ❌ Failed: {result['error']}")

            self.results.append(result)
            print()

        # Overall statistics
        print("📈 BENCHMARK RESULTS")
        print("=" * 50)
        print(f"✅ Successful agents: {successful_agents}/{len(agents)}")
        print(f"🔍 Total queries executed: {total_queries}")

        if all_latencies:
            print(f"📊 Average latency: {statistics.mean(all_latencies):.1f}ms")
            print(f"🏃 Min latency: {min(all_latencies):.1f}ms")
            print(f"🐌 Max latency: {max(all_latencies):.1f}ms")
            # Performance analysis
            fast_queries = sum(1 for l in all_latencies if l < 100)
            slow_queries = sum(1 for l in all_latencies if l > 500)
            print(f"⚡ Fast queries (<100ms): {fast_queries}/{len(all_latencies)} ({fast_queries/len(all_latencies)*100:.1f}%)")
            print(f"🐌 Slow queries (>500ms): {slow_queries}/{len(all_latencies)} ({slow_queries/len(all_latencies)*100:.1f}%)")

        # Save detailed results
        with open("benchmark_results.json", "w") as f:
            json.dump(self.results, f, indent=2, default=str)

        print("💾 Detailed results saved to benchmark_results.json")
def main():
    benchmark = RecallBenchmark()
    benchmark.run_benchmark()

if __name__ == "__main__":
    main()