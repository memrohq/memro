#!/usr/bin/env python3
"""
Benchmark Comparison Tool
Compare before/after benchmark results to measure improvement impact
"""

import json
import sys
from typing import Dict, Any
from datetime import datetime


class BenchmarkComparison:
    """Compare two benchmark results."""
    
    def __init__(self, baseline_file: str, improved_file: str):
        """Load benchmark files."""
        with open(baseline_file) as f:
            self.baseline = json.load(f)
        
        with open(improved_file) as f:
            self.improved = json.load(f)
    
    def compare_latency(self, test_name: str) -> Dict[str, Any]:
        """Compare latency improvements."""
        # Handle nested structure
        baseline_data = self.baseline.get("raw_results", {}).get("results", {}).get(test_name, {})
        improved_data = self.improved.get("raw_results", {}).get("results", {}).get(test_name, {})
        
        baseline_latency = baseline_data.get("avg_latency_ms", 0)
        improved_latency = improved_data.get("avg_latency_ms", 0)
        
        # Fallback to top-level metrics
        if not baseline_latency:
            baseline_latency = self.baseline.get("metrics", {}).get("latency", {}).get("mean_ms", 0)
        if not improved_latency:
            improved_latency = self.improved.get("metrics", {}).get("latency", {}).get("mean_ms", 0)
        
        if not baseline_latency or not improved_latency:
            return {"error": "Latency data not found"}
        
        improvement = ((baseline_latency - improved_latency) / baseline_latency) * 100
        degradation = improved_latency - baseline_latency
        
        return {
            "test": test_name,
            "baseline_ms": round(baseline_latency, 2),
            "improved_ms": round(improved_latency, 2),
            "improvement_percent": round(improvement, 1),
            "change_ms": round(degradation, 2),
            "status": "✅ FASTER" if improvement > 0 else ("❌ SLOWER" if improvement < 0 else "➡️  SAME")
        }
    
    def compare_recall(self, test_name: str) -> Dict[str, Any]:
        """Compare recall/accuracy improvements."""
        baseline_data = self.baseline.get("results", {}).get(test_name, {})
        improved_data = self.improved.get("results", {}).get(test_name, {})
        
        # Try different metric names
        baseline_recall = (
            baseline_data.get("avg_recall") or 
            baseline_data.get("avg_chain_coverage") or
            0
        )
        improved_recall = (
            improved_data.get("avg_recall") or 
            improved_data.get("avg_chain_coverage") or
            0
        )
        
        if not baseline_recall or not improved_recall:
            return {"error": "Recall data not found"}
        
        improvement = ((improved_recall - baseline_recall) / baseline_recall) * 100 if baseline_recall > 0 else 0
        
        return {
            "test": test_name,
            "baseline": round(baseline_recall, 3),
            "improved": round(improved_recall, 3),
            "improvement_percent": round(improvement, 1),
            "status": "✅ IMPROVED" if improvement > 0 else ("⚠️  REGRESSED" if improvement < 0 else "➡️  SAME")
        }
    
    def generate_report(self) -> str:
        """Generate comparison report."""
        report = []
        report.append("\n" + "="*80)
        report.append("📊 MEMRO BENCHMARK COMPARISON REPORT")
        report.append("="*80)
        
        # Metadata
        report.append(f"\nBaseline:  {self.baseline.get('timestamp', 'N/A')}")
        report.append(f"Improved:  {self.improved.get('timestamp', 'N/A')}")
        baseline_duration = self.baseline.get('duration_seconds', 0)
        improved_duration = self.improved.get('duration_seconds', 0)
        report.append(f"Baseline Duration:  {baseline_duration:.1f}s")
        report.append(f"Improved Duration:  {improved_duration:.1f}s")
        
        # Latency Comparisons
        report.append("\n" + "="*80)
        report.append("⚡ LATENCY COMPARISON")
        report.append("="*80)
        
        latency_tests = ["hybrid_search", "entity_filtering", "temporal_ranking", "query_latency"]
        for test in latency_tests:
            comparison = self.compare_latency(test)
            if "error" not in comparison:
                report.append(f"\n{test.upper()}")
                report.append(f"  Baseline:  {comparison['baseline_ms']}ms")
                report.append(f"  Improved:  {comparison['improved_ms']}ms")
                report.append(f"  Change:    {comparison['change_ms']:+.2f}ms ({comparison['improvement_percent']:+.1f}%) {comparison['status']}")
        
        # Recall Comparisons
        report.append("\n" + "="*80)
        report.append("📈 RECALL & ACCURACY COMPARISON")
        report.append("="*80)
        
        recall_tests = ["hybrid_search", "entity_filtering", "agent_isolation", "knowledge_chains"]
        for test in recall_tests:
            comparison = self.compare_recall(test)
            if "error" not in comparison:
                report.append(f"\n{test.upper()}")
                report.append(f"  Baseline:  {comparison['baseline']}")
                report.append(f"  Improved:  {comparison['improved']}")
                report.append(f"  Gain:      {comparison['improvement_percent']:+.1f}% {comparison['status']}")
        
        # Summary
        report.append("\n" + "="*80)
        report.append("📋 SUMMARY")
        report.append("="*80)
        
        # Overall assessment
        improved_mode = self.improved.get("mode", "unknown")
        baseline_mode = self.baseline.get("mode", "unknown")
        
        report.append(f"\nBaseline Mode: {baseline_mode}")
        report.append(f"Improved Mode: {improved_mode}")
        
        # Calculate overall metrics
        try:
            baseline_recall = self.baseline.get("results", {}).get("hybrid_search", {}).get("avg_recall", 0)
            improved_recall = self.improved.get("results", {}).get("hybrid_search", {}).get("avg_recall", 0)
            
            if baseline_recall and improved_recall:
                recall_gain = improved_recall - baseline_recall
                report.append(f"\n🎯 Overall Recall Improvement: {recall_gain:+.3f} ({(recall_gain/baseline_recall*100):+.1f}%)")
        except:
            pass
        
        report.append("\n" + "="*80 + "\n")
        
        return "\n".join(report)
    
    def save_comparison(self, output_file: str = None):
        """Save comparison as JSON."""
        if not output_file:
            output_file = f"benchmark_comparison_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"
        
        comparison = {
            "timestamp": datetime.now().isoformat(),
            "baseline": self.baseline,
            "improved": self.improved,
            "comparison": {
                "latency": {test: self.compare_latency(test) for test in ["hybrid_search", "entity_filtering"]},
                "recall": {test: self.compare_recall(test) for test in ["hybrid_search", "entity_filtering"]}
            }
        }
        
        with open(output_file, "w") as f:
            json.dump(comparison, f, indent=2)
        
        print(f"✅ Comparison saved to: {output_file}")
        return output_file


def main():
    """Compare benchmark files."""
    import argparse
    
    parser = argparse.ArgumentParser(description="Compare benchmark results")
    parser.add_argument("baseline", help="Baseline benchmark.json file")
    parser.add_argument("improved", help="Improved benchmark.json file")
    parser.add_argument("--output", help="Output comparison file (optional)")
    args = parser.parse_args()
    
    try:
        comparison = BenchmarkComparison(args.baseline, args.improved)
        report = comparison.generate_report()
        print(report)
        
        if args.output:
            comparison.save_comparison(args.output)
        else:
            comparison.save_comparison()
    
    except Exception as e:
        print(f"❌ Error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
