"""
memro Protocol - Python Examples

Install dependencies:
    pip install pynacl requests
"""

import hashlib
import json
import time
from nacl.signing import SigningKey
import requests

# Configuration
BASE_URL = "http://localhost:8081"

# ============================================================================
# Example 1: Create Agent Identity
# ============================================================================

def create_identity():
    """Generate a new agent identity"""
    response = requests.post(f"{BASE_URL}/identity")
    identity = response.json()
    
    print("✅ Identity Created:")
    print(f"   Agent ID: {identity['agent_id']}")
    print(f"   Public Key: {identity['public_key']}")
    print(f"   ⚠️  Private Key: {identity['private_key']} (KEEP SECRET!)")
    
    return identity

# ============================================================================
# Example 2: Sign a Request
# ============================================================================

def sign_request(private_key_hex, agent_id, content, memory_type="episodic"):
    """Sign a memory creation request"""
    # Convert hex private key to bytes
    private_key_bytes = bytes.fromhex(private_key_hex)
    signing_key = SigningKey(private_key_bytes)
    
    # Create timestamp
    timestamp = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    
    # Create message to sign
    message = f"{agent_id}:{content}:{timestamp}"
    
    # Sign the message
    signed = signing_key.sign(message.encode())
    signature = signed.signature.hex()
    
    return {
        "agent_id": agent_id,
        "content": content,
        "type": memory_type,
        "timestamp": timestamp,
        "signature": signature
    }

# ============================================================================
# Example 3: Create Memory
# ============================================================================

def create_memory(identity, content, memory_type="episodic"):
    """Create a new memory"""
    # Sign the request
    signed_request = sign_request(
        identity['private_key'],
        identity['agent_id'],
        content,
        memory_type
    )
    
    # Send request
    response = requests.post(
        f"{BASE_URL}/memory",
        json=signed_request
    )
    
    memory = response.json()
    print(f"✅ Memory Created: {memory['id']}")
    print(f"   Content: {memory['content']}")
    print(f"   Type: {memory['type']}")
    
    return memory

# ============================================================================
# Example 4: Search Memories
# ============================================================================

def search_memories(agent_id, query, limit=5):
    """Search memories semantically"""
    response = requests.get(
        f"{BASE_URL}/search",
        params={
            "agent_id": agent_id,
            "query": query,
            "limit": limit
        }
    )
    
    results = response.json()
    
    print(f"\n🔍 Search Results for: '{query}'")
    print(f"   Found {results['total']} results in {results['took_ms']}ms\n")
    
    for result in results['results']:
        memory = result['memory']
        score = result['score']
        print(f"   [{score:.2f}] {memory['content']}")
        print(f"           Type: {memory['type']}, Created: {memory['created_at']}\n")
    
    return results

# ============================================================================
# Example 5: Get Recent Memories
# ============================================================================

def get_recent_memories(agent_id, limit=10, memory_type=None):
    """Get recent memories without semantic search"""
    params = {"limit": limit}
    if memory_type:
        params["type"] = memory_type
    
    response = requests.get(
        f"{BASE_URL}/memory/{agent_id}",
        params=params
    )
    
    data = response.json()
    memories = data['memories']
    
    print(f"\n📋 Recent Memories ({len(memories)} of {data['total']})")
    for memory in memories:
        print(f"   • {memory['content']}")
        print(f"     Type: {memory['type']}, {memory['created_at']}\n")
    
    return memories

# ============================================================================
# Example 6: Export All Data
# ============================================================================

def export_data(agent_id):
    """Export all agent data"""
    response = requests.get(f"{BASE_URL}/export/{agent_id}")
    export = response.json()
    
    print(f"\n📦 Export Complete")
    print(f"   Agent: {export['agent_id']}")
    print(f"   Memories: {len(export['memories'])}")
    print(f"   Exported at: {export['exported_at']}")
    
    # Save to file
    filename = f"memro_export_{agent_id[:8]}.json"
    with open(filename, 'w') as f:
        json.dump(export, f, indent=2)
    
    print(f"   Saved to: {filename}")
    
    return export

# ============================================================================
# Example 7: Complete Workflow
# ============================================================================

def complete_workflow():
    """Demonstrate complete memro workflow"""
    print("=" * 60)
    print("memro Protocol - Complete Workflow Example")
    print("=" * 60)
    
    # 1. Create identity
    print("\n1️⃣  Creating Agent Identity...")
    identity = create_identity()
    agent_id = identity['agent_id']
    
    # 2. Create various memories
    print("\n2️⃣  Creating Memories...")
    
    create_memory(identity, "User prefers Python over JavaScript", "profile")
    create_memory(identity, "User is working on memro protocol implementation", "episodic")
    create_memory(identity, "Docker Compose is used for multi-container orchestration", "semantic")
    create_memory(identity, "To deploy: 1) Build image 2) Push to registry 3) Update manifest", "procedural")
    create_memory(identity, "User asked about deployment at 10:30 AM", "episodic")
    
    # Wait for embeddings to be generated
    print("\n⏳ Waiting for embeddings to be generated...")
    time.sleep(2)
    
    # 3. Search memories
    print("\n3️⃣  Searching Memories...")
    search_memories(agent_id, "what does the user prefer?")
    search_memories(agent_id, "how to deploy")
    
    # 4. Get recent memories
    print("\n4️⃣  Getting Recent Memories...")
    get_recent_memories(agent_id, limit=5)
    
    # 5. Export data
    print("\n5️⃣  Exporting All Data...")
    export_data(agent_id)
    
    print("\n" + "=" * 60)
    print("✅ Workflow Complete!")
    print("=" * 60)

# ============================================================================
# Run Examples
# ============================================================================

if __name__ == "__main__":
    # Run complete workflow
    complete_workflow()
    
    # Or run individual examples:
    # identity = create_identity()
    # create_memory(identity, "Test memory", "episodic")
    # search_memories(identity['agent_id'], "test")
