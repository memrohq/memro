# Recall & Search

## Overview

The memro protocol provides semantic search capabilities for retrieving relevant memories based on meaning, not just keywords.

---

## Search Mechanism

### Semantic Search

Uses vector embeddings to find memories by **meaning**:

```
Query: "how to deploy"
↓
Embedding: [0.234, -0.567, ...]
↓
Vector Search (cosine similarity)
↓
Ranked Results:
1. "To deploy: build, push, update manifest" (0.92)
2. "Deployment process documented in wiki" (0.87)
3. "User asked about deployment yesterday" (0.81)
```

---

## Search Endpoint

```
GET /search?agent_id={id}&query={text}&limit={n}&type={type}&min_score={score}
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `agent_id` | string | Yes | Agent whose memories to search |
| `query` | string | Yes | Search query (natural language) |
| `limit` | integer | No | Max results (default: 10, max: 100) |
| `type` | string | No | Filter by memory type |
| `min_score` | float | No | Minimum similarity score (0-1) |
| `visibility` | string | No | Filter by visibility level |

### Example Request

```bash
curl "http://localhost:8081/search?agent_id=5bd80dd5&query=user%20preferences&limit=5"
```

### Example Response

```json
{
  "results": [
    {
      "memory": {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "agent_id": "5bd80dd5...",
        "content": "User prefers concise responses",
        "type": "profile",
        "visibility": "private",
        "created_at": "2026-02-01T10:30:00Z"
      },
      "score": 0.94,
      "rank": 1
    },
    {
      "memory": {
        "id": "660e8400-e29b-41d4-a716-446655440001",
        "agent_id": "5bd80dd5...",
        "content": "User timezone is UTC+5:30",
        "type": "profile",
        "visibility": "private",
        "created_at": "2026-02-01T09:15:00Z"
      },
      "score": 0.89,
      "rank": 2
    }
  ],
  "total": 2,
  "query": "user preferences",
  "took_ms": 23
}
```

---

## Ranking Algorithm

### Similarity Score

Based on **cosine similarity** between query and memory embeddings:

```
score = cos(query_vector, memory_vector)
     = (query · memory) / (||query|| × ||memory||)
```

**Range**: 0.0 (unrelated) to 1.0 (identical)

### Hybrid Ranking (Optional)

Implementations MAY combine:
1. **Semantic similarity** (vector distance)
2. **Recency** (time decay)
3. **Type priority** (profile > semantic > episodic)

Example formula:
```
final_score = 0.7 × similarity + 0.2 × recency + 0.1 × type_weight
```

---

## Filtering

### By Type

```
GET /search?agent_id=...&query=deployment&type=procedural
```

Returns only procedural memories.

### By Visibility

```
GET /search?agent_id=...&query=API&visibility=public
```

Returns only public memories.

### By Minimum Score

```
GET /search?agent_id=...&query=docker&min_score=0.8
```

Returns only memories with similarity ≥ 0.8.

### By Date Range

```
GET /search?agent_id=...&query=bug&after=2026-01-01&before=2026-02-01
```

Returns memories created within date range.

---

## Performance Characteristics

### Latency

- **Target**: < 50ms (p95)
- **Typical**: 20-30ms
- **Breakdown**:
  - Query embedding: 10-15ms
  - Vector search: 5-10ms
  - Memory fetch: 3-5ms
  - Serialization: 2-3ms

### Throughput

- **Target**: 100 QPS per instance
- **Scaling**: Horizontal (add more instances)
- **Bottleneck**: Embedding generation

### Caching

Implementations SHOULD cache:
- Query embeddings (same query)
- Frequent memories
- Agent metadata

---

## Recent Memories

### Endpoint

```
GET /memory/:agent_id?limit=10&type=episodic
```

Returns most recent memories without semantic search.

### Use Cases

- Conversation history
- Recent actions
- Timeline view

### Performance

- **Latency**: < 5ms (no embedding needed)
- **Throughput**: 1000+ QPS

---

## Advanced Features

### Multi-Agent Search

Search across multiple agents (if permissions allow):

```
GET /search?agent_ids=agent1,agent2,agent3&query=deployment
```

### Faceted Search

Group results by type:

```json
{
  "facets": {
    "episodic": 12,
    "semantic": 8,
    "procedural": 3,
    "profile": 2
  },
  "results": [...]
}
```

### Contextual Boosting

Boost certain memory types based on context:

```
GET /search?agent_id=...&query=deploy&boost_type=procedural:2.0
```

---

## Search Quality

### Relevance Metrics

Implementations SHOULD track:
- **Precision@K**: Relevant results in top K
- **Recall@K**: Coverage of relevant memories
- **MRR**: Mean reciprocal rank

### Improving Quality

1. **Better embeddings**: Use larger models
2. **Metadata filtering**: Narrow search scope
3. **Hybrid ranking**: Combine signals
4. **User feedback**: Learn from clicks

---

## Error Handling

### No Results

```json
{
  "results": [],
  "total": 0,
  "query": "nonexistent topic",
  "took_ms": 15
}
```

### Invalid Agent

```json
{
  "error": "agent_not_found",
  "message": "Agent ID does not exist",
  "code": 404
}
```

### Rate Limited

```json
{
  "error": "rate_limit_exceeded",
  "message": "Max 100 searches per minute",
  "code": 429,
  "retry_after": 30
}
```

---

## Best Practices

### Query Formulation

**Good**: Natural language
```
"how to deploy the application"
"user's preferred programming language"
```

**Bad**: Keywords only
```
"deploy app"
"user lang"
```

### Result Limits

- **Default**: 10 results
- **Maximum**: 100 results
- **Recommendation**: Start with 5-10, paginate if needed

### Filtering Strategy

1. Start broad (no filters)
2. Add type filter if needed
3. Add min_score if too many results
4. Add date range for temporal queries

---

## Implementation Requirements

Any memro-compatible implementation MUST:

1. Support semantic search via embeddings
2. Return results ranked by similarity
3. Support filtering by type and visibility
4. Provide latency < 100ms (p95)
5. Handle at least 10 QPS per instance

---

## Example Queries

### Find User Preferences

```bash
curl "http://localhost:8081/search?agent_id=5bd80dd5&query=what%20does%20user%20prefer&type=profile&limit=5"
```

### Find Recent Conversations

```bash
curl "http://localhost:8081/memory/5bd80dd5?type=episodic&limit=20"
```

### Find Deployment Knowledge

```bash
curl "http://localhost:8081/search?agent_id=5bd80dd5&query=how%20to%20deploy&type=procedural&min_score=0.8"
```

### Find All Public Memories

```bash
curl "http://localhost:8081/search?agent_id=5bd80dd5&query=*&visibility=public"
```

---

## Future Extensions

Implementations MAY add:
- **Multi-modal search**: Images, audio, video
- **Cross-lingual search**: Query in one language, find in another
- **Temporal reasoning**: "What did I learn last week?"
- **Graph traversal**: "Find related memories"
