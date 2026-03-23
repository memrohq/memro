import asyncio
import json
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

async def test_mee_complete():
    # Configure the MCP server parameters
    # Note: Using the same environment as the user's running terminal
    server_params = StdioServerParameters(
        command="python3",
        args=["-m", "memro_mcp.server"],
        env={
            "PYTHONPATH": "src",
            "MEMRO_BACKEND_URL": "http://localhost:8081",
            "MEMRO_AGENT_ID": "test-agent-mee",
        }
    )

    async with stdio_client(server_params) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            
            print("\n1. Testing 'remember' with sample episodic data...")
            await session.call_tool("remember", {
                "content": "I am working on the Memory Evolution Engine. We implemented semantic clustering today.",
                "type": "episodic"
            })
            await session.call_tool("remember", {
                "content": "The MEE optimization includes fixing the hybrid search scoring logic.",
                "type": "episodic"
            })
            
            print("\n2. Testing 'mee_reason' (Persistence Check)...")
            reason_res = await session.call_tool("mee_reason", {
                "query": "What are the latest MEE optimizations?"
            })
            print(f"Reasoning Result: {reason_res.content[0].text[:200]}...")

            print("\n3. Testing 'mee_evolve' (Consolidation & Clustering Check)...")
            evolve_res = await session.call_tool("mee_evolve", {})
            print(f"Evolution Result: {evolve_res.content[0].text}")

            print("\n4. Testing 'recall' to see if reasoning was persisted...")
            recall_res = await session.call_tool("recall", {
                "query": "Memory Evolution Engine implementation gaps",
                "limit": 5
            })
            print(f"Recall Result: {recall_res.content[0].text[:500]}...")

if __name__ == "__main__":
    asyncio.run(test_mee_complete())
