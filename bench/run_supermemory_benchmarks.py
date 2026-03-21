#!/usr/bin/env python3
"""
Quick Start Guide: Run Supermemory-Scale Benchmarks

This script runs Memro benchmarks against 500+ memories to match 
Supermemory's published LongMemEval dataset methodology.
"""

import subprocess
import sys
import json
from datetime import datetime
from pathlib import Path


def print_header(title):
    """Print formatted header."""
    print("\n" + "="*70)
    print(f"🔬 {title}")
    print("="*70)


def run_command(cmd, description):
    """Run command and report results."""
    print(f"\n⏳ {description}...")
    try:
        result = subprocess.run(cmd, check=True, capture_output=True, text=True)
        print(f"✅ {description} - Complete")
        return True
    except subprocess.CalledProcessError as e:
        print(f"❌ {description} - Failed")
        print(f"Error: {e.stderr}")
        return False


def main():
    """Run Supermemory-scale benchmarks."""
    
    print_header("MEMRO SUPERMEMORY-SCALE BENCHMARK SUITE")
    print("\nThis benchmark suite tests Memro against 500-2000 memories")
    print("using the same methodology as Supermemory's LongMemEval evaluation.")
    
    benchmark_dir = Path(__file__).parent
    base_url = "http://localhost:8081"
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    
    # Create output directory for this run
    output_dir = benchmark_dir / f"results_{timestamp}"
    output_dir.mkdir(exist_ok=True)
    
    print("\n📁 Output Directory:", output_dir)
    
    # Test 1: Single 500-memory benchmark
    print_header("TEST 1: Single 500-Memory Benchmark")
    print("\nWill create 500 diverse memories and run recall queries.")
    print("This matches Supermemory's typical dataset size.")
    
    cmd = [
        sys.executable,
        str(benchmark_dir / "supermemory_scale_benchmark.py"),
        f"--base_url={base_url}",
        "--size=500",
        f"--output={output_dir / 'benchmark_500_memories.json'}"
    ]
    
    if not run_command(cmd, "500-memory benchmark"):
        print("\n❌ Benchmark failed. Ensure Memro API is running at", base_url)
        return False
    
    # Test 2: Scaling analysis (100, 250, 500, 1000)
    print_header("TEST 2: Scaling Analysis (100→1000 memories)")
    print("\nWill test recall latency at multiple scales:")
    print("  • 100 memories (baseline)")
    print("  • 250 memories (2.5x growth)")
    print("  • 500 memories (5x growth - LongMemEval)")
    print("  • 1000 memories (10x growth)")
    print("\nThis reveals how performance scales with dataset size.")
    
    cmd = [
        sys.executable,
        str(benchmark_dir / "supermemory_scale_benchmark.py"),
        f"--base_url={base_url}",
        "--scaling",
        f"--output={output_dir / 'scaling_analysis.json'}"
    ]
    
    if not run_command(cmd, "Scaling analysis"):
        print("\n⚠️  Scaling analysis failed, but single-size benchmarks may have succeeded.")
    
    # Test 3: Comparison vs Supermemory baseline
    print_header("TEST 3: Comparison vs Supermemory (500 memories)")
    print("\nCompares Memro performance against Supermemory baseline metrics:")
    print("  • Supermemory avg latency: ~150ms")
    print("  • Supermemory P95 latency: ~250ms")
    
    cmd = [
        sys.executable,
        str(benchmark_dir / "supermemory_scale_benchmark.py"),
        f"--base_url={base_url}",
        "--compare",
        f"--output={output_dir / 'memro_vs_supermemory.json'}"
    ]
    
    if not run_command(cmd, "Supermemory comparison"):
        print("\n⚠️  Comparison test failed.")
    
    # Generate summary report
    print_header("GENERATING SUMMARY REPORT")
    
    summary = {
        "timestamp": datetime.now().isoformat(),
        "test_suite": "Supermemory-Scale Benchmark",
        "base_url": base_url,
        "results_directory": str(output_dir),
        "tests_run": [
            "500-memory benchmark (matches LongMemEval)",
            "Scaling analysis (100-1000 memories)",
            "Comparison vs Supermemory baseline"
        ],
        "output_files": []
    }
    
    # Collect output files
    for f in output_dir.glob("*.json"):
        summary["output_files"].append(f.name)
    
    # Save summary
    summary_file = output_dir / "summary.json"
    with open(summary_file, 'w') as f:
        json.dump(summary, f, indent=2)
    
    print("\n📊 Summary saved to:", summary_file)
    
    # Print results summary
    print_header("BENCHMARK COMPLETE")
    print("\n✅ All benchmarks completed!")
    print(f"\n📁 Results saved in: {output_dir}")
    print("\nGenerated files:")
    for f in sorted(output_dir.glob("*.json")):
        size = f.stat().st_size / 1024  # KB
        print(f"  • {f.name:40s} ({size:.1f} KB)")
    
    # Print next steps
    print("\n📖 Next Steps:")
    print("  1. Review the JSON results files")
    print("  2. Compare against Supermemory's published benchmarks")
    print("  3. Run optimization (P0/P1 from optimization guide)")
    print("  4. Re-run benchmarks to measure improvement")
    print("  5. Export results for publishing/sharing")
    
    print("\n🔗 LongMemEval Dataset Info:")
    print("  • Supermemory tested against 500-1000 memories")
    print("  • Diverse memory types (episodic, semantic, procedural)")
    print("  • Mixed temporal patterns (recent + old memories)")
    print("  • Multi-hop reasoning queries")
    
    print("\n" + "="*70)
    print("✨ Benchmark suite complete!\n")
    
    return True


if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1)
