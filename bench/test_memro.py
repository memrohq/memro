
import os
import sys
from memro import Agent, MemroClient

def test_memro():
    agent_id = os.environ["MEMRO_AGENT_ID"]
    private_key = os.environ["MEMRO_PRIVATE_KEY"]
    base_url = os.environ.get("MEMRO_BASE_URL", "http://localhost:8081")
    
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
