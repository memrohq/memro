# Memro Python SDK - Multi-Tenant Usage Guide

## Installation

```bash
pip install memro
```

## Quick Start

### Single Tenant (Default)

```python
from memro import Agent, MemroClient

# Create new agent identity
agent = Agent.create()  # Registers with default tenant
agent.remember("User loves Python", type="profile")

# Recall memories
results = agent.recall("user preferences")
for memory in results:
    print(f"Score: {memory.score}, Content: {memory.content}")

# Save identity locally
agent.save("identity.json")
```

### Multi-Tenant Setup

In a multi-tenant deployment, each tenant gets a separate server instance or API endpoint:

```bash
# Tenant A
export MEMRO_BASE_URL="https://tenant-a.memro.example.com"
export MEMRO_AGENT_ID="pub_key_tenant_a"
export MEMRO_PRIVATE_KEY="priv_key_tenant_a"

# Your application runs against Tenant A's endpoint
# All operations are automatically isolated to Tenant A
```

### Environment Configuration

```bash
# Single instance with default tenant
export MEMRO_BASE_URL="http://localhost:8081"
export MEMRO_AGENT_ID="agent_public_key"
export MEMRO_PRIVATE_KEY="agent_private_key"

# Load agent from environment
from memro import Agent
agent = Agent.from_env()

# Now all operations are tenant-isolated
```

## Advanced Usage

### Batch Operations

```python
from memro import Agent

agent = Agent.from_env()

# Store multiple memories efficiently
memories = [
    {"content": "User prefers dark mode", "type": "profile"},
    {"content": "Last visited on 2024-01-15", "type": "episodic"},
    {"content": "API integration pattern", "type": "procedural"},
]

for memory in memories:
    agent.remember(
        memory["content"],
        type=memory["type"],
        visibility="private"
    )
```

### Search with Filters

```python
# Semantic search across memories
results = agent.recall(
    "user preferences",
    limit=10
)

# Filter results
high_confidence = [r for r in results if r.score > 0.7]
```

### Data Export

```python
# Export all agent data
export_data = agent.export()
print(f"Total memories: {export_data.total}")
print(f"Exported at: {export_data.exported_at}")

# Save to file
import json
with open("agent_export.json", "w") as f:
    json.dump(export_data.to_dict(), f)
```

## Memory Types

### Profile Memories

Information about the user or agent:
```python
agent.remember("User is a software engineer", type="profile")
agent.remember("Timezone: UTC+8", type="profile")
```

### Episodic Memories

Events and experiences:
```python
agent.remember("User asked about machine learning", type="episodic")
agent.remember("Integration completed successfully", type="episodic")
```

### Semantic Memories

Knowledge and facts:
```python
agent.remember("FastAPI is a Python web framework", type="semantic")
agent.remember("Vector databases enable semantic search", type="semantic")
```

### Procedural Memories

Skills and procedures:
```python
agent.remember("How to deploy to Kubernetes", type="procedural")
agent.remember("Authentication flow with Ed25519", type="procedural")
```

## Visibility Control

```python
# Private (default) - visible only to this agent
agent.remember("Secret API key formula", type="procedural", visibility="private")

# Shared - visible to other agents in organization
agent.remember("Team best practices", type="semantic", visibility="shared")

# Public - visible to anyone (subject to tenant permissions)
agent.remember("Public API documentation", type="semantic", visibility="public")
```

## Integration with LLM

### Auto-Extract and Store from LLM Responses

```python
import re
from memro import Agent

agent = Agent.from_env()

def extract_and_store(llm_response: str):
    """Extract tagged information from LLM response."""
    # Pattern: [REMEMBER: something important]
    remember_pattern = re.compile(r"\[REMEMBER:\s*(.+?)\]", re.IGNORECASE)
    profile_pattern = re.compile(r"\[PROFILE:\s*(.+?)\]", re.IGNORECASE)
    
    for match in remember_pattern.finditer(llm_response):
        agent.remember(match.group(1).strip(), type="episodic")
    
    for match in profile_pattern.finditer(llm_response):
        agent.remember(match.group(1).strip(), type="profile")

# Example LLM response with embedded instructions
llm_response = """
The user mentioned they're building an AI startup.
[PROFILE: User is building a startup called Memro]
[REMEMBER: User interested in persistence layer for AI]
That sounds exciting!
"""

extract_and_store(llm_response)
```

### Voice Agent Integration

```python
from memro import Agent

class VoiceAgent:
    def __init__(self):
        self.agent = Agent.from_env()
    
    def process_audio(self, audio_transcript: str):
        # Recall relevant context
        context = self.agent.recall(audio_transcript, limit=3)
        
        # Build LLM prompt with context
        prompt = f"""
        Context from previous conversations:
        {chr(10).join([m.content for m in context])}
        
        User said: {audio_transcript}
        """
        
        # Get LLM response (pseudo-code)
        llm_response = call_llm(prompt)
        
        # Extract and store learnings
        self.extract_and_store(llm_response)
        
        return llm_response
    
    def extract_and_store(self, response: str):
        # Store key facts for future reference
        self.agent.remember(response, type="episodic")
```

## Error Handling

```python
from memro import Agent, MemroError

agent = Agent.from_env()

try:
    results = agent.recall("something")
except MemroError as e:
    print(f"Memro error: {e.status_code}")
    if e.status_code == 429:
        print("Rate limited - backing off...")
    elif e.status_code == 401:
        print("Authentication failed - check credentials")
```

## Performance Tips

### 1. Reuse Client Connection

```python
# ❌ Don't do this (creates new connection each time)
for i in range(100):
    agent = Agent.from_env()
    agent.remember(f"Memory {i}")

# ✅ Do this (reuses connection)
agent = Agent.from_env()
for i in range(100):
    agent.remember(f"Memory {i}")
```

### 2. Batch Semantic Searches

```python
# ✅ Good - single query with limit
results = agent.recall("query", limit=20)

# ❌ Avoid - multiple small queries
for i in range(5):
    results = agent.recall(f"query variation {i}", limit=4)
```

### 3. Monitor Rate Limits

```python
import time
from memro import Agent, MemroError

agent = Agent.from_env()

def remember_with_backoff(content: str, max_retries: int = 3):
    for attempt in range(max_retries):
        try:
            agent.remember(content)
            return
        except MemroError as e:
            if e.status_code == 429:  # Rate limited
                wait_time = 2 ** attempt
                print(f"Rate limited, retrying in {wait_time}s...")
                time.sleep(wait_time)
            else:
                raise
    raise Exception("Failed after max retries")
```

## Testing

### Mock Client for Testing

```python
from unittest.mock import Mock, patch
from memro import Agent

def test_remember():
    with patch('memro.client.MemroClient.store_memory') as mock_store:
        mock_store.return_value = {"success": True}
        
        agent = Agent("test_id", "test_key")
        agent.remember("test content")
        
        mock_store.assert_called_once()
```

## Debugging

### Enable Logging

```python
import logging

logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger('memro')
logger.setLevel(logging.DEBUG)

# Now you'll see detailed HTTP requests and responses
```

### Check Server Status

```python
from memro import MemroClient

client = MemroClient()
response = client.get_health()
print(response)
# Output: {'status': 'ok', 'db': 'ok', 'vector_store': 'ok'}
```

## Examples

See the [examples](./examples) directory for complete applications:

- [Voice Demo](../../memro-voicedemo/src/) - Audio processing with memory
- [Simple Chat](./examples/simple_chat.py) - Conversational agent
- [Data Pipeline](./examples/data_pipeline.py) - Batch memory ingestion

## API Reference

### Agent Methods

- `remember(content, type="episodic", visibility="private")` - Store memory
- `recall(query, limit=5)` - Search for relevant memories
- `export()` - Export all memories
- `save(path)` - Save agent identity to file

### Memory Model

```python
@dataclass
class Memory:
    id: str
    content: str
    memory_type: str  # "episodic", "semantic", "procedural", "profile"
    visibility: str   # "private", "shared", "public"
    created_at: str
    score: float      # Relevance score (0-1)
    asset_url: Optional[str]
```

## Support

- Documentation: https://memro.dev/docs
- Issues: https://github.com/memrohq/memro/issues
- Discord: https://discord.gg/memro
