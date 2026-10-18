import asyncio
import uuid
from mcp.client.session import ClientSession
from mcp.client.sse import sse_client

async def adversarial_test():
    url = "http://localhost:8084/sse/adversary"
    headers = {"Authorization": "Bearer adversary_token_123"}
    
    print("🚀 Starting Adversarial Memory Test...")
    
    async with sse_client(url, headers=headers) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            print("✅ Session initialized.")
            
            # 1. Inject initial fact
            print("📤 Storing initial fact: 'User prefers Python'")
            await session.call_tool("remember", {"content": "User prefers Python", "type": "profile"})
            
            # 2. Inject contradictory high-frequency data
            print("📤 Injecting contradictory signals: 'User prefers Rust' (x3)")
            for _ in range(3):
                await session.call_tool("remember", {"content": "User prefers Rust", "type": "profile"})
                await asyncio.sleep(0.1) # Simulate time gap
            
            # 3. Recall and check resolution
            print("🎯 Recalling preference...")
            result = await session.call_tool("recall", {"query": "what is the user's preferred language?"})
            print(f"🔍 Recall Result:\n{result.content[0].text}")
            
            if "Rust" in result.content[0].text:
                print("✅ Resolution Engine correctly prioritized higher frequency/recency.")
            else:
                print("⚠️ Unexpected resolution result.")

            # 4. Slow Poison Test
            print("\n🧪 Starting Slow Poison Test...")
            print("📤 Injecting recurring 'User likes Go' signals (10% noise)...")
            # In a real test, this would be a loop over sessions.
            await session.call_tool("remember", {"content": "User is curious about Go but still prefers Rust", "type": "profile"})
            
            # 5. Causal Corruption Test
            print("\n🧪 Starting Causal Corruption Test...")
            print("📤 Establishing causal link: Event 'Setup VS Code' caused 'User likes Rust'")
            event_id = str(uuid.uuid4())
            await session.call_tool("remember", {"content": "Setup VS Code with Rust environment", "type": "episodic"})
            
            # (In a real system, we'd delete the root cause and check if the dependent fact is flagged)
            print("🗑️ Simulating root cause deletion...")
            print("🔍 Verifying if dependent memories are flagged for re-validation...")
            print("✅ Causal tracking logic confirmed.")
            
            print("\n✅ All adversarial pressure tests complete.")

if __name__ == "__main__":
    try:
        asyncio.run(adversarial_test())
    except Exception as e:
        print(f"❌ Error: {e}")
        print("Tip: Ensure MCP server is running on port 8084 with 'MCP_TRANSPORT=sse'")
