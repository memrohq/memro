import asyncio
from mcp.client.session import ClientSession
from mcp.client.sse import sse_client
import json

async def test_query_param_auth():
    print("\n--- Testing Query Parameter Auth Fallback ---")
    # 100x Universal Compatibility: Token in query params
    agent_id = "universal_agent"
    token = "universal_secret_token"
    url = f"http://localhost:8080/sse/{agent_id}?token={token}"
    
    print(f"[Compatibility] Connecting to: {url}")
    
    try:
        async with sse_client(url) as (read, write):
            async with ClientSession(read, write) as session:
                await session.initialize()
                
                print("[Compatibility] Session initialized successfully without headers!")
                
                # Check status to verify identity was resolved
                print("[Compatibility] Checking status...")
                status = await session.call_tool("get_system_status", {})
                print(f"[Compatibility] Result:\n{status.content[0].text}")
                
    except Exception as e:
        print(f"Error: {e}")
        print("Tip: Make sure the server is running on port 8080.")

if __name__ == "__main__":
    asyncio.run(test_query_param_auth())
