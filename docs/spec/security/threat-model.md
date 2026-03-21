# Security Model

## Overview

The memro protocol uses **cryptographic signatures** to ensure identity ownership and request authenticity.

---

## Threat Model

### Assets

1. **Agent Private Keys**: The most critical asset
2. **Memory Content**: Potentially sensitive information
3. **API Endpoints**: Must prevent unauthorized access
4. **Database**: Contains all memories and identities

### Threat Actors

1. **External Attackers**: Trying to access/modify memories
2. **Malicious Agents**: Trying to impersonate others
3. **Compromised Infrastructure**: Server-side attacks
4. **Insider Threats**: Database access abuse

### Attack Vectors

1. **Key Theft**: Stealing private keys
2. **Replay Attacks**: Reusing valid signatures
3. **Man-in-the-Middle**: Intercepting requests
4. **SQL Injection**: Database manipulation
5. **DoS**: Resource exhaustion

---

## Security Mechanisms

### 1. Cryptographic Identity

**Mechanism**: ed25519 public-key cryptography

```
Private Key (secret) → Signs requests
Public Key (shared) → Verifies signatures
Agent ID = SHA-256(Public Key)
```

**Guarantees**:
- Only private key holder can act as agent
- Signatures cannot be forged
- Agent ID is deterministic

### 2. Request Signing

All **mutating** requests MUST be signed:

```json
{
  "agent_id": "5bd80dd5...",
  "content": "New memory",
  "timestamp": "2026-02-01T10:30:00Z",
  "signature": "a1b2c3d4..."
}
```

**Signature Generation**:
```python
message = f"{agent_id}:{content}:{timestamp}"
signature = ed25519_sign(private_key, message)
```

**Verification**:
```python
public_key = get_public_key(agent_id)
is_valid = ed25519_verify(public_key, message, signature)
```

### 3. Timestamp Validation

Prevent replay attacks:

```
Request timestamp MUST be within ±5 minutes of server time
```

If outside window → reject request.

### 4. HTTPS/TLS

All production deployments MUST use HTTPS:
- Prevents MITM attacks
- Encrypts data in transit
- Validates server identity

### 5. Rate Limiting

Prevent DoS attacks:
- **Per Agent**: 100 req/sec
- **Per IP**: 1000 req/sec
- **Global**: 10,000 req/sec

### 6. Input Validation

All inputs MUST be validated:
- **Content length**: Max 10,000 chars
- **Agent ID format**: 64-char hex
- **Type**: Must be valid enum
- **SQL injection**: Use parameterized queries

---

## Authentication Flow

### Creating Memory

```
1. Client generates request:
   {
     "agent_id": "5bd80dd5...",
     "content": "New memory",
     "timestamp": "2026-02-01T10:30:00Z"
   }

2. Client signs request:
   message = "5bd80dd5...:New memory:2026-02-01T10:30:00Z"
   signature = sign(private_key, message)

3. Client sends request with signature:
   POST /memory
   {
     ...request...,
     "signature": "a1b2c3d4..."
   }

4. Server verifies:
   - Fetch public_key for agent_id
   - Verify signature
   - Check timestamp
   - Validate inputs

5. Server processes if valid, rejects if not
```

### Searching Memories

**No signature required** for read-only operations:

```
GET /search?agent_id=5bd80dd5&query=deployment
```

**Rationale**: Agent ID is public, memories are agent-scoped.

---

## Data Protection

### At Rest

**Recommended**:
- Encrypt database volumes
- Use encrypted file systems
- Secure key storage (HSM, KMS)

**Not Required** (protocol-level):
- Protocol doesn't mandate encryption at rest
- Implementations MAY add this

### In Transit

**Required**:
- HTTPS/TLS 1.2+ for production
- Valid SSL certificates
- No self-signed certs in production

### In Memory

**Recommended**:
- Clear sensitive data after use
- Avoid logging private keys
- Use secure memory allocation

---

## Access Control

### Agent-Level Isolation

Agents can ONLY access their own memories:

```sql
SELECT * FROM memories 
WHERE agent_id = :requested_agent_id
AND agent_id = :authenticated_agent_id
```

If mismatch → reject.

### Visibility Enforcement

```sql
SELECT * FROM memories
WHERE agent_id = :agent_id
AND (
  visibility = 'private' OR
  visibility = 'public' OR
  (visibility = 'shared' AND :requester IN shared_with)
)
```

### Admin Access

Implementations MAY provide admin endpoints:
- MUST use separate authentication
- MUST log all admin actions
- SHOULD require 2FA

---

## Deletion & Privacy

### Verifiable Deletion

When memory is deleted:

```json
{
  "memory_id": "550e8400...",
  "deleted_at": "2026-02-01T10:30:00Z",
  "proof": "sha256_hash_of_deletion_record",
  "signature": "agent_signature"
}
```

**Guarantees**:
- Deletion is logged
- Deletion is signed by agent
- Deletion is irreversible

### Right to be Forgotten

Agents can delete:
1. **All memories**: `DELETE /memory?agent_id=...`
2. **Specific memory**: `DELETE /memory/:id`
3. **Identity**: `DELETE /identity/:id`

### Data Export

Agents can export all data:

```
GET /export/:agent_id
→ Returns JSON with all memories, embeddings, metadata
```

---

## Audit Logging

Implementations SHOULD log:

```json
{
  "timestamp": "2026-02-01T10:30:00Z",
  "action": "create_memory",
  "agent_id": "5bd80dd5...",
  "ip_address": "192.168.1.1",
  "user_agent": "memro-sdk-python/1.0",
  "success": true,
  "latency_ms": 23
}
```

**Retention**: At least 90 days

---

## Key Management

### Private Key Storage

**Client-side**:
- Encrypted at rest
- OS keychain (macOS Keychain, Windows Credential Manager)
- Environment variables (development only)
- Hardware security modules (production)

**Never**:
- Plain text files
- Version control
- Logs
- Error messages

### Key Rotation

Agents can generate new identity:
1. Create new key pair
2. Export old memories
3. Import to new identity
4. Delete old identity

**Note**: No automatic rotation (by design).

### Key Recovery

**No recovery mechanism**:
- Lost key = lost identity
- No password reset
- No backup keys

**Recommendation**: Agents should backup keys securely.

---

## Security Best Practices

### For Implementations

1. ✅ Use parameterized SQL queries
2. ✅ Validate all inputs
3. ✅ Rate limit all endpoints
4. ✅ Use HTTPS in production
5. ✅ Log security events
6. ✅ Keep dependencies updated
7. ✅ Run security audits

### For SDK Developers

1. ✅ Never log private keys
2. ✅ Use secure key storage
3. ✅ Validate server certificates
4. ✅ Implement request signing correctly
5. ✅ Handle errors securely
6. ✅ Provide key backup guidance

### For Users

1. ✅ Backup private keys
2. ✅ Use strong passphrases
3. ✅ Don't share private keys
4. ✅ Use HTTPS endpoints
5. ✅ Monitor audit logs
6. ✅ Rotate keys periodically

---

## Vulnerability Disclosure

Report security issues to: **security@memro.co**

**Do not** open public GitHub issues for security vulnerabilities.

---

## Compliance

### GDPR

- ✅ Right to access (export)
- ✅ Right to deletion
- ✅ Data portability
- ✅ Consent (agent creates identity)

### SOC 2

Implementations MAY pursue SOC 2 certification:
- Access controls
- Encryption
- Audit logging
- Incident response

---

## Security Checklist

Before deploying to production:

- [ ] HTTPS enabled with valid certificate
- [ ] Rate limiting configured
- [ ] Audit logging enabled
- [ ] Database encrypted at rest
- [ ] Backups configured
- [ ] Monitoring and alerting set up
- [ ] Security headers configured
- [ ] Dependencies updated
- [ ] Penetration testing completed
- [ ] Incident response plan documented
