# Memro: Developer Preview Guide 🚀

Welcome to the **Developer Preview** of Memro: The Active Knowledge Layer for AI Agents.

Memro provides your agents with a persistent, causal, and evolving memory substrate. This guide will get you started with our simplified Python SDK.

## 1. Quick Start (Python SDK)

```python
from memro_mcp.sdk import MemroSDK

# Initialize with your Address (Agent ID) and Verification (Token)
memro = MemroSDK(
    agent_id="your_agent_id",
    token="your_secret_token"
)

# 📥 Remember something
await memro.remember("The user is allergic to peanuts.", memory_type="profile")

# 🔍 Recall relevant info
results = await memro.recall("What should I know about the user's diet?")

# 🤔 Intelligent Reasoning (MEE)
thought = await memro.reason("Can I serve the user a PB&J sandwich?")
print(thought['answer']) # "No, the user mentioned a peanut allergy."
```

## 2. Shared Knowledge Infrastructure
Memro is more than a database; it's a **Memory Evolution Engine (MEE)**. It automatically:
- **Resolves Conflicts**: Prevents contradictory memories from corrupting your agent's logic.
- **Evolves Fact**: Consolidates multiple episodic events into single semantic truth.
- **Tracks Causality**: Understands *why* an agent knows what it knows.

## 3. Soft Guardrails & Usage
During the Preview, we enforce minimal guardrails to ensure infrastructure stability:
- **Reasoning Limits**: 20 requests per minute.
- **Evolution Limits**: 5 cycles per minute.
- **Daily Budget**: 100k reasoning tokens per agent.

Check your current usage at any time:
```python
usage = await memro.get_usage()
print(f"Nodes: {usage['memory_nodes']}, Edges: {usage['graph_edges']}")
```

## 4. Golden Use Cases
- **Persistent AI Assistants**: Memories that survive every session.
- **Multi-Agent Coding**: Shared codebase knowledge across a swarm of agents.
- **Autonomous Scheduling**: Recalling complex user preferences across months of interaction.

---
**Get Involved**: Join the #dev-preview channel to share your feedback and help us build the future of AI infrastructure.
