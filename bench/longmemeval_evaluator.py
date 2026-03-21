#!/usr/bin/env python3
"""
Proper LongMemEval_S Evaluation Script
Tests recall performance against the full 500-question dataset using LLM-as-a-judge scoring.
"""

import json
import time
import os
from typing import List, Dict, Any, Optional
import openai
from memro import Agent, MemroClient

class LongMemEvalEvaluator:
    def __init__(self, base_url: str = "http://localhost:8081", openai_key: str = None):
        self.client = MemroClient(base_url)
        self.openai_key = openai_key or os.getenv("OPENAI_API_KEY")
        if self.openai_key:
            self.openai_client = openai.OpenAI(api_key=self.openai_key)
        else:
            self.openai_client = None
            print("⚠️  No OpenAI API key found - LLM scoring disabled")

        self.results = []

    def load_dataset(self, dataset_path: str) -> List[Dict]:
        """Load the LongMemEval_S dataset."""
        with open(dataset_path, 'r') as f:
            return json.load(f)

    def find_agent_for_question(self, question: Dict) -> Optional[Agent]:
        """
        Find the appropriate agent for a question.
        Since LongMemEval creates one agent per batch, we need to determine which agent has the relevant memories.
        """
        # For now, try the first agent - in a real implementation we'd need to track agent-to-session mapping
        # This is a simplified approach for the benchmark
        try:
            # Try to find an existing agent that might have the memories
            # Since we don't have direct agent lookup, we'll create a test agent and see if it has memories
            test_agent = Agent.create(client=self.client)

            # Quick test recall to see if this agent has relevant memories
            test_results = test_agent.recall("test query", limit=1)
            if len(test_results) > 0:
                return test_agent

        except Exception as e:
            print(f"   ❌ Agent creation failed: {e}")

        return None

    def evaluate_recall_with_llm_judge(self, question: str, answer: str, recall_results: List[Any]) -> Dict[str, Any]:
        """
        Use GPT-4o as LLM judge to evaluate if recall results contain the correct answer.
        """
        if not self.openai_client:
            # Fallback to simple string matching if no OpenAI key
            recall_text = " ".join([r.content for r in recall_results])
            score = 1.0 if answer.lower() in recall_text.lower() else 0.0
            return {
                "score": score,
                "reasoning": "Simple string matching (no LLM judge available)",
                "method": "string_match"
            }

        # Prepare context for LLM judge
        recall_texts = [r.content for r in recall_results[:5]]  # Top 5 results
        context = f"""
Question: {question}
Expected Answer: {answer}

Recall Results:
{chr(10).join(f"{i+1}. {text}" for i, text in enumerate(recall_texts))}

Please evaluate if the recall results contain information that correctly answers the question.
Score from 0.0 to 1.0, where:
- 1.0 = Perfect answer, directly addresses the question
- 0.5 = Partial answer, contains relevant information but incomplete
- 0.0 = No relevant information found

Provide a brief reasoning for your score.
"""

        try:
            response = self.openai_client.chat.completions.create(
                model="gpt-4o",
                messages=[
                    {"role": "system", "content": "You are an expert evaluator for memory recall systems. Score answers based on accuracy and relevance."},
                    {"role": "user", "content": context}
                ],
                temperature=0.1,
                max_tokens=200
            )

            content = response.choices[0].message.content.strip()

            # Parse score and reasoning
            lines = content.split('\n')
            score_line = next((line for line in lines if 'score' in line.lower() or line.strip().replace('.', '').isdigit()), "0.0")
            score = float(''.join(c for c in score_line if c.isdigit() or c == '.'))

            return {
                "score": min(max(score, 0.0), 1.0),  # Clamp to 0-1
                "reasoning": content,
                "method": "llm_judge_gpt4o"
            }

        except Exception as e:
            print(f"   ❌ LLM judge failed: {e}")
            # Fallback to string matching
            recall_text = " ".join([r.content for r in recall_results])
            score = 1.0 if answer.lower() in recall_text.lower() else 0.0
            return {
                "score": score,
                "reasoning": f"LLM judge failed ({e}), fell back to string matching",
                "method": "string_match_fallback"
            }

    def evaluate_question(self, question_data: Dict) -> Dict[str, Any]:
        """Evaluate a single question from the dataset."""
        question_id = question_data["question_id"]
        question_type = question_data["question_type"]
        question = question_data["question"]
        expected_answer = question_data["answer"]

        print(f"🔍 Evaluating Q{question_id} ({question_type}): {question[:50]}...")

        # Find appropriate agent
        agent = self.find_agent_for_question(question_data)
        if not agent:
            return {
                "question_id": question_id,
                "question_type": question_type,
                "question": question,
                "expected_answer": expected_answer,
                "success": False,
                "error": "No suitable agent found",
                "recall_results": [],
                "score": 0.0,
                "latency_ms": 0
            }

        # Perform recall
        start_time = time.time()
        try:
            recall_results = agent.recall(question, limit=10, mode="hybrid")
            latency = (time.time() - start_time) * 1000

            # Evaluate with LLM judge
            evaluation = self.evaluate_recall_with_llm_judge(question, expected_answer, recall_results)

            return {
                "question_id": question_id,
                "question_type": question_type,
                "question": question,
                "expected_answer": expected_answer,
                "success": True,
                "recall_count": len(recall_results),
                "recall_results": [{"content": r.content[:200], "score": r.score} for r in recall_results[:3]],
                "score": evaluation["score"],
                "evaluation_method": evaluation["method"],
                "reasoning": evaluation["reasoning"],
                "latency_ms": latency
            }

        except Exception as e:
            latency = (time.time() - start_time) * 1000
            return {
                "question_id": question_id,
                "question_type": question_type,
                "question": question,
                "expected_answer": expected_answer,
                "success": False,
                "error": str(e),
                "recall_results": [],
                "score": 0.0,
                "latency_ms": latency
            }

    def run_evaluation(self, dataset_path: str, limit: int = None):
        """Run evaluation on the dataset."""
        print("🚀 Starting LongMemEval_S Evaluation")
        print("=" * 60)

        # Load dataset
        dataset = self.load_dataset(dataset_path)
        if limit:
            dataset = dataset[:limit]

        print(f"📊 Dataset: {len(dataset)} questions")
        print(f"🎯 Question Types: {set(q['question_type'] for q in dataset)}")
        print(f"🤖 LLM Judge: {'✅ GPT-4o' if self.openai_client else '❌ Disabled (string matching only)'}")
        print()

        # Evaluate each question
        successful_evaluations = 0
        total_score = 0
        latencies = []

        for i, question in enumerate(dataset, 1):
            print(f"[{i:3d}/{len(dataset)}] ", end="")
            result = self.evaluate_question(question)
            self.results.append(result)

            if result["success"]:
                successful_evaluations += 1
                total_score += result["score"]
                latencies.append(result["latency_ms"])

                status = "✅" if result["score"] >= 0.5 else "❌"
                print(f"{status} Score: {result['score']:.2f} | {result['latency_ms']:.0f}ms")
            else:
                print(f"❌ Failed: {result.get('error', 'Unknown error')}")

        # Final results
        print("\n" + "=" * 60)
        print("📈 LONGMEMEVAL_S RESULTS")
        print("=" * 60)

        if successful_evaluations > 0:
            avg_score = total_score / successful_evaluations
            avg_latency = sum(latencies) / len(latencies)

            print(f"✅ Successful Evaluations: {successful_evaluations}/{len(dataset)} ({successful_evaluations/len(dataset)*100:.1f}%)")
            print(f"🎯 Average Score: {avg_score:.3f}")
            print(f"⚡ Average Latency: {avg_latency:.1f}ms")

            # Breakdown by question type
            type_results = {}
            for result in self.results:
                if result["success"]:
                    qtype = result["question_type"]
                    if qtype not in type_results:
                        type_results[qtype] = []
                    type_results[qtype].append(result["score"])

            print(f"\n📊 Performance by Question Type:")
            for qtype, scores in type_results.items():
                avg_type_score = sum(scores) / len(scores)
                print(f"   • {qtype}: {avg_type_score:.3f} ({len(scores)} questions)")

        else:
            print("❌ No successful evaluations")

        # Save detailed results
        output_file = "longmemeval_s_results.json"
        with open(output_file, "w") as f:
            json.dump(self.results, f, indent=2, default=str)

        print(f"\n💾 Detailed results saved to {output_file}")

        return successful_evaluations > 0

def main():
    import sys

    dataset_path = "data/eval/longmemeval_s.json"
    limit = None

    if len(sys.argv) > 1:
        limit = int(sys.argv[1])

    evaluator = LongMemEvalEvaluator()
    success = evaluator.run_evaluation(dataset_path, limit)

    if success:
        print("\n🎉 Evaluation completed successfully!")
    else:
        print("\n⚠️  Evaluation completed with issues.")

if __name__ == "__main__":
    main()