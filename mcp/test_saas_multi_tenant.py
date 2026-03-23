import asyncio
import json
import uuid
from memro_mcp.server import CoordinationEngine

class MockMemory:
    def __init__(self, content, memory_type, metadata):
        self.content = content
        self.memory_type = memory_type
        self.metadata = metadata

class MockAgent:
    def __init__(self, agent_id, memories=None):
        self.agent_id = agent_id
        self.memories = memories or []
    
    def get_recent(self, limit=50):
        return self.memories

async def test_session_isolation():
    session_a = "team_alpha"
    session_b = "team_beta"
    resource = "src/auth.ts"
    
    # 1. Setup Session A claim
    mem_a = MockMemory(
        content="Claiming auth for refactor",
        memory_type="procedural",
        metadata={
            "action": "claim",
            "resource": resource,
            "status": "active",
            "session_id": session_a,
            "agent_id": "agent_1"
        }
    )
    
    agent_a = MockAgent("agent_1", [mem_a])
    engine_a = CoordinationEngine(agent_a, session_id=session_a)
    
    print(f"--- Session A ({session_a}) ---")
    decision_a = engine_a.can_act(resource, "Refactoring")
    print(f"Can agent_1 act on {resource} in {session_a}? {decision_a['decision']}")
    
    # 2. Check Session B (Isolation Test)
    # Even if the underlying agent has access to the same memories (e.g. shared DB), 
    # the engine should filter by the session_id provided to the engine.
    agent_b = MockAgent("agent_2", [mem_a]) 
    engine_b = CoordinationEngine(agent_b, session_id=session_b)
    
    print(f"\n--- Session B ({session_b}) ---")
    decision_b = engine_b.can_act(resource, "Bug fix")
    print(f"Can agent_2 act on {resource} in {session_b}? {decision_b['decision']}")
    
    # Validation
    if decision_a["decision"] == "block" and decision_b["decision"] == "allow":
        print("\nSUCCESS: Isolation Test logic verified (Session-based filtering works).")
    else:
        print("\nFAILURE: Isolation Test failed.")
        print(f"Session A Decision: {decision_a['decision']} (Expected: block)")
        print(f"Session B Decision: {decision_b['decision']} (Expected: allow)")
        exit(1)

if __name__ == "__main__":
    asyncio.run(test_session_isolation())
