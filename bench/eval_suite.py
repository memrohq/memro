#!/usr/bin/env python3
"""
Memro Measurable Verification Evaluation Suite
Runs measurable evaluations for:
1. Contradictory Facts (superseding / newer assertion preference)
2. Stale vs. Updated Facts (temporal freshness & evolution)
3. Identity Boundaries (Agent A cannot read Agent B's memories)
4. Distractor / Wrong Retrieval Resistance (negative query rejection & precision)

Usage:
  python bench/eval_suite.py [--base-url http://localhost:8081]
"""

import os
import sys
import json
import time
from dataclasses import dataclass, asdict
from typing import List, Dict, Any, Optional

from memro import Agent, MemroClient

@dataclass
class EvalResult:
    suite: str
    case_name: str
    passed: bool
    expected: str
    actual: str
    latency_ms: float
    details: Optional[str] = None

class MeasurableEvalRunner:
    def __init__(self, base_url: str):
        self.base_url = base_url
        self.client = MemroClient(base_url=base_url)
        self.results: List[EvalResult] = []

    def run_all(self) -> Dict[str, Any]:
        print(f"=== Starting Memro Measurable Evaluation Suite on {self.base_url} ===\n")
        
        self.eval_identity_boundaries()
        self.eval_contradictory_facts()
        self.eval_stale_updated_facts()
        self.eval_distractor_resistance()

        summary = self.compute_summary()
        self.print_report(summary)
        return summary

    def eval_identity_boundaries(self):
        """Test: Agent A cannot read Agent B's private memories under any circumstance."""
        print("Running Suite 1: Identity Boundaries (Multi-Tenant Isolation)...")
        start = time.perf_counter()
        
        # Create two distinct sovereign agents
        agent_a = Agent.create(client=self.client)
        agent_b = Agent.create(client=self.client)
        
        # Agent A stores a secret fact
        secret_fact = f"Confidential audit token for Agent A: sec_tok_a_{int(time.time())}"
        agent_a.remember(secret_fact, visibility="private", type="semantic")
        
        # Agent A verifies it can recall its own secret
        res_a = agent_a.recall("Confidential audit token for Agent A", limit=5)
        can_a_read = any("sec_tok_a_" in getattr(r, "content", "") for r in res_a)
        
        # Agent B attempts to recall Agent A's secret
        res_b = agent_b.recall("Confidential audit token for Agent A", limit=5)
        can_b_read = any("sec_tok_a_" in getattr(r, "content", "") for r in res_b)
        
        elapsed = (time.perf_counter() - start) * 1000
        
        passed = can_a_read and not can_b_read
        self.results.append(EvalResult(
            suite="Identity Boundaries",
            case_name="Cross-Agent Private Isolation",
            passed=passed,
            expected="Agent A can recall secret; Agent B receives 0 results",
            actual=f"Agent A found: {len(res_a)}, Agent B found: {len(res_b)}",
            latency_ms=round(elapsed, 2),
            details="Cryptographic identity separation between distinct Ed25519 public keys."
        ))

    def eval_contradictory_facts(self):
        """Test: When contradictory facts are saved sequentially, newer assertion is prioritized."""
        print("Running Suite 2: Contradictory Facts Resolution...")
        start = time.perf_counter()
        agent = Agent.create(client=self.client)

        fact_old = "The project deployment branch is set to release-v1"
        fact_new = "The project deployment branch is updated to release-v2"

        agent.remember(fact_old, type="semantic")
        time.sleep(0.5)
        agent.remember(fact_new, type="semantic")

        res = agent.recall("project deployment branch", limit=3)
        elapsed = (time.perf_counter() - start) * 1000

        top_content = getattr(res[0], "content", "") if res else ""
        passed = "release-v2" in top_content
        
        self.results.append(EvalResult(
            suite="Contradictory Facts",
            case_name="Branch Setting Update",
            passed=passed,
            expected="Top result contains 'release-v2'",
            actual=f"Top result: '{top_content}'",
            latency_ms=round(elapsed, 2),
            details="Reciprocal Rank Fusion with temporal decay ranks the latest state highest."
        ))

    def eval_stale_updated_facts(self):
        """Test: Stale fact supersession and temporal freshness."""
        print("Running Suite 3: Stale vs. Updated Facts...")
        start = time.perf_counter()
        agent = Agent.create(client=self.client)

        # Ingest original configuration
        agent.remember("Staging database cluster endpoint is postgres-legacy.internal:5432", type="procedural")
        time.sleep(0.5)
        # Ingest updated configuration
        agent.remember("Staging database cluster migrated to postgres-aurora-v2.internal:5432", type="procedural")

        res = agent.recall("staging database cluster endpoint", limit=2)
        elapsed = (time.perf_counter() - start) * 1000

        top_content = getattr(res[0], "content", "") if res else ""
        passed = "postgres-aurora-v2" in top_content

        self.results.append(EvalResult(
            suite="Stale/Updated Facts",
            case_name="Database Endpoint Migration",
            passed=passed,
            expected="Top result points to postgres-aurora-v2",
            actual=f"Top result: '{top_content}'",
            latency_ms=round(elapsed, 2),
            details="Verifies newer procedural memory outranks legacy procedural record."
        ))

    def eval_distractor_resistance(self):
        """Test: Distractor resistance and out-of-domain negative query rejection."""
        print("Running Suite 4: Distractor Rejection & Precision...")
        start = time.perf_counter()
        agent = Agent.create(client=self.client)

        # Distractor memories
        distractors = [
            "Office pantry coffee machine is descaled on the 1st of every month",
            "Visitor badges must be returned to reception by 6 PM daily",
            "Annual team retreat scheduled for October in Lake Tahoe",
            "Conference room Delta projector uses HDMI input 2",
        ]
        for d in distractors:
            agent.remember(d, type="semantic")

        # Specific target
        target = "Production release freeze window starts December 15th at 18:00 UTC."
        agent.remember(target, type="semantic")

        # 1. Target retrieval precision
        res_target = agent.recall("production release freeze window date and time", limit=3)
        target_found = any("December 15th" in getattr(r, "content", "") for r in res_target)
        top_is_target = "December 15th" in getattr(res_target[0], "content", "") if res_target else False

        # 2. Irrelevant negative query (min_score threshold rejection)
        res_irrelevant = agent.recall("quantum cryptography entanglement simulation algorithms", limit=5, min_score=0.4)
        irrelevant_empty = len(res_irrelevant) == 0

        elapsed = (time.perf_counter() - start) * 1000
        passed = top_is_target and irrelevant_empty

        self.results.append(EvalResult(
            suite="Distractor Rejection",
            case_name="Precision & Out-of-Domain Threshold Rejection",
            passed=passed,
            expected="Target memory is #1 rank; unrelated query returns 0 results at min_score 0.4",
            actual=f"Target #1: {top_is_target}, Irrelevant count: {len(res_irrelevant)}",
            latency_ms=round(elapsed, 2),
            details="Tests dense/sparse RRF fusion score cutoff against unrelated corpus."
        ))

    def compute_summary(self) -> Dict[str, Any]:
        total = len(self.results)
        passed = sum(1 for r in self.results if r.passed)
        failed = total - passed
        pass_rate = (passed / total * 100) if total > 0 else 0.0

        return {
            "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "base_url": self.base_url,
            "total_tests": total,
            "passed": passed,
            "failed": failed,
            "pass_rate_pct": round(pass_rate, 1),
            "results": [asdict(r) for r in self.results],
        }

    def print_report(self, summary: Dict[str, Any]):
        print("\n" + "=" * 70)
        print("MEMRO MEASURABLE VERIFICATION REPORT")
        print("=" * 70)
        print(f"Target:    {summary['base_url']}")
        print(f"Date:      {summary['timestamp']}")
        print(f"Pass Rate: {summary['passed']}/{summary['total_tests']} ({summary['pass_rate_pct']}%)")
        print("-" * 70)
        
        for r in self.results:
            status = "✓ PASS" if r.passed else "✗ FAIL"
            print(f"[{status}] {r.suite} :: {r.case_name} ({r.latency_ms} ms)")
            print(f"  Expected: {r.expected}")
            print(f"  Actual:   {r.actual}")
            if r.details:
                print(f"  Notes:    {r.details}")
            print("")
        print("=" * 70)

def main():
    base_url = os.environ.get("MEMRO_BASE_URL", "https://api.memro.co")
    if len(sys.argv) > 1 and sys.argv[1].startswith("http"):
        base_url = sys.argv[1]

    runner = MeasurableEvalRunner(base_url)
    summary = runner.run_all()

    # Save artifact
    output_path = os.path.join(os.path.dirname(__file__), "measurable_eval_results.json")
    with open(output_path, "w") as f:
        json.dump(summary, f, indent=2)
    print(f"\nArtifact saved to: {output_path}")

    if summary["failed"] > 0:
        sys.exit(1)

if __name__ == "__main__":
    main()
