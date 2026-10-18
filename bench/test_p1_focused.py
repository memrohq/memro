#!/usr/bin/env python3
"""
P1 Test: Hybrid Search - Focused Test on Actual Data
Tests with queries that match the 3,255 memories in test agent
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


class P1FocusedTest:
    """Focused P1 test on real data."""
    
    def __init__(self, agent_id: str, private_key: str, base_url: str = "http://localhost:8081"):
        """Initialize with agent credentials."""
        self.client = MemroClient(base_url)
        self.agent = Agent(agent_id=agent_id, private_key=private_key, client=self.client)
        self.agent_id = agent_id
        self.base_url = base_url
        
    def test_hybrid_search_modes(self) -> Dict[str, Any]:
        """Test hybrid search with queries matching actual data."""
        print("\n" + "="*80)
        print("🚀 P1 TEST: HYBRID SEARCH PERFORMANCE EVALUATION")
        print("="*80)
        print(f"Agent: {self.agent_id[:16]}...")
        print(f"Server: {self.base_url}")
        
        # Queries matched to actual data in test agent
        test_queries = [
            {
                "query": "React and TypeScript projects",
                "description": "Technology stack query",
                "keywords": ["React", "TypeScript", "project", "completed"]
            },
            {
                "query": "Did I meet Sarah about roadmap?",
                "description": "Person + meeting query",
                "keywords": ["Sarah", "meeting", "roadmap", "Q2"]
            },
            {
                "query": "Central Park running",
                "description": "Activity query",
                "keywords": ["Central Park", "run", "morning", "miles"]
            },
            {
                "query": "Italy vacation",
                "description": "Travel query",
                "keywords": ["Italy", "travel", "Rome", "Venice"]
            },
            {
                "query": "chess rating",
                "description": "Hobby query",
                "keywords": ["chess", "online", "rating"]
            }
        ]
        
        all_results = []
        
        for idx, test_query in enumerate(test_queries, 1):
            query = test_query["query"]
            keywords = test_query["keywords"]
            description = test_query["description"]
            
            print(f"\n{'─'*80}")
            print(f"Test {idx}: {query}")
            print(f"  Category: {description}")
            
            # Test both modes
            modes = ["semantic", "hybrid"]
            mode_results = {}
            
            for mode in modes:
                try:
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
                    
                    # Calculate metrics
                    avg_score = (sum(r.score for r in results) / len(results)) if results else 0
                    
                    mode_results[mode] = {
                        "results_found": len(results),
                        "keyword_matches": keyword_matches,
                        "keyword_match_rate": keyword_matches / len(results) if results else 0,
                        "avg_score": avg_score,
                        "latency_ms": latency,
                        "top_score": results[0].score if results else 0,
                        "results": [{"content": r.content[:80], "score": r.score} for r in results[:3]]
                    }
                    
                    print(f"\n  📊 {mode.upper()} Search:")
                    print(f"     Results: {len(results)}")
                    print(f"     Keyword Matches: {keyword_matches}/{len(results)} ({keyword_matches/len(results)*100:.1f}%)" if results else "     No results found")
                    print(f"     Avg Score: {avg_score:.4f}")
                    print(f"     Latency: {latency:.2f}ms")
                    
                    if results:
                        print(f"     Top result: {results[0].content[:60]}...")
                    
                except Exception as e:
                    print(f"\n  ❌ {mode.upper()} Error: {str(e)}")
                    mode_results[mode] = {"error": str(e)}
            
            # Compare improvements
            if "semantic" in mode_results and "hybrid" in mode_results:
                sem = mode_results["semantic"]
                hyb = mode_results["hybrid"]
                
                if "error" not in  sem and "error" not in hyb:
                    # Calculate improvements
                    result_diff = hyb["results_found"] - sem["results_found"]
                    match_improvement = (hyb["keyword_match_rate"] - sem["keyword_match_rate"]) * 100
                    latency_diff = hyb["latency_ms"] - sem["latency_ms"]
                    
                    print(f"\n  📈 Hybrid vs Semantic:")
                    print(f"     Results Difference: {result_diff:+d}")
                    print(f"     Keyword Match Improvement: {match_improvement:+.1f}%")
                    print(f"     Latency Difference: {latency_diff:+.2f}ms")
                    
                    if result_diff > 0 or match_improvement > 0:
                        print(f"     ✅ HYBRID WINS")
                    elif latency_diff < 0:
                        print(f"     ⚡ HYBRID FASTER ({abs(latency_diff):.1f}ms faster)")
            
            all_results.append({
                "query": query,
                "category": description,
                "results": mode_results
            })
        
        # Aggregate analysis
        print(f"\n{'='*80}")
        print("📊 AGGREGATE ANALYSIS")
        print(f"{'='*80}")
        
        hybrid_results_found  = []
        semantic_results_found = []
        hybrid_latencies = []
        semantic_latencies = []
        hybrid_match_rates = []
        semantic_match_rates = []
        
        for test in all_results:
            results = test["results"]
            if "hybrid" in results and "error" not in results["hybrid"]:
                hybrid_results_found.append(results["hybrid"]["results_found"])
                hybrid_latencies.append(results["hybrid"]["latency_ms"])
                hybrid_match_rates.append(results["hybrid"]["keyword_match_rate"])
            if "semantic" in results and "error" not in results["semantic"]:
                semantic_results_found.append(results["semantic"]["results_found"])
                semantic_latencies.append(results["semantic"]["latency_ms"])
                semantic_match_rates.append(results["semantic"]["keyword_match_rate"])
        
        # Print comparisons
        if hybrid_results_found and semantic_results_found:
            print(f"\n📌 Results Found:")
            print(f"   Hybrid:    {sum(hybrid_results_found)} results ({sum(hybrid_results_found)/len(hybrid_results_found):.1f} avg/query)")
            print(f"   Semantic:  {sum(semantic_results_found)} results ({sum(semantic_results_found)/len(semantic_results_found):.1f} avg/query)")
            print(f"   Difference: {sum(hybrid_results_found) - sum(semantic_results_found):+d} ({((sum(hybrid_results_found)/sum(semantic_results_found)-1)*100) if sum(semantic_results_found) > 0 else 0:+.1f}%)")
            
        if hybrid_latencies and semantic_latencies:
            avg_hybrid_lat = sum(hybrid_latencies) / len(hybrid_latencies)
            avg_semantic_lat = sum(semantic_latencies) / len(semantic_latencies)
            print(f"\n⏱️  Latency:")
            print(f"   Hybrid:    {avg_hybrid_lat:.2f}ms avg")
            print(f"   Semantic:  {avg_semantic_lat:.2f}ms avg")
            print(f"   Difference: {avg_hybrid_lat - avg_semantic_lat:+.2f}ms")
            if avg_hybrid_lat < avg_semantic_lat:
                print(f"   ✅ Hybrid is {((avg_semantic_lat/avg_hybrid_lat - 1)*100):.1f}% faster")
            
        if hybrid_match_rates and semantic_match_rates:
            avg_hybrid_match = sum(hybrid_match_rates) / len(hybrid_match_rates)
            avg_semantic_match = sum(semantic_match_rates) / len(semantic_match_rates)
            print(f"\n🎯 Keyword Match Rate:")
            print(f"   Hybrid:    {avg_hybrid_match*100:.1f}%")
            print(f"   Semantic:  {avg_semantic_match*100:.1f}%")
            print(f"   Improvement: {(avg_hybrid_match - avg_semantic_match)*100:+.1f}%")
        
        # Final verdict
        print(f"\n{'─'*80}")
        if hybrid_latencies and semantic_latencies:
            avg_h_lat = sum(hybrid_latencies) / len(hybrid_latencies)
            avg_s_lat = sum(semantic_latencies) / len(semantic_latencies)
            
            if sum(hybrid_results_found) > sum(semantic_results_found) and avg_h_lat < 50:
                print("✅ P1 HYBRID SEARCH: POSITIVE - More results + Fast latency")
            elif avg_h_lat < avg_s_lat:
                print("✅ P1 HYBRID SEARCH: POSITIVE - Significantly faster")
            elif sum(hybrid_results_found) >= sum(semantic_results_found):
                print("✅ P1 HYBRID SEARCH: NEUTRAL - Same/better results with acceptable latency")
            else:
                print("⚠️  P1 HYBRID SEARCH: Check implementation")
        
        return {
            "timestamp": datetime.now().isoformat(),
            "agent_id": self.agent_id,
            "test_name": "p1_hybrid_focused_test",
            "test_count": len(test_queries),
            "results": all_results,
            "summary": {
                "total_hybrid_results": sum(hybrid_results_found) if hybrid_results_found else 0,
                "total_semantic_results": sum(semantic_results_found) if semantic_results_found else 0,
                "avg_hybrid_latency_ms": sum(hybrid_latencies) / len(hybrid_latencies) if hybrid_latencies else 0,
                "avg_semantic_latency_ms": sum(semantic_latencies) / len(semantic_latencies) if semantic_latencies else 0,
                "avg_hybrid_match_rate": sum(hybrid_match_rates) / len(hybrid_match_rates) if hybrid_match_rates else 0,
                "avg_semantic_match_rate": sum(semantic_match_rates) / len(semantic_match_rates) if semantic_match_rates else 0
            }
        }
    
    def save_results(self, results: Dict[str, Any], filename: str = None) -> str:
        """Save results to JSON."""
        if not filename:
            filename = f"p1_focused_results_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"
        
        with open(filename, "w") as f:
            json.dump(results, f, indent=2)
        
        print(f"\n💾 Results saved to: {filename}")
        return filename


def main():
    import argparse
    
    parser = argparse.ArgumentParser(description="P1 Focused Hybrid Search Test")
    parser.add_argument("--agent_id", required=True)
    parser.add_argument("--private_key", required=True)
    parser.add_argument("--base_url", default="http://localhost:8081")
    parser.add_argument("--output")
    
    args = parser.parse_args()
    
    try:
        tester = P1FocusedTest(args.agent_id, args.private_key, args.base_url)
        results = tester.test_hybrid_search_modes()
        tester.save_results(results, args.output)
        
        print(f"\n{'='*80}")
        print("✅ P1 FOCUSED TEST COMPLETE")
        print(f"{'='*80}\n")
        
    except Exception as e:
        print(f"\n❌ Error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()
