
import os
import sys
from memro import Agent, MemroClient

def test_memro():
    agent_id = "ab7a24f744520625fb44fda13e3c5e7704cd80e15806cf4b1eada850fc9d79ca"
    private_key = "REDACTED_TEST_KEY"
    base_url = "http://localhost:8081"
    
    print(f"Connecting to {base_url}...")
    client = MemroClient(base_url)
    agent = Agent(agent_id=agent_id, private_key=private_key, client=client)
    
    print("Storing a memory...")
    memory = agent.remember(
        content="The user's favorite color is cyan and they love playing chess.",
        type="profile"
    )
    print(f"Stored memory ID: {memory.id}")
    
    print("Recalling memory (semantic search)...")
    results = agent.recall("what is the user's favorite color?")
    
    if results:
        print(f"Found {len(results)} results:")
        for r in results:
            print(f"- [{r.memory_type} | score: {r.score:.2f}] {r.content}")
    else:
        print("No results found.")
        
    print("Getting recent memories...")
    recent = agent.get_recent(limit=5)
    for m in recent:
        print(f"- [{m.memory_type}] {m.content}")

if __name__ == "__main__":
    test_memro()
