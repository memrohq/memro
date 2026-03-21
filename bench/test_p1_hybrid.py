#!/usr/bin/env python3
"""
P1 Test: Hybrid BM25+Vector Search Evaluation
Compare semantic-only vs hybrid search on existing 326K memories
"""

import time
import json
import sys
from typing import List, Dict, Any
from datetime import datetime

try:
    from memro import Agent, MemroClient
except ImportError:
    print("❌ Error: memro SDK not installed")
    sys.exit(1)


class P1HybridSearchTest:
    """Test P1 hybrid search against existing data."""
    
    def __init__(self, agent_id: str, private_key: str, base_url: str = "http://localhost:8081"):
        """Initialize with agent credentials."""
        self.client = MemroClient(base_url)
        self.agent = Agent(agent_id=agent_id, private_key=private_key, client=self.client)
        self.agent_id = agent_id
        self.base_url = base_url
        
    def test_search_modes(self) -> Dict[str, Any]:
        """Compare semantic vs keyword vs hybrid search results."""
        print("\n" + "="*80)
        print("📊 P1 HYBRID SEARCH TEST - Comparing Search Modes")
        print("="*80)
        print(f"Agent: {self.agent_id[:16]}...")
        print(f"Server: {self.base_url}")
        
        # Representative test queries
        test_queries = [
            {
                "query": "What are my recent projects?",
                "intent": "Temporal + specific domain",
                "keywords": ["project", "recent", "work", "current"]
            },
            {
                "query": "Tell me about my family members",
                "intent": "Entity lookup (person names)",
                "keywords": ["family", "member", "relative", "parent", "sibling"]
            },
            {
                "query": "How do I prefer to communicate?",
                "intent": "Preference/personality",
                "keywords": ["prefer", "communicate", "email", "phone", "message"]
            },
            {
                "query": "What skills do I have in Python?",
                "intent": "Technology + capability",
                "keywords": ["Python", "skill", "programming", "code", "library"]
            },
            {
                "query": "Which cities have I visited?",
                "intent": "Location history",
                "keywords": ["city", "visited", "travel", "trip", "location"]
            }
        ]
        
        all_results = []
        
        for idx, test_query in enumerate(test_queries, 1):
            query = test_query["query"]
            keywords = test_query["keywords"]
            intent = test_query["intent"]
            
            print(f"\n{'─'*80}")
            print(f"Query {idx}: {query}")
            print(f"  Intent: {intent}")
            print(f"  Keywords: {', '.join(keywords)}")
            
            # Test all three modes
            modes = ["semantic", "keyword", "hybrid"]
            mode_results = {}
            
            for mode in modes:
                try:
                    # Run search with specific mode
                    start = time.time()
                    results = self.agent.recall(
                        query=query,
                        limit=10,
                        mode=mode
                    )
                    latency = (time.time() - start) * 1000
                    
                    # Count keyword matches
                    keyword_matches = sum(
                        1 for result in results
                        if any(kw.lower() in result.content.lower() for kw in keywords)
                    )
                    
                    # Calculate relevance score
                    avg_score = (sum(r.score for r in results) / len(results)) if results else 0
                    
                    mode_results[mode] = {
                        "count": len(results),
                        "keyword_matches": keyword_matches,
                        "keyword_match_rate": keyword_matches / len(results) if results else 0,
                        "avg_score": avg_score,
                        "latency_ms": latency,
                        "top_score": results[0].score if results else 0
                    }
                    
                    print(f"\n  🔹 {mode.upper()} Search:")
                    print(f"     Results: {len(results)}")
                    print(f"     Keyword Matches: {keyword_matches}/{len(results)} ({mode_results[mode]['keyword_match_rate']*100:.1f}%)")
                    print(f"     Avg Score: {avg_score:.4f}")
                    print(f"     Latency: {latency:.2f}ms")
                    
                except Exception as e:
                    print(f"\n  ❌ {mode.upper()} Error: {str(e)}")
                    mode_results[mode] = {"error": str(e)}
            
            # Compare modes
            if "semantic" in mode_results and "hybrid" in mode_results:
                sem = mode_results["semantic"]
                hyb = mode_results["hybrid"]
                
                if "error" not in sem and "error" not in hyb:
                    # Calculate improvement
                    match_improvement = (hyb["keyword_match_rate"] - sem["keyword_match_rate"]) * 100
                    latency_diff = hyb["latency_ms"] - sem["latency_ms"]
                    
                    print(f"\n  📈 Hybrid vs Semantic Comparison:")
                    print(f"     Keyword Match Improvement: {match_improvement:+.1f}%")
                    print(f"     Latency Difference: {latency_diff:+.2f}ms")
                    print(f"     Results Count Diff: {hyb['count'] - sem['count']:+d}")
            
            # Store results
            all_results.append({
                "query": query,
                "intent": intent,
                "results": mode_results
            })
        
        # Calculate aggregate metrics
        print(f"\n{'='*80}")
        print("📊 AGGREGATE RESULTS")
        print(f"{'='*80}")
        
        hybrid_keyword_rates = []
        semantic_keyword_rates = []
        hybrid_latencies = []
        semantic_latencies = []
        
        for test in all_results:
            results = test["results"]
            if "hybrid" in results and "error" not in results["hybrid"]:
                hybrid_keyword_rates.append(results["hybrid"]["keyword_match_rate"])
                hybrid_latencies.append(results["hybrid"]["latency_ms"])
            if "semantic" in results and "error" not in results["semantic"]:
                semantic_keyword_rates.append(results["semantic"]["keyword_match_rate"])
                semantic_latencies.append(results["semantic"]["latency_ms"])
        
        # Calculate averages
        if hybrid_keyword_rates and semantic_keyword_rates:
            avg_hybrid_match = sum(hybrid_keyword_rates) / len(hybrid_keyword_rates)
            avg_semantic_match = sum(semantic_keyword_rates) / len(semantic_keyword_rates)
            avg_hybrid_lat = sum(hybrid_latencies) / len(hybrid_latencies)
            avg_semantic_lat = sum(semantic_latencies) / len(semantic_latencies)
            
            match_improvement = (avg_hybrid_match - avg_semantic_match) * 100
            latency_overhead = avg_hybrid_lat - avg_semantic_lat
            
            print(f"\n🎯 Key Metrics:")
            print(f"  Hybrid Keyword Match Rate:   {avg_hybrid_match*100:.1f}%")
            print(f"  Semantic Keyword Match Rate: {avg_semantic_match*100:.1f}%")
            print(f"  📈 Improvement: {match_improvement:+.1f}%")
            print(f"\n  Hybrid Avg Latency:   {avg_hybrid_lat:.2f}ms")
            print(f"  Semantic Avg Latency: {avg_semantic_lat:.2f}ms")
            print(f"  ⏱️  Overhead: {latency_overhead:+.2f}ms")
        
        return {
            "timestamp": datetime.now().isoformat(),
            "agent_id": self.agent_id,
            "test": "p1_hybrid_search_comparison",
            "test_count": len(test_queries),
            "results": all_results,
            "summary": {
                "avg_hybrid_keyword_match_rate": sum(hybrid_keyword_rates) / len(hybrid_keyword_rates) if hybrid_keyword_rates else 0,
                "avg_semantic_keyword_match_rate": sum(semantic_keyword_rates) / len(semantic_keyword_rates) if semantic_keyword_rates else 0,
                "avg_hybrid_latency_ms": sum(hybrid_latencies) / len(hybrid_latencies) if hybrid_latencies else 0,
                "avg_semantic_latency_ms": sum(semantic_latencies) / len(semantic_latencies) if semantic_latencies else 0,
                "improvement_percent": ((sum(hybrid_keyword_rates) / len(hybrid_keyword_rates) if hybrid_keyword_rates else 0) - 
                                      (sum(semantic_keyword_rates) / len(semantic_keyword_rates) if semantic_keyword_rates else 0)) * 100
            }
        }
    
    def save_results(self, results: Dict[str, Any], filename: str = None) -> str:
        """Save test results to JSON."""
        if not filename:
            filename = f"p1_hybrid_test_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"
        
        with open(filename, "w") as f:
            json.dump(results, f, indent=2)
        
        print(f"\n✅ Results saved to: {filename}")
        return filename


def main():
    """Run P1 hybrid search test."""
    import argparse
    
    parser = argparse.ArgumentParser(description="Test P1 Hybrid Search Implementation")
    parser.add_argument("--agent_id", required=True, help="Agent ID")
    parser.add_argument("--private_key", required=True, help="Private key")
    parser.add_argument("--base_url", default="http://localhost:8081", help="API base URL")
    parser.add_argument("--output", help="Output file for results")
    
    args = parser.parse_args()
    
    try:
        tester = P1HybridSearchTest(
            agent_id=args.agent_id,
            private_key=args.private_key,
            base_url=args.base_url
        )
        
        results = tester.test_search_modes()
        tester.save_results(results, args.output)
        
        print(f"\n{'='*80}")
        print("✅ P1 HYBRID SEARCH TEST COMPLETE")
        print(f"{'='*80}\n")
        
    except Exception as e:
        print(f"\n❌ Error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()
