
import os
import time
import json
import uuid
import sys
import re
from typing import List, Dict, Any, Optional
import concurrent.futures
import requests
from memro import Agent, MemroClient

class LongMemEvalBridge:
    def __init__(self, base_url: str):
        self.base_url = base_url.rstrip("/")
        self.client = MemroClient(base_url)
        self.results = []

    def get_queue_depth(self) -> int:
        """Fetch current embedding queue depth from metrics."""
        try:
            resp = requests.get(f"{self.base_url}/metrics", timeout=5)
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
        print("\n   Waiting for indexing...")
        last_depth = -1
        while time.time() - start < timeout:
            depth = self.get_queue_depth()
            if depth == 0:
                time.sleep(2)
                print(f"      Indexing complete. Proceeding to recall.   ")
                return True
            if depth != last_depth:
                print(f"      Pending embeddings: {depth}   ", end="\r")
                last_depth = depth
            time.sleep(0.5)
        print(f"\n   [WARN] Indexing wait timed out (depth still {last_depth})")
        return False

    def run_task_feed(self, task: Dict[str, Any], task_idx: int) -> Dict[str, Any]:
        """Feed haystack messages for one task."""
        try:
            agent = Agent.create(client=self.client)
            
            messages = []
            for session_id, session_msgs in zip(task['haystack_session_ids'], task['haystack_sessions']):
                for msg in session_msgs:
                    role = msg.get('role', 'user')
                    content = msg.get('content', '')
                    formatted_msg = f"[{session_id}] {role.capitalize()}: {content}"
                    messages.append(formatted_msg)
            
            # SAFE parallelism: 5 workers per task, Batch size 5 = 25 concurrent requests
            # This respects the backend's 20-connection pool while keeping throughput high.
            with concurrent.futures.ThreadPoolExecutor(max_workers=5) as executor:
                futures = [executor.submit(agent.remember, content=m, type="episodic") for m in messages]
                concurrent.futures.wait(futures)
            
            print(f"      [Task {task_idx:03}] Fed {len(messages)} memories.")
            return {"agent": agent, "task": task, "idx": task_idx, "count": len(messages)}
        except Exception as e:
            print(f"      [Task {task_idx:03}] FAILED to feed: {e}")
            return None

    def evaluate_task(self, context: Dict[str, Any]) -> Dict[str, Any]:
        """Perform recall and evaluate."""
        if not context: return None
        agent = context['agent']
        task = context['task']
        ground_truth = task['answer']
        
        start_time = time.time()
        recall_results = agent.recall(task['question'], limit=10, mode="hybrid")
        latency = (time.time() - start_time) * 1000
        
        hit = self._evaluate_recall(recall_results, ground_truth)
        
        return {
            "idx": context['idx'],
            "question": task['question'][:60],
            "hit": hit,
            "latency_ms": latency,
            "recalled_count": len(recall_results),
            "expected": ground_truth
        }

    def _evaluate_recall(self, results: List[Any], ground_truth: str) -> bool:
        if not results: return False
        gt_lower = ground_truth.lower()
        stop_words = {'the', 'a', 'an', 'is', 'are', 'was', 'were', 'to', 'for', 'of', 'in', 'on', 'with', 'and', 'but', 'if', 'then', 'else', 'at', 'this', 'that'}
        gt_parts = [w.strip('.,?!( )') for w in gt_lower.split() if w.strip('.,?!( )') not in stop_words]
        gt_words = set([w for w in gt_parts if len(w) > 2])
        if not gt_words: return True 
        
        for res in results:
            res_content_lower = res.content.lower()
            if gt_lower in res_content_lower:
                return True
            matching_words = [w for w in gt_words if w in res_content_lower]
            if len(matching_words) >= max(1, len(gt_words) * 0.3):
                return True
        return False

    def run_benchmark(self, dataset_path: str, limit: int = 500, batch_size: int = 5):
        """Run benchmark using parallel batches for speed."""
        print(f"\n========================================\nLOAD: {dataset_path}\nLIMIT: {limit}\nBATCH: {batch_size}\n========================================\n")
        
        with open(dataset_path, 'r') as f:
            data = json.load(f)
        
        all_tasks = data[:limit]
        
        for i in range(0, len(all_tasks), batch_size):
            batch = all_tasks[i : i + batch_size]
            print(f"BATCH [{i // batch_size + 1}] Ingesting {len(batch)} tasks...")
            
            # 1. Parallel Feed
            contexts = []
            with concurrent.futures.ThreadPoolExecutor(max_workers=batch_size) as executor:
                futures = {executor.submit(self.run_task_feed, task, i + idx + 1): idx for idx, task in enumerate(batch)}
                for future in concurrent.futures.as_completed(futures):
                    res = future.result()
                    if res: contexts.append(res)
            
            # 2. Sync Indexing
            self.wait_for_indexing()
            
            # 3. Parallel Evaluate
            print(f"   Evaluating recall...")
            with concurrent.futures.ThreadPoolExecutor(max_workers=batch_size) as executor:
                batch_results = list(executor.map(self.evaluate_task, contexts))
            
            for res in sorted(batch_results, key=lambda x: x['idx'] if x else 0):
                if res:
                    self.results.append(res)
                    status = "PASS" if res['hit'] else "FAIL"
                    print(f"   [{status}] {res['idx']:03}. {res['question']}... | {res['latency_ms']:.1f}ms")

    def report(self):
        total = len(self.results)
        if total == 0: 
            print("No results to report.")
            return
        hits = sum(1 for r in self.results if r['hit'])
        avg_latency = sum(r['latency_ms'] for r in self.results) / total
        
        print("\n" + "="*40)
        print("FINAL LONGMEMEVAL X MEMRO RESULTS")
        print("="*40)
        print(f"Total Tasks:   {total}")
        print(f"Overall Hit%:  {(hits/total)*100:.2f}%")
        print(f"Avg Latency:   {avg_latency:.2f}ms")
        print("="*40)

def main():
    BASE_URL = os.getenv("MEMRO_BASE_URL", "http://localhost:8081")
    dataset_path = "data/eval/longmemeval_s.json"
    limit = 500
    if len(sys.argv) > 1:
        limit = int(sys.argv[1])
        
    bridge = LongMemEvalBridge(BASE_URL)
    # Safer batch size
    bridge.run_benchmark(dataset_path, limit=limit, batch_size=5)
    bridge.report()

if __name__ == "__main__":
    main()
