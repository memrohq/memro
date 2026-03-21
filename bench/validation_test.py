#!/usr/bin/env python3
"""
Simple LongMemEval_S Test
Quick validation of core functionality
"""

import json
import time
from memro import Agent, MemroClient
import requests

def test_basic_functionality():
    """Test basic agent creation, memory feeding, and recall."""
    print("🧪 Testing Basic Functionality")

    client = MemroClient("http://localhost:8081")

    # Test agent creation
    try:
        agent = Agent.create(client=client)
        print("   ✅ Agent created")
    except Exception as e:
        print(f"   ❌ Agent creation failed: {e}")
        return False

    # Test memory feeding
    test_memories = [
        "Alice went to the store to buy groceries",
        "Bob met Alice at the coffee shop",
        "Charlie joined them for lunch at the restaurant"
    ]

    try:
        for memory in test_memories:
            agent.remember(content=memory, type="episodic")
        print("   ✅ Memories fed")
    except Exception as e:
        print(f"   ❌ Memory feeding failed: {e}")
        return False

    # Wait for indexing
    print("   ⏳ Waiting for indexing...")
    time.sleep(5)

    # Test recall
    try:
        results = agent.recall("Where did Alice go?", limit=5, mode="hybrid")
        print(f"   ✅ Recall successful: {len(results)} results")
        if results:
            print(f"      Sample result: {results[0].content[:100]}...")
        return True
    except Exception as e:
        print(f"   ❌ Recall failed: {e}")
        return False

def test_dataset_loading():
    """Test loading the LongMemEval_S dataset."""
    print("📊 Testing Dataset Loading")

    try:
        with open("../data/eval/longmemeval_s.json", 'r') as f:
            dataset = json.load(f)

        print(f"   ✅ Dataset loaded: {len(dataset)} questions")

        # Check structure
        if dataset:
            sample = dataset[0]
            required_fields = ["question_id", "question_type", "question", "answer", "haystack_sessions"]
            missing = [f for f in required_fields if f not in sample]
            if missing:
                print(f"   ❌ Missing fields: {missing}")
                return False
            else:
                print("   ✅ Dataset structure valid")
                print(f"      Sample question type: {sample['question_type']}")
                print(f"      Sample sessions: {len(sample.get('haystack_sessions', []))}")

        return True
    except Exception as e:
        print(f"   ❌ Dataset loading failed: {e}")
        return False

def test_openai_integration():
    """Test OpenAI API integration for LLM judging."""
    print("🤖 Testing OpenAI Integration")

    import os
    api_key = os.getenv("OPENAI_API_KEY")

    if not api_key:
        print("   ⚠️  No OPENAI_API_KEY environment variable")
        return False

    try:
        import openai
        client = openai.OpenAI(api_key=api_key)

        # Simple test
        response = client.chat.completions.create(
            model="gpt-4o",
            messages=[{"role": "user", "content": "Say 'test'"}],
            max_tokens=10
        )

        if response.choices[0].message.content.strip().lower() == "test":
            print("   ✅ OpenAI API working")
            return True
        else:
            print("   ❌ OpenAI API returned unexpected response")
            return False

    except Exception as e:
        print(f"   ❌ OpenAI API failed: {e}")
        return False

def run_quick_benchmark():
    """Run a quick 2-question benchmark."""
    print("🚀 Running Quick 2-Question Benchmark")

    # Load dataset
    with open("../data/eval/longmemeval_s.json", 'r') as f:
        dataset = json.load(f)

    questions = dataset[:2]
    client = MemroClient("http://localhost:8081")

    results = []

    for i, q_data in enumerate(questions):
        print(f"   🔄 Processing question {i+1}/2")

        # Create agent
        agent = Agent.create(client=client)

        # Feed session memories
        memories_fed = 0
        for session_msgs in q_data.get('haystack_sessions', []):
            for msg in session_msgs:
                content = msg.get('content', '')
                if content:
                    try:
                        agent.remember(content=content, type="episodic")
                        memories_fed += 1
                    except Exception as e:
                        print(f"      ❌ Memory feed error: {e}")

        print(f"      📊 Fed {memories_fed} memories")

        # Wait for indexing
        time.sleep(3)

        # Recall
        try:
            recall_results = agent.recall(q_data['question'], limit=5, mode="hybrid")
            print(f"      🔍 Got {len(recall_results)} recall results")

            # Simple evaluation (string matching)
            recall_text = " ".join([r.content for r in recall_results])
            expected = q_data['answer']
            score = 1.0 if expected.lower() in recall_text.lower() else 0.0

            result = {
                "question_id": q_data["question_id"],
                "question": q_data["question"],
                "expected": expected,
                "score": score,
                "recall_count": len(recall_results)
            }
            results.append(result)

            status = "✅" if score >= 0.5 else "❌"
            print(f"      {status} Score: {score}")

        except Exception as e:
            print(f"      ❌ Recall failed: {e}")
            results.append({
                "question_id": q_data["question_id"],
                "error": str(e),
                "score": 0.0
            })

    # Summary
    successful = sum(1 for r in results if r.get("score", 0) > 0)
    avg_score = sum(r.get("score", 0) for r in results) / len(results) if results else 0

    print("
📈 Quick Benchmark Results:"    print(f"   Questions: {len(results)}")
    print(f"   Successful: {successful}")
    print(f"   Average Score: {avg_score:.3f}")

    return results

def main():
    print("🔬 LongMemEval_S Validation Test")
    print("=" * 50)

    tests = [
        ("Basic Functionality", test_basic_functionality),
        ("Dataset Loading", test_dataset_loading),
        ("OpenAI Integration", test_openai_integration)
    ]

    all_passed = True
    for name, test_func in tests:
        try:
            passed = test_func()
            all_passed = all_passed and passed
        except Exception as e:
            print(f"   ❌ {name} crashed: {e}")
            all_passed = False
        print()

    if all_passed:
        print("✅ All validation tests passed!")
        print("🚀 Running quick benchmark...")
        results = run_quick_benchmark()

        # Save results
        with open("validation_test_results.json", "w") as f:
            json.dump(results, f, indent=2)

        print("💾 Results saved to validation_test_results.json")
    else:
        print("❌ Some validation tests failed. Check issues above.")

if __name__ == "__main__":
    main()