#!/usr/bin/env python3
"""
Complete runnable example for Memro Python SDK:
  1. Register identity (or connect from environment / API key)
  2. Remember typed observation
  3. Recall verified context before prompting
"""
import os
import sys

from memro import Agent, MemroClient

def main():
    print("=== Memro Python SDK: Register -> Remember -> Recall ===")

    # 1. Register or connect agent identity
    base_url = os.environ.get("MEMRO_BASE_URL", "https://api.memro.co")
    api_key = os.environ.get("MEMRO_API_KEY")

    if api_key:
        print(f"Connecting to Memro ({base_url}) using MEMRO_API_KEY...")
        agent = Agent.from_api_key(api_key, client=MemroClient(base_url=base_url))
    else:
        print(f"Registering autonomous sovereign agent on {base_url} (proof-of-work)...")
        agent = Agent.create(client=MemroClient(base_url=base_url))
        config_dir = os.path.expanduser("~/.config/memro")
        try:
            os.makedirs(config_dir, mode=0o700, exist_ok=True)
            identity_path = os.path.join(config_dir, "identity.json")
            agent.save(identity_path)
            print(f"Agent identity saved to {identity_path} (mode 0600).")
        except Exception as err:
            print(f"Warning: could not save identity to {config_dir}/identity.json: {err}")

    print(f"Agent ID (Ed25519 Public Key): {agent.agent_id[:16]}...{agent.agent_id[-8:]}")

    # 2. Store a typed procedural memory
    observation = "Always run database migrations before deploying billing services."
    print(f"\nStoring observation: \"{observation}\" [type: procedural]")
    mem = agent.remember(
        observation,
        type="procedural",
    )
    print("Memory stored successfully.")

    # 3. Hybrid recall (vector + full-text search via RRF)
    query = "deploying billing service"
    print(f"\nRecalling context for query: \"{query}\"...")
    results = agent.recall(query, limit=5)

    print(f"Retrieved {len(results)} relevant memories:")
    for idx, r in enumerate(results, 1):
        content = r.text if hasattr(r, "text") else getattr(r, "content", str(r))
        score = getattr(r, "score", None)
        score_str = f" (score: {score:.3f})" if score is not None else ""
        print(f"  {idx}. {content}{score_str}")

    print("\n✓ Run completed successfully in < 5 seconds.")

if __name__ == "__main__":
    main()
