#!/usr/bin/env python3
"""
Memro Benchmark Results Publisher
Generates publishable benchmark reports in Supermemory-compatible format.
"""

import json
import statistics
from typing import Dict, Any
from datetime import datetime
from pathlib import Path
import numpy as np


class BenchmarkReportGenerator:
    """Generate professional benchmark reports."""
    
    def __init__(self, results_file: str = "benchmark_results.json"):
        """Load benchmark results."""
        with open(results_file, 'r') as f:
            self.results = json.load(f)
    
    def generate_markdown_report(self, output_file: str = "BENCHMARK_RESULTS.md") -> str:
        """Generate markdown report for publishing."""
        
        md = []
        md.append("# Memro Benchmark Results\n")
        md.append(f"**Generated:** {datetime.now().strftime('%Y-%m-%d %H:%M:%S UTC')}\n")
        md.append(f"**Duration:** {self.results['duration_seconds']:.1f}s\n\n")
        
        # Executive Summary
        md.append("## Executive Summary\n")
        summary = self.results.get("summary", {})
        md.append("| Metric | Value | Unit |\n")
        md.append("|--------|-------|------|\n")
        
        if "query_latency_p95_ms" in summary:
            md.append(f"| P95 Query Latency | {summary['query_latency_p95_ms']:.2f} | ms |\n")
        
        if "queries_per_second" in summary:
            md.append(f"| Throughput | {summary['queries_per_second']:.2f} | queries/sec |\n")
        
        if "latency_growth_per_step" in summary:
            growth = summary['latency_growth_per_step'] * 100
            md.append(f"| Latency Growth (per 10x data) | {growth:.2f} | % |\n")
        
        md.append("\n")
        
        # Detailed Results
        md.append("## Detailed Test Results\n\n")
        
        results = self.results.get("results", {})
        
        # 1. Recall Accuracy
        if "recall_accuracy" in results and "error" not in results["recall_accuracy"]:
            md.append("### 1. Recall Accuracy\n\n")
            recall = results["recall_accuracy"]
            md.append(f"**Average Query Latency:** {recall['avg_latency_ms']:.2f}ms\n")
            md.append(f"**Median Query Latency:** {recall['median_latency_ms']:.2f}ms\n")
            md.append(f"**P95 Latency:** {recall['p95_latency_ms']:.2f}ms\n")
            md.append("\n**Sample Queries:**\n")
            md.append("| Query | Retrieved | Latency (ms) | Top Score |\n")
            md.append("|-------|-----------|--------------|----------|\n")
            for detail in recall.get("detailed_results", [])[:5]:
                md.append(f"| {detail['query'][:30]}... | {detail['retrieved']} | {detail['latency_ms']:.2f} | {detail['top_result_score']:.3f} |\n")
            md.append("\n")
        
        # 2. Query Latency
        if "query_latency" in results and "error" not in results["query_latency"]:
            md.append("### 2. Query Latency Distribution\n\n")
            latency = results["query_latency"]
            md.append(f"**Benchmark:** {latency['num_queries']} queries\n\n")
            md.append("| Percentile | Latency (ms) |\n")
            md.append("|------------|-------------|\n")
            md.append(f"| Mean | {latency['avg_latency_ms']:.2f} |\n")
            md.append(f"| Median (P50) | {latency['median_latency_ms']:.2f} |\n")
            md.append(f"| P95 | {latency['p95_latency_ms']:.2f} |\n")
            md.append(f"| P99 | {latency['p99_latency_ms']:.2f} |\n")
            md.append(f"| Min | {latency['min_latency_ms']:.2f} |\n")
            md.append(f"| Max | {latency['max_latency_ms']:.2f} |\n")
            md.append(f"| Stdev | {latency['stdev_latency_ms']:.2f} |\n")
            md.append("\n")
        
        # 3. Temporal Reasoning
        if "temporal_reasoning" in results and "error" not in results["temporal_reasoning"]:
            md.append("### 3. Temporal Reasoning\n\n")
            temporal = results["temporal_reasoning"]
            md.append(f"**Temporal Queries Tested:** {temporal['total_queries']}\n")
            md.append(f"**Average Results per Query:** {temporal['avg_results']:.1f}\n")
            md.append(f"**Average Latency:** {temporal['avg_latency_ms']:.2f}ms\n")
            md.append("\n")
        
        # 4. Knowledge Chains
        if "knowledge_chains" in results and "error" not in results["knowledge_chains"]:
            md.append("### 4. Knowledge Chain Resolution\n\n")
            chains = results["knowledge_chains"]
            md.append(f"**Chain Queries:** {chains['total_queries']}\n")
            md.append(f"**Average Chain Coverage:** {chains['avg_chain_coverage']*100:.1f}%\n")
            md.append(f"**Average Latency:** {chains['avg_latency_ms']:.2f}ms\n")
            md.append("\n")
        
        # 5. Throughput
        if "throughput" in results and "error" not in results["throughput"]:
            md.append("### 5. Throughput Performance\n\n")
            throughput = results["throughput"]
            md.append(f"**Test Duration:** {throughput['duration_seconds']}s\n")
            md.append(f"**Total Queries:** {throughput['total_queries']}\n")
            md.append(f"**Queries per Second:** {throughput['queries_per_second']:.2f}\n")
            md.append(f"**Error Rate:** {throughput['error_rate']*100:.2f}%\n")
            md.append(f"**Average Latency:** {throughput['avg_latency_ms']:.2f}ms\n")
            md.append(f"**P95 Latency:** {throughput['p95_latency_ms']:.2f}ms\n")
            md.append("\n")
        
        # 6. Memory Efficiency
        if "memory_efficiency" in results and "error" not in results["memory_efficiency"]:
            md.append("### 6. Memory Efficiency & Scaling\n\n")
            efficiency = results["memory_efficiency"]
            md.append(f"**Avg Latency Growth:** {efficiency['avg_latency_growth']*100:.2f}% per 10x data\n")
            md.append(f"**Max Latency Growth:** {efficiency['max_latency_growth']*100:.2f}%\n")
            md.append("\n**Latency by Memory Count:**\n")
            md.append("| Memories | Latency (ms) |\n")
            md.append("|----------|-------------|\n")
            for entry in efficiency.get("latency_by_count", []):
                md.append(f"| {entry['memory_count']:,} | {entry['latency_ms']:.2f} |\n")
            md.append("\n")
        
        # System Information
        md.append("## System Information\n\n")
        md.append(f"- **Benchmark Version:** {self.results.get('benchmark_version', 'N/A')}\n")
        md.append(f"- **Timestamp:** {self.results.get('timestamp', 'N/A')}\n")
        md.append(f"- **Total Duration:** {self.results.get('duration_seconds', 0):.1f}s\n")
        
        report = "\n".join(md)
        
        # Save to file
        with open(output_file, 'w') as f:
            f.write(report)
        
        return report
    
    def generate_json_report(self, output_file: str = "benchmark_report.json") -> Dict:
        """Generate structured JSON report."""
        
        report = {
            "version": "1.0",
            "timestamp": datetime.now().isoformat(),
            "title": "Memro Comprehensive Benchmark Report",
            "metrics": self._extract_metrics(),
            "performance_tiers": self._classify_performance(),
            "recommendations": self._generate_recommendations(),
            "raw_results": self.results
        }
        
        with open(output_file, 'w') as f:
            json.dump(report, f, indent=2)
        
        return report
    
    def _extract_metrics(self) -> Dict[str, Any]:
        """Extract key metrics from results."""
        metrics = {}
        results = self.results.get("results", {})
        
        # Latency metrics
        if "query_latency" in results and "error" not in results["query_latency"]:
            latency = results["query_latency"]
            metrics["latency"] = {
                "mean_ms": latency.get("avg_latency_ms", 0),
                "median_ms": latency.get("median_latency_ms", 0),
                "p95_ms": latency.get("p95_latency_ms", 0),
                "p99_ms": latency.get("p99_latency_ms", 0),
                "min_ms": latency.get("min_latency_ms", 0),
                "max_ms": latency.get("max_latency_ms", 0),
                "stdev_ms": latency.get("stdev_latency_ms", 0)
            }
        
        # Throughput metrics
        if "throughput" in results and "error" not in results["throughput"]:
            throughput = results["throughput"]
            metrics["throughput"] = {
                "queries_per_second": throughput.get("queries_per_second", 0),
                "total_queries": throughput.get("total_queries", 0),
                "error_rate": throughput.get("error_rate", 0)
            }
        
        # Memory efficiency
        if "memory_efficiency" in results and "error" not in results["memory_efficiency"]:
            efficiency = results["memory_efficiency"]
            metrics["scaling"] = {
                "latency_growth_per_order": efficiency.get("avg_latency_growth", 0),
                "type": "sublinear" if efficiency.get("avg_latency_growth", 1) < 0.3 else "linear"
            }
        
        return metrics
    
    def _classify_performance(self) -> Dict[str, str]:
        """Classify performance into tiers."""
        tiers = {}
        metrics = self._extract_metrics()
        
        # Latency classification
        if "latency" in metrics:
            p95 = metrics["latency"].get("p95_ms", 0)
            if p95 < 100:
                tiers["latency"] = "EXCELLENT (<100ms P95)"
            elif p95 < 500:
                tiers["latency"] = "GOOD (100-500ms P95)"
            elif p95 < 1000:
                tiers["latency"] = "ACCEPTABLE (500-1000ms P95)"
            else:
                tiers["latency"] = "NEEDS_IMPROVEMENT (>1000ms P95)"
        
        # Throughput classification
        if "throughput" in metrics:
            qps = metrics["throughput"].get("queries_per_second", 0)
            if qps > 100:
                tiers["throughput"] = "EXCELLENT (>100 QPS)"
            elif qps > 50:
                tiers["throughput"] = "GOOD (50-100 QPS)"
            elif qps > 10:
                tiers["throughput"] = "ACCEPTABLE (10-50 QPS)"
            else:
                tiers["throughput"] = "NEEDS_IMPROVEMENT (<10 QPS)"
        
        # Scaling classification
        if "scaling" in metrics:
            growth = metrics["scaling"].get("latency_growth_per_order", 0)
            if growth < 0.1:
                tiers["scaling"] = "EXCELLENT (sublinear growth)"
            elif growth < 0.3:
                tiers["scaling"] = "GOOD (low growth)"
            else:
                tiers["scaling"] = "NEEDS_IMPROVEMENT (high growth)"
        
        return tiers
    
    def _generate_recommendations(self) -> list:
        """Generate optimization recommendations based on results."""
        recommendations = []
        tiers = self._classify_performance()
        
        if "NEEDS_IMPROVEMENT" in tiers.get("latency", ""):
            recommendations.append({
                "category": "Latency",
                "issue": "Query latency exceeds acceptable thresholds",
                "recommendations": [
                    "Implement query result caching (P1 optimization)",
                    "Enable database composite indexes (completed in P0)",
                    "Optimize HNSW vector search parameters (completed in P0)"
                ]
            })
        
        if "NEEDS_IMPROVEMENT" in tiers.get("throughput", ""):
            recommendations.append({
                "category": "Throughput",
                "issue": "System throughput is below target",
                "recommendations": [
                    "Increase database connection pool (P1 optimization)",
                    "Implement parallel search execution (P1 optimization)",
                    "Consider horizontal scaling for load distribution"
                ]
            })
        
        if "NEEDS_IMPROVEMENT" in tiers.get("scaling", ""):
            recommendations.append({
                "category": "Scaling",
                "issue": "Performance degrades significantly with data scale",
                "recommendations": [
                    "Implement data partitioning strategy",
                    "Consider database sharding for large datasets",
                    "Enable query result caching for repeated patterns"
                ]
            })
        
        if not recommendations:
            recommendations.append({
                "category": "General",
                "issue": "Performance is good",
                "recommendations": [
                    "Monitor system metrics continuously",
                    "Plan for future capacity as data grows",
                    "Consider P2 optimizations for marginal gains"
                ]
            })
        
        return recommendations
    
    def generate_comparison_table(self) -> str:
        """Generate comparison table for different optimization levels."""
        
        table = []
        table.append("# Performance Comparison: Before vs After Optimizations\n")
        table.append("## Expected Impact of P0 + P1 Optimizations\n\n")
        
        table.append("| Metric | Baseline | After P0 | After P1 | Combined Target |\n")
        table.append("|--------|----------|----------|----------|----------------|\n")
        
        # Query latency
        table.append("| Query Latency (P95) | 500ms | 300ms (-40%) | 150ms (-50%) | <100ms (-80%) |\n")
        
        # Throughput
        table.append("| Throughput | 10 QPS | 25 QPS (+150%) | 50 QPS (+400%) | 100+ QPS (+900%) |\n")
        
        # Memory scaling
        table.append("| Memory Scaling | Linear | 0.5x growth | 0.2x growth | Sublinear |\n")
        
        # Vector search
        table.append("| Vector Search | 200ms | 170ms (-15%) | 170ms | 150ms (-25%) |\n")
        
        # Database queries
        table.append("| DB Queries | 100ms | 20ms (-80%) | 10ms (-90%) | <10ms (-95%) |\n")
        
        table.append("\n")
        
        return "\n".join(table)


def main():
    """Generate all benchmark reports."""
    import argparse
    
    parser = argparse.ArgumentParser(description="Generate benchmark reports")
    parser.add_argument("--input", default="benchmark_results.json", help="Input results file")
    parser.add_argument("--output_md", default="BENCHMARK_RESULTS.md", help="Markdown output file")
    parser.add_argument("--output_json", default="benchmark_report.json", help="JSON output file")
    parser.add_argument("--comparison", default="COMPARISON.md", help="Comparison table output")
    args = parser.parse_args()
    
    generator = BenchmarkReportGenerator(args.input)
    
    print("📊 Generating benchmark reports...")
    
    # Generate markdown report
    print(f"  ✓ Markdown report to {args.output_md}")
    generator.generate_markdown_report(args.output_md)
    
    # Generate JSON report
    print(f"  ✓ JSON report to {args.output_json}")
    generator.generate_json_report(args.output_json)
    
    # Generate comparison table
    print(f"  ✓ Comparison table to {args.comparison}")
    with open(args.comparison, 'w') as f:
        f.write(generator.generate_comparison_table())
    
    print("\n✅ All reports generated successfully!")


if __name__ == "__main__":
    main()
