#!/usr/bin/env python3
"""
Memro Benchmark Runner - Complete Orchestration
Runs benchmarks, generates reports, and publishes results.
"""

import subprocess
import sys
import json
from pathlib import Path
from datetime import datetime


class BenchmarkRunner:
    """Orchestrate benchmark execution."""
    
    def __init__(self):
        self.benchmark_dir = Path(__file__).parent
        self.results_file = self.benchmark_dir / "benchmark_results.json"
        self.markdown_report = self.benchmark_dir / "BENCHMARK_RESULTS.md"
        self.json_report = self.benchmark_dir / "benchmark_report.json"
        self.comparison_file = self.benchmark_dir / "OPTIMIZATION_COMPARISON.md"
    
    def run_benchmarks(self, base_url: str = "http://localhost:8081", quick: bool = False, agent_id: str = None, private_key: str = None):
        """Run comprehensive benchmarks."""
        print("\n" + "="*70)
        print("🚀 Starting Memro Benchmark Suite")
        print("="*70)
        print(f"Base URL: {base_url}")
        print(f"Mode: {'QUICK' if quick else 'FULL'}")
        if agent_id:
            print(f"Agent ID: {agent_id[:16]}...")
        print(f"Start: {datetime.now().isoformat()}")
        
        cmd = [
            sys.executable,
            str(self.benchmark_dir / "comprehensive_benchmark.py"),
            f"--base_url={base_url}",
            f"--output={self.results_file}",
        ]
        
        if quick:
            cmd.append("--quick")
        
        if agent_id:
            cmd.append(f"--agent_id={agent_id}")
        
        if private_key:
            cmd.append(f"--private_key={private_key}")
        
        try:
            result = subprocess.run(cmd, check=True)
            return True
        except subprocess.CalledProcessError as e:
            print(f"❌ Benchmark execution failed: {e}")
            return False
    
    def generate_reports(self):
        """Generate all benchmark reports."""
        print("\n" + "="*70)
        print("📊 Generating Benchmark Reports")
        print("="*70)
        
        if not self.results_file.exists():
            print(f"❌ Results file not found: {self.results_file}")
            return False
        
        cmd = [
            sys.executable,
            str(self.benchmark_dir / "benchmark_report_generator.py"),
            f"--input={self.results_file}",
            f"--output_md={self.markdown_report}",
            f"--output_json={self.json_report}",
            f"--comparison={self.comparison_file}",
        ]
        
        try:
            result = subprocess.run(cmd, check=True)
            return True
        except subprocess.CalledProcessError as e:
            print(f"❌ Report generation failed: {e}")
            return False
    
    def print_summary(self):
        """Print benchmark summary."""
        if not self.json_report.exists():
            print("WARNING: Report file not found")
            return
        
        with open(self.json_report, 'r') as f:
            report = json.load(f)
        
        print("\n" + "="*70)
        print("📈 BENCHMARK SUMMARY")
        print("="*70)
        
        metrics = report.get("metrics", {})
        tiers = report.get("performance_tiers", {})
        
        print("\n🎯 Performance Metrics:")
        if "latency" in metrics:
            lat = metrics["latency"]
            print(f"  Query Latency (P95):  {lat.get('p95_ms', 'N/A'):.2f}ms")
            print(f"  Query Latency (P99):  {lat.get('p99_ms', 'N/A'):.2f}ms")
            print(f"  Query Latency (Mean): {lat.get('mean_ms', 'N/A'):.2f}ms")
        
        if "throughput" in metrics:
            tp = metrics["throughput"]
            print(f"  Throughput:           {tp.get('queries_per_second', 'N/A'):.2f} QPS")
            print(f"  Error Rate:           {tp.get('error_rate', 0)*100:.2f}%")
        
        print("\n📊 Performance Tiers:")
        for category, tier in tiers.items():
            print(f"  {category.capitalize():20s}: {tier}")
        
        print("\n💡 Recommendations:")
        recommendations = report.get("recommendations", [])
        for rec in recommendations[:3]:  # Show top 3 recommendations
            print(f"  • {rec.get('issue', 'N/A')}")
            for item in rec.get("recommendations", [])[:2]:
                print(f"    - {item}")
        
        print("\n" + "="*70)
        print(f"📁 Full Reports:")
        print(f"  • Markdown:  {self.markdown_report}")
        print(f"  • JSON:      {self.json_report}")
        print(f"  • Comparison: {self.comparison_file}")
        print("="*70 + "\n")
    
    def run_all(self, base_url: str = "http://localhost:8081", quick: bool = False, agent_id: str = None, private_key: str = None):
        """Run complete benchmark suite."""
        success = True
        
        # Run benchmarks
        if not self.run_benchmarks(base_url, quick, agent_id, private_key):
            success = False
        
        # Generate reports
        if not self.generate_reports():
            success = False
        
        # Print summary
        if success:
            self.print_summary()
        
        return success


def main():
    """Main entry point."""
    import argparse
    
    parser = argparse.ArgumentParser(description="Run Memro benchmark suite")
    parser.add_argument("--base_url", default="http://localhost:8081", help="Base URL for API")
    parser.add_argument("--quick", action="store_true", help="Skip throughput test")
    parser.add_argument("--agent_id", default=None, help="Agent ID to benchmark (if not provided, creates new agent)")
    parser.add_argument("--private_key", default=None, help="Private key for agent authentication")
    args = parser.parse_args()
    
    runner = BenchmarkRunner()
    success = runner.run_all(args.base_url, args.quick, args.agent_id, args.private_key)
    
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
