#!/usr/bin/env python3
"""
Benchmark Existing Memro Data
Evaluates recall performance against data already present in the database.
"""

import json
import time
import os
import sys
import argparse
from typing import List, Dict, Any, Optional
import numpy as np
from pathlib import Path

try:
    from memro import Agent, MemroClient
except ImportError:
    print("❌ memro SDK not found. Install with: pip install -e memro-sdk-python")
    sys.exit(1)

class ExistingDataEvaluator:
    def __init__(self, base_url: str = "http://localhost:8081", agent_id: str = None, openai_key: str = None):
        self.base_url = base_url.rstrip("/")
        self.client = MemroClient(base_url=self.base_url)
        self.agent_id = agent_id
        self.agent = None
        if self.agent_id:
            print(f"🔑 Creating temporary agent for authentication...")
            # Create a real agent identity to get a valid agent_id and private_key for signing
            # This identity will be used in the X-Agent-Id and X-Signature headers
            temp_agent = Agent.create(client=self.client)
            self.auth_agent_id = temp_agent.agent_id
            self.auth_private_key = temp_agent._private_key
            print(f"✅ Auth agent created: {self.auth_agent_id}")
        else:
            self.auth_agent_id = None
            self.auth_private_key = None
        
        self.openai_key = openai_key or os.getenv("OPENAI_API_KEY")
        if self.openai_key:
            import openai
            self.openai_client = openai.OpenAI(api_key=self.openai_key)
        else:
            self.openai_client = None
            print("⚠️  No OpenAI API key found - LLM scoring disabled")

        self.results = []

    def load_dataset(self, dataset_path: str) -> List[Dict]:
        """Load the LongMemEval_S dataset."""
        print(f"📖 Loading dataset from {dataset_path}...")
        with open(dataset_path, 'r') as f:
            return json.load(f)

    def evaluate_recall_simple(self, answer: str, recall_results: List[Any]) -> float:
        """Simple string matching score."""
        recall_text = " ".join([r.content for r in recall_results]).lower()
        return 1.0 if answer.lower() in recall_text else 0.0

    def evaluate_recall_llm(self, question: str, answer: str, recall_results: List[Any]) -> Dict[str, Any]:
        """GPT-4o evaluation."""
        if not self.openai_client:
            score = self.evaluate_recall_simple(answer, recall_results)
            return {"score": score, "reasoning": "String match", "method": "string_match"}

        recall_texts = [r.content for r in recall_results[:5]]
        context = f"""
Question: {question}
Expected Answer: {answer}

Recall Results:
{chr(10).join(f"{i+1}. {text}" for i, text in enumerate(recall_texts))}

Does the recall text contain the answer? Score 0.0 to 1.0. Just return the score followed by reasoning.
"""
        try:
            response = self.openai_client.chat.completions.create(
                model="gpt-4o",
                messages=[{"role": "user", "content": context}],
                temperature=0.0
            )
            content = response.choices[0].message.content.strip()
            # Crude parsing of score
            import re
            score_match = re.search(r'(\d+\.?\d*)', content)
            score = float(score_match.group(1)) if score_match else 0.0
            return {"score": min(max(score, 0.0), 1.0), "reasoning": content, "method": "llm_judge"}
        except Exception as e:
            print(f"   ❌ LLM Judge failed: {e}")
            score = self.evaluate_recall_simple(answer, recall_results)
            return {"score": score, "reasoning": "Fallback to string match", "method": "fallback"}

    def run(self, dataset_path: str, limit: int = None):
        if not self.agent_id:
            print("❌ No agent ID configured.")
            return

        dataset = self.load_dataset(dataset_path)
        if limit:
            dataset = dataset[:limit]
        
        print(f"🚀 Benchmarking {len(dataset)} items against Agent {self.agent_id}")
        
        total_score = 0
        latencies = []
        
        for i, item in enumerate(dataset):
            question = item["question"]
            answer = item["answer"]
            
            start = time.time()
            try:
                # Use requests directly to handle the authentication headers with the auth agent
                # while querying for the target agent_id
                params = {
                    "agent_id": self.agent_id,
                    "query": question,
                    "limit": 10,
                    "mode": "hybrid"
                }
                
                url = f"{self.base_url}/search"
                
                # Use MemroClient's signing logic
                from memro.crypto import sign_body
                import requests
                
                # Sign empty body for GET request
                headers = sign_body(self.auth_private_key, b"")
                headers["X-Agent-Id"] = self.auth_agent_id
                headers["Content-Type"] = "application/json"
                
                response = requests.get(url, params=params, headers=headers)
                
                if response.status_code != 200:
                    results.append({
                        "question_id": record.get("question_id"),
                        "success": False,
                        "error": f"[{response.status_code}] {response.text}"
                    })
                    print(f"[{i+1}/{total}] ❌ Error: [{response.status_code}] {response.text}")
                    continue
                
                raw_results = response.json().get("results", [])
                
                # Convert raw dicts to mock objects with content/score attributes for the evaluator
                class MockResult:
                    def __init__(self, content, score):
                        self.content = content
                        self.score = score
                
                recalls = [MockResult(r["content"], r["score"]) for r in raw_results]
                latency = (time.time() - start) * 1000
                latencies.append(latency)
                
                eval_res = self.evaluate_recall_llm(question, answer, recalls)
                score = eval_res["score"]
                total_score += score
                
                print(f"[{i+1}/{len(dataset)}] Score: {score:.2f} | Latency: {latency:.1f}ms | Q: {question[:50]}...")
                
                self.results.append({
                    "question_id": item.get("question_id"),
                    "question": question,
                    "answer": answer,
                    "score": score,
                    "latency_ms": latency,
                    "reasoning": eval_res.get("reasoning"),
                    "method": eval_res.get("method")
                })
            except Exception as e:
                print(f"[{i+1}/{len(dataset)}] ❌ Error: {e}")
                self.results.append({"question": question, "error": str(e), "success": False})

        # Summary
        if latencies:
            print("\n" + "="*50)
            print("📊 BENCHMARK SUMMARY")
            print("="*50)
            print(f"Total Items:    {len(dataset)}")
            print(f"Average Score:  {total_score/len(dataset):.3f}")
            print(f"Mean Latency:   {np.mean(latencies):.2f}ms")
            print(f"P95 Latency:    {np.percentile(latencies, 95):.2f}ms")
            print(f"P99 Latency:    {np.percentile(latencies, 99):.2f}ms")
            
            output_file = "existing_data_benchmark_results.json"
            with open(output_file, 'w') as f:
                json.dump({
                    "summary": {
                        "total_items": len(dataset),
                        "avg_score": total_score/len(dataset),
                        "mean_latency_ms": np.mean(latencies),
                        "p95_latency_ms": np.percentile(latencies, 95),
                        "p99_latency_ms": np.percentile(latencies, 99)
                    },
                    "results": self.results
                }, f, indent=2)
            print(f"\n✅ Results saved to {output_file}")

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--agent_id", default="aa94d130d9c0ab2ac2c72d95779ec582e97057706e7d1d2f9b4b8a3d39fe3ac5")
    parser.add_argument("--dataset", default="data/eval/longmemeval_s.json")
    parser.add_argument("--limit", type=int, default=None)
    parser.add_argument("--base_url", default="http://localhost:8081")
    args = parser.parse_args()
    
    evaluator = ExistingDataEvaluator(base_url=args.base_url, agent_id=args.agent_id)
    evaluator.run(args.dataset, args.limit)

if __name__ == "__main__":
    main()
