import asyncio
from memro_mcp.sdk import MemroSDK

async def run_example():
    # Initialize with your Agent ID and Secret Token
    # In production, these should be securely stored
    sdk = MemroSDK(
        agent_id="ed25519_pub_key_here",
        token="verifiable_secret_token_here",
        base_url="http://localhost:8081"
    )

    print("🧠 Memro: The Active Knowledge Layer")
    
    # 1. Storing Memories (remember)
    print("\n📥 Storing memories...")
    await sdk.remember("The user preferred a dark theme for the dashboard.")
    await sdk.remember("Meeting scheduled for Thursday at 2 PM to discuss the architecture.")
    await sdk.remember("User mentioned they are allergic to peanuts.", memory_type="profile")
    print("✅ Memories stored.")

    # 2. Semantic Retrieval (recall)
    print("\n🔍 Recalling related info...")
    results = await sdk.recall("What are the user's settings?")
    for res in results['results']:
        print(f" - [{res['memory_type']}] {res['content']} (Score: {res['score']:.2f})")

    # 3. Cognitive Reasoning (reason)
    print("\n🤔 Asking the Engine (MEE)...")
    thought = await sdk.reason("Is there anything I should know about preparing food for the user?")
    print(f"Engine Thought: {thought['answer']}")
    print(f"Confidence: {thought['confidence']}")

    # 4. Usage Insights (get_usage)
    print("\n📊 Checking usage limits...")
    usage = await sdk.get_usage()
    print(f"Reasoning Calls: {usage['reason_calls']}/20 per min")
    print(f"Memory Nodes: {usage['memory_nodes']}")
    print(f"Graph Edges: {usage['graph_edges']}")

if __name__ == "__main__":
    asyncio.run(run_example())
