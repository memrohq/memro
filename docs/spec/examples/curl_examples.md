# memro Protocol - cURL Examples

## Prerequisites

- `curl` installed
- `jq` for JSON formatting (optional)
- memro server running on `http://localhost:8081`

---

## 1. Create Agent Identity

```bash
curl -X POST http://localhost:8081/identity | jq
```

**Response:**
```json
{
  "agent_id": "5bd80dd5331ebe5a02d267e03c3c5cb57fcb91b1f12f5c08773e65718717976e",
  "public_key": "a1b2c3d4...",
  "private_key": "x1y2z3..."
}
```

**Save the response:**
```bash
curl -X POST http://localhost:8081/identity > identity.json
export AGENT_ID=$(cat identity.json | jq -r '.agent_id')
export PRIVATE_KEY=$(cat identity.json | jq -r '.private_key')
```

---

## 2. Get Agent Identity (Public Info)

```bash
curl http://localhost:8081/identity/$AGENT_ID | jq
```

**Response:**
```json
{
  "agent_id": "5bd80dd5...",
  "public_key": "a1b2c3d4...",
  "created_at": "2026-02-01T10:30:00Z"
}
```

---

## 3. Create Memory (Requires Signature)

**Note:** Signing in bash is complex. Use Python/JS for production.

For demonstration, here's the request structure:

```bash
curl -X POST http://localhost:8081/memory \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "5bd80dd5...",
    "content": "User prefers concise responses",
    "type": "profile",
    "visibility": "private",
    "timestamp": "2026-02-01T10:30:00Z",
    "signature": "a1b2c3d4..."
  }' | jq
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "5bd80dd5...",
  "content": "User prefers concise responses",
  "type": "profile",
  "visibility": "private",
  "created_at": "2026-02-01T10:30:00Z"
}
```

---

## 4. Get Recent Memories

```bash
curl "http://localhost:8081/memory/$AGENT_ID?limit=10" | jq
```

**With filters:**
```bash
# Only profile memories
curl "http://localhost:8081/memory/$AGENT_ID?type=profile&limit=5" | jq

# Only public memories
curl "http://localhost:8081/memory/$AGENT_ID?visibility=public&limit=10" | jq
```

**Response:**
```json
{
  "memories": [
    {
      "id": "550e8400...",
      "agent_id": "5bd80dd5...",
      "content": "User prefers concise responses",
      "type": "profile",
      "visibility": "private",
      "created_at": "2026-02-01T10:30:00Z"
    }
  ],
  "total": 1
}
```

---

## 5. Search Memories (Semantic)

```bash
curl "http://localhost:8081/search?agent_id=$AGENT_ID&query=user%20preferences&limit=5" | jq
```

**With filters:**
```bash
# Search only procedural memories
curl "http://localhost:8081/search?agent_id=$AGENT_ID&query=deployment&type=procedural&limit=5" | jq

# Search with minimum score
curl "http://localhost:8081/search?agent_id=$AGENT_ID&query=docker&min_score=0.8&limit=10" | jq
```

**Response:**
```json
{
  "results": [
    {
      "memory": {
        "id": "550e8400...",
        "agent_id": "5bd80dd5...",
        "content": "User prefers concise responses",
        "type": "profile",
        "visibility": "private",
        "created_at": "2026-02-01T10:30:00Z"
      },
      "score": 0.94,
      "rank": 1
    }
  ],
  "total": 1,
  "query": "user preferences",
  "took_ms": 23
}
```

---

## 6. Upload File as Memory

```bash
curl -X POST http://localhost:8081/upload \
  -F "file=@document.txt" \
  -F "agent_id=$AGENT_ID" \
  -F "type=semantic" \
  -F "signature=..." | jq
```

---

## 7. Export All Data

```bash
curl "http://localhost:8081/export/$AGENT_ID" | jq > export.json
```

**Export as CSV:**
```bash
curl "http://localhost:8081/export/$AGENT_ID?format=csv" > export.csv
```

**Response:**
```json
{
  "agent_id": "5bd80dd5...",
  "exported_at": "2026-02-01T10:30:00Z",
  "memories": [...],
  "identity": {...}
}
```

---

## 8. Delete Memory (Requires Signature)

```bash
curl -X DELETE http://localhost:8081/memory/550e8400-e29b-41d4-a716-446655440000 \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "5bd80dd5...",
    "timestamp": "2026-02-01T10:30:00Z",
    "signature": "a1b2c3d4..."
  }' | jq
```

**Response:**
```json
{
  "id": "550e8400...",
  "deleted_at": "2026-02-01T10:30:00Z",
  "proof": "sha256_hash...",
  "signature": "a1b2c3d4..."
}
```

---

## 9. Delete Agent Identity (Requires Signature)

```bash
curl -X DELETE http://localhost:8081/identity/$AGENT_ID \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "5bd80dd5...",
    "timestamp": "2026-02-01T10:30:00Z",
    "signature": "a1b2c3d4..."
  }' | jq
```

---

## 10. Health Check

```bash
curl http://localhost:8081/health | jq
```

**Response:**
```json
{
  "status": "healthy",
  "version": "1.0.0"
}
```

---

## Complete Workflow Script

```bash
#!/bin/bash

# 1. Create identity
echo "Creating identity..."
curl -s -X POST http://localhost:8081/identity > identity.json
AGENT_ID=$(cat identity.json | jq -r '.agent_id')
echo "Agent ID: $AGENT_ID"

# 2. Get recent memories (should be empty)
echo -e "\nGetting recent memories..."
curl -s "http://localhost:8081/memory/$AGENT_ID?limit=10" | jq '.total'

# 3. Search memories (requires memories to exist first)
echo -e "\nSearching memories..."
curl -s "http://localhost:8081/search?agent_id=$AGENT_ID&query=test&limit=5" | jq '.total'

# 4. Export data
echo -e "\nExporting data..."
curl -s "http://localhost:8081/export/$AGENT_ID" > export.json
echo "Exported to export.json"

# 5. Health check
echo -e "\nHealth check..."
curl -s http://localhost:8081/health | jq '.status'

echo -e "\n✅ Workflow complete!"
```

---

## Error Handling

### 404 Not Found
```bash
curl http://localhost:8081/identity/nonexistent
```

**Response:**
```json
{
  "error": "agent_not_found",
  "message": "Agent ID does not exist",
  "code": 404
}
```

### 401 Unauthorized
```bash
curl -X POST http://localhost:8081/memory \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "...", "content": "...", "signature": "invalid"}'
```

**Response:**
```json
{
  "error": "invalid_signature",
  "message": "Signature verification failed",
  "code": 401
}
```

### 429 Rate Limited
```bash
# After exceeding rate limit
curl http://localhost:8081/search?agent_id=...&query=...
```

**Response:**
```json
{
  "error": "rate_limit_exceeded",
  "message": "Max 100 searches per minute",
  "code": 429,
  "retry_after": 30
}
```

---

## Tips

1. **Use jq for formatting**: `curl ... | jq`
2. **Save responses**: `curl ... > response.json`
3. **Extract values**: `jq -r '.agent_id'`
4. **Pretty print**: `jq '.'`
5. **Filter results**: `jq '.memories[] | select(.type == "profile")'`
