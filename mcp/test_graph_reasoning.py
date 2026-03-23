import asyncio
from mcp.client.session import ClientSession
from mcp.client.sse import sse_client
import json

async def test_graph_reasoning():
    print("\n--- Testing Graph-Augmented Reasoning (Neo4j Path) ---")
    url = "http://localhost:8080/sse/graph_tester"
    headers = {"Authorization": "Bearer mock_graph_token"}
    
    async with sse_client(url, headers=headers) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            
            # 1. Store a relational fact (via standard remember for now)
            print("[Graph] Storing fact: 'Alice is a developer at Memro'")
            await session.call_tool("remember", {"content": "Alice is a developer at Memro"})
            
            # 2. Query the Knowledge Graph for complex relations
            print("[Graph] Querying: 'Who does Alice collaborate with?'")
            result = await session.call_tool("mee_reason", {"query": "Find collaborators of Alice"})
            
            # Since this is a POC stub, we expect our simulated results
            print(f"[Graph] Result:\n{result.content[0].text}")
            
            # 3. Check System Status
            print("[Graph] Checking System Status...")
            status = await session.call_tool("get_system_status", {})
            print(f"[Graph] Status:\n{status.content[0].text}")

if __name__ == "__main__":
    try:
        asyncio.run(test_graph_reasoning())
    except Exception as e:
        print(f"Error: {e}")
        print("Tip: Make sure the server is running on port 8080.")
