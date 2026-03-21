# Memory Model

## Overview

The memro protocol defines how memories are structured, stored, and managed for AI agents.

---

## Memory Structure

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "5bd80dd5331ebe5a02d267e03c3c5cb57fcb91b1f12f5c08773e65718717976e",
  "content": "User prefers concise responses",
  "type": "profile",
  "visibility": "private",
  "created_at": "2026-02-01T10:30:00Z",
  "metadata": {
    "source": "conversation",
    "confidence": 0.95
  }
}
```

### Fields

- **id**: UUID v4, unique identifier for this memory
- **agent_id**: SHA-256 hash of agent's public key (owner)
- **content**: The actual memory text (UTF-8, max 10,000 chars)
- **type**: Memory classification (see below)
- **visibility**: Access control level (see below)
- **created_at**: ISO 8601 timestamp (UTC)
- **metadata**: Optional JSON object for additional context

---

## Memory Types

### Episodic Memory

**What happened**: Specific events and experiences

```json
{
  "type": "episodic",
  "content": "User asked about deployment on 2026-02-01 at 10:30 AM",
  "metadata": {
    "timestamp": "2026-02-01T10:30:00Z",
    "context": "deployment_discussion"
  }
}
```

**Use cases**:
- Conversation history
- Action logs
- Event tracking

### Semantic Memory

**What is known**: Facts and knowledge

```json
{
  "type": "semantic",
  "content": "Docker Compose is used for multi-container orchestration",
  "metadata": {
    "domain": "devops",
    "verified": true
  }
}
```

**Use cases**:
- Learned facts
- Domain knowledge
- Conceptual understanding

### Procedural Memory

**How to do things**: Skills and procedures

```json
{
  "type": "procedural",
  "content": "To deploy: 1) Build image 2) Push to registry 3) Update k8s manifest",
  "metadata": {
    "skill": "deployment",
    "steps": 3
  }
}
```

**Use cases**:
- Workflows
- Processes
- Step-by-step instructions

### Profile Memory

**Who the user is**: Preferences and characteristics

```json
{
  "type": "profile",
  "content": "User is a backend engineer who prefers Python over JavaScript",
  "metadata": {
    "category": "preferences",
    "confidence": 0.9
  }
}
```

**Use cases**:
- User preferences
- Behavioral patterns
- Personalization data

---

## Visibility Levels

### Private

- **Access**: Only the owning agent
- **Use case**: Sensitive information, internal state
- **Default**: Yes

```json
{
  "visibility": "private",
  "content": "API key stored in environment variable"
}
```

### Shared

- **Access**: Specified agents or groups
- **Use case**: Collaborative memory, team knowledge
- **Implementation**: Requires access control list (ACL)

```json
{
  "visibility": "shared",
  "content": "Team coding standards document",
  "metadata": {
    "shared_with": ["agent_id_1", "agent_id_2"]
  }
}
```

### Public

- **Access**: Anyone with the agent_id
- **Use case**: Public knowledge, published content
- **Warning**: Irreversible once published

```json
{
  "visibility": "public",
  "content": "Public API documentation for memro protocol"
}
```

---

## Storage Guarantees

### Immutability

Once created, memories **cannot be modified**.

- Updates create new memories
- Original memories remain unchanged
- Deletion is separate operation

### Append-Only

Memories are always added, never changed:

```
Memory Timeline:
[M1] → [M2] → [M3] → [M4] ...
  ↓      ↓      ↓      ↓
 kept   kept   kept   kept
```

### Deletion

Memories can be deleted, but:
- Deletion is **verifiable** (cryptographic proof)
- Deletion is **permanent** (no recovery)
- Deletion leaves audit trail

---

## Embeddings

Each memory has an associated vector embedding for semantic search.

```json
{
  "memory_id": "550e8400-e29b-41d4-a716-446655440000",
  "vector": [0.123, -0.456, 0.789, ...], // 1536 dimensions
  "model": "text-embedding-3-small",
  "created_at": "2026-02-01T10:30:00Z"
}
```

### Embedding Generation

1. Memory content is sent to embedding model
2. Vector (1536-dim) is generated
3. Vector is stored in Qdrant
4. Vector is linked to memory by `memory_id`

### Supported Models

- **Default**: `text-embedding-3-small` (OpenAI)
- **Alternative**: `text-embedding-3-large` (OpenAI)
- **Custom**: Implementations MAY support other models

---

## Memory Lifecycle

### Creation

```
POST /memory
{
  "agent_id": "...",
  "content": "...",
  "type": "episodic",
  "visibility": "private"
}

→ Memory created
→ Embedding generated
→ Returns memory object
```

### Retrieval

```
GET /memory/:agent_id?type=profile&limit=10

→ Returns array of memories
→ Ordered by created_at (desc)
→ Filtered by type
```

### Search (Semantic)

```
GET /search?agent_id=...&query=deployment&limit=5

→ Generates query embedding
→ Searches vector database
→ Returns ranked memories
```

### Deletion

```
DELETE /memory/:id
{
  "agent_id": "...",
  "signature": "..."
}

→ Verifies signature
→ Marks memory as deleted
→ Returns deletion proof
```

---

## Constraints

### Content Length

- **Minimum**: 1 character
- **Maximum**: 10,000 characters
- **Encoding**: UTF-8

### Rate Limits

Implementations SHOULD enforce:
- Max 100 memories/second per agent
- Max 1,000 memories/minute per agent
- Max 10,000 memories/hour per agent

### Storage Limits

Implementations MAY enforce:
- Max total memories per agent
- Max storage size per agent
- Retention policies (e.g., 90 days)

---

## Best Practices

### Memory Granularity

**Good**: Atomic, focused memories
```json
{"content": "User prefers dark mode"}
{"content": "User timezone is UTC+5:30"}
```

**Bad**: Compound memories
```json
{"content": "User prefers dark mode and timezone is UTC+5:30"}
```

### Memory Types

Use the most specific type:
- **Episodic**: "User asked X at Y time"
- **Semantic**: "X is a type of Y"
- **Procedural**: "To do X, follow steps Y"
- **Profile**: "User prefers X"

### Metadata Usage

Add context for better recall:
```json
{
  "content": "Deployment failed",
  "metadata": {
    "timestamp": "2026-02-01T10:30:00Z",
    "service": "backend",
    "error_code": "E500"
  }
}
```

---

## Implementation Notes

- Use **PostgreSQL** or equivalent for memory storage
- Use **Qdrant** or equivalent for vector storage
- Ensure **ACID** properties for memory writes
- Implement **connection pooling** for performance
- Use **indexes** on `agent_id`, `type`, `created_at`
