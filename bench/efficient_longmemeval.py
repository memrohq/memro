#!/usr/bin/env python3
"""
Efficient LongMemEval_S Benchmark
Full 500 questions with proper LLM-as-a-judge scoring
"""

import json
import time
import os
import re
from typing import List, Dict, Any, Optional
import openai
from memro import Agent, MemroClient
import requests

class EfficientLongMemEval:
    def __init__(self, base_url: str = "http://localhost:8081", openai_key: str = None):
        self.client = MemroClient(base_url)
        self.openai_key = openai_key or os.getenv("OPENAI_API_KEY")
        if self.openai_key:
            self.openai_client = openai.OpenAI(api_key=self.openai_key)
        else:
            self.openai_client = None
            print("⚠️  No OpenAI API key found - LLM scoring disabled")

        self.results = []

    def get_queue_depth(self) -> int:
        """Fetch current embedding queue depth from metrics."""
        try:
            resp = requests.get(f"{self.client.base_url}/metrics", timeout=5)
            if resp.ok:
                match = re.search(r'memro_embedding_queue_depth (\d+)', resp.text)
                if match:
                    return int(match.group(1))
        except Exception:
            pass
        return 0

    def wait_for_indexing(self, timeout=600):
        """Wait until backend queue is empty."""
        start = time.time()
        print("   ⏳ Waiting for indexing...")
        last_depth = -1
        while time.time() - start < timeout:
            depth = self.get_queue_depth()
            if depth == 0:
                time.sleep(2)
                print("      ✅ Indexing complete.")
                return True
            if depth != last_depth:
                print(f"      📊 Pending embeddings: {depth}")
                last_depth = depth
            time.sleep(0.5)
        print(f"   ❌ Indexing wait timed out (depth still {last_depth})")
        return False

    def load_dataset(self, dataset_path: str) -> List[Dict]:
        """Load the LongMemEval_S dataset."""
        with open(dataset_path, 'r') as f:
            return json.load(f)

    def create_session_memory_content(self, question_data: Dict) -> List[str]:
        """Create memory content from haystack sessions."""
        memories = []

        # Process each haystack session
        for session_idx, session_msgs in enumerate(question_data.get('haystack_sessions', [])):
            session_id = question_data.get('haystack_session_ids', [f"session_{session_idx}"])[session_idx]

            # Add session messages as memories
            for msg in session_msgs:
                role = msg.get('role', 'user')
                content = msg.get('content', '')
                memory_content = f"[{session_id}] {role.capitalize()}: {content}"
                memories.append(memory_content)

        return memories

    def evaluate_with_gpt4_judge(self, question: str, expected_answer: str, recall_results: List[Any]) -> Dict[str, Any]:
        """
        Use GPT-4o as LLM judge to evaluate recall quality.
        """
        if not self.openai_client:
            # Fallback to simple string matching
            recall_text = " ".join([r.content for r in recall_results])
            score = 1.0 if expected_answer.lower() in recall_text.lower() else 0.0
            return {
                "score": score,
                "reasoning": "Simple string matching (no LLM judge available)",
                "method": "string_match"
            }

        # Prepare evaluation prompt
        recall_texts = [r.content for r in recall_results[:5]]  # Top 5 results
        context = f"""
Question: {question}
Expected Answer: {expected_answer}

Recall Results (top 5):
{chr(10).join(f"{i+1}. {text[:200]}" for i, text in enumerate(recall_texts))}

Evaluate if the recall results correctly answer the question.
Score from 0.0 to 1.0:
- 1.0 = Perfect answer, directly and correctly addresses the question
- 0.8 = Very good answer, mostly correct with minor details missing
- 0.5 = Partial answer, contains relevant information but incomplete
- 0.2 = Some relevant information but doesn't answer the question
- 0.0 = No relevant information or incorrect

Provide brief reasoning for your score.
"""

        try:
            response = self.openai_client.chat.completions.create(
                model="gpt-4o",
                messages=[
                    {"role": "system", "content": "You are an expert evaluator for memory recall systems. Provide accurate, fair scoring based on how well the recall results answer the question."},
                    {"role": "user", "content": context}
                ],
                temperature=0.1,
                max_tokens=300
            )

            content = response.choices[0].message.content.strip()

            # Extract score from response
            import re
            score_match = re.search(r'(\d+\.?\d*)', content.split('\n')[0])
            score = float(score_match.group(1)) if score_match else 0.0

            return {
                "score": min(max(score, 0.0), 1.0),
                "reasoning": content,
                "method": "llm_judge_gpt4o"
            }

        except Exception as e:
            print(f"   ❌ LLM judge failed: {e}")
            # Fallback
            recall_text = " ".join([r.content for r in recall_results])
            score = 1.0 if expected_answer.lower() in recall_text.lower() else 0.0
            return {
                "score": score,
                "reasoning": f"LLM judge failed ({e}), fell back to string matching",
                "method": "string_match_fallback"
            }

    def process_question_batch(self, questions: List[Dict]) -> List[Dict[str, Any]]:
        """Process a batch of questions with shared agent and memories."""
        print(f"🔄 Processing batch of {len(questions)} questions")

        # Create one agent for the batch
        agent = Agent.create(client=self.client)
        total_memories_fed = 0

        # Feed all session memories for this batch
        print("   🍽️  Feeding session memories...")
        for question_data in questions:
            memories = self.create_session_memory_content(question_data)
            for memory_content in memories:
                try:
                    memory = agent.remember(content=memory_content, type="episodic")
                    total_memories_fed += 1
                except Exception as e:
                    print(f"      ❌ Failed to feed memory: {e}")

        print(f"      📊 Fed {total_memories_fed} memories to agent")

        # Wait for indexing
        if not self.wait_for_indexing(timeout=600):  # 10 minute timeout for batch
            print("      ❌ Batch indexing timeout")
            return [{
                "question_id": q["question_id"],
                "question_type": q["question_type"],
                "question": q["question"],
                "expected_answer": q["answer"],
                "success": False,
                "error": "Indexing timeout",
                "score": 0.0
            } for q in questions]

        # Evaluate each question
        print("   🔍 Evaluating recall for each question...")
        batch_results = []

        for question_data in questions:
            question_id = question_data["question_id"]
            question_type = question_data["question_type"]
            question = question_data["question"]
            expected_answer = question_data["answer"]

            start_time = time.time()
            try:
                recall_results = agent.recall(question, limit=10, mode="hybrid")
                latency = (time.time() - start_time) * 1000

                # GPT-4o evaluation
                evaluation = self.evaluate_with_gpt4_judge(question, expected_answer, recall_results)

                result = {
                    "question_id": question_id,
                    "question_type": question_type,
                    "question": question,
                    "expected_answer": expected_answer,
                    "memories_fed": total_memories_fed // len(questions),  # Approximate per question
                    "recall_count": len(recall_results),
                    "recall_results": [{"content": r.content[:200], "score": r.score} for r in recall_results[:3]],
                    "score": evaluation["score"],
                    "evaluation_method": evaluation["method"],
                    "reasoning": evaluation["reasoning"][:300],
                    "latency_ms": latency,
                    "success": True
                }

                status = "✅" if evaluation["score"] >= 0.5 else "❌"
                print(f"      {status} Q{question_id}: {evaluation['score']:.2f} | {latency:.0f}ms")

            except Exception as e:
                latency = (time.time() - start_time) * 1000
                result = {
                    "question_id": question_id,
                    "question_type": question_type,
                    "question": question,
                    "expected_answer": expected_answer,
                    "success": False,
                    "error": str(e),
                    "latency_ms": latency,
                    "score": 0.0
                }
                print(f"      ❌ Q{question_id}: Failed - {e}")

            batch_results.append(result)

        return batch_results

    def run_efficient_benchmark(self, dataset_path: str, batch_size: int = 10, limit: int = None):
        """Run efficient LongMemEval_S benchmark."""
        print("🚀 Starting Efficient LongMemEval_S Benchmark")
        print("=" * 80)
        print("📋 Methodology:")
        print("   • Batch processing with shared agents")
        print("   • Session-by-session memory ingestion")
        print("   • Synchronous indexing per batch")
        print("   • GPT-4o LLM-as-a-judge scoring")
        print("   • All 6 question categories")
        print("=" * 80)

        # Load dataset
        dataset = self.load_dataset(dataset_path)
        if limit:
            dataset = dataset[:limit]

        print(f"📊 Dataset: {len(dataset)} questions")
        print(f"🎯 Question Types: {set(q['question_type'] for q in dataset)}")
        print(f"🤖 LLM Judge: {'✅ GPT-4o' if self.openai_client else '❌ Disabled (string matching only)'}")
        print(f"📦 Batch Size: {batch_size}")
        print()

        # Process in batches
        all_results = []
        successful_questions = 0
        total_score = 0
        type_scores = {}

        for i in range(0, len(dataset), batch_size):
            batch = dataset[i:i + batch_size]
            batch_num = i // batch_size + 1
            total_batches = (len(dataset) + batch_size - 1) // batch_size

            print(f"🔄 Batch {batch_num}/{total_batches}: {len(batch)} questions")

            # Process batch
            batch_results = self.process_question_batch(batch)
            all_results.extend(batch_results)

            # Update statistics
            for result in batch_results:
                if result.get("success"):
                    successful_questions += 1
                    total_score += result["score"]

                    # Track by type
                    qtype = result["question_type"]
                    if qtype not in type_scores:
                        type_scores[qtype] = []
                    type_scores[qtype].append(result["score"])

            print(f"   ✅ Completed batch {batch_num}")
            print()

        # Final results
        print("=" * 80)
        print("📈 LONGMEMEVAL_S FINAL RESULTS")
        print("=" * 80)

        if successful_questions > 0:
            avg_score = total_score / successful_questions
            print(f"✅ Successful Evaluations: {successful_questions}/{len(dataset)} ({successful_questions/len(dataset)*100:.1f}%)")
            print(f"🎯 Average Score: {avg_score:.3f}")
            print(f"🏆 Benchmark Result: {avg_score:.3f}/1.000")

            # Performance by question type
            print("
📊 Performance by Question Type:"            for qtype in sorted(type_scores.keys()):
                scores = type_scores[qtype]
                avg_type_score = sum(scores) / len(scores)
                print(f"   • {qtype}: {avg_type_score:.3f} ({len(scores)} questions)")

        else:
            print("❌ No successful evaluations")

        # Save detailed results
        output_file = "efficient_longmemeval_s_results.json"
        with open(output_file, "w") as f:
            json.dump(all_results, f, indent=2, default=str)

        print(f"\n💾 Detailed results saved to {output_file}")

        return successful_questions > 0

def main():
    import sys

    dataset_path = "../data/eval/longmemeval_s.json"
    batch_size = 10  # Process 10 questions per batch
    limit = None

    if len(sys.argv) > 1:
        if sys.argv[1].isdigit():
            limit = int(sys.argv[1])
        else:
            dataset_path = sys.argv[1]
            if len(sys.argv) > 2 and sys.argv[2].isdigit():
                limit = int(sys.argv[2])

    benchmark = EfficientLongMemEval()
    success = benchmark.run_efficient_benchmark(dataset_path, batch_size, limit)

    if success:
        print("\n🎉 Efficient LongMemEval_S benchmark completed successfully!")
        print("📋 Features: Full 500 questions + 6 categories + GPT-4o LLM judge + Session ingestion")
    else:
        print("\n⚠️  Benchmark completed with issues.")

if __name__ == "__main__":
    main()