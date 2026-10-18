# Protocol Guarantees

## Overview

The memro protocol provides **cryptographic guarantees** about identity, memory integrity, and data ownership.

---

## Core Guarantees

### 1. Identity Ownership

**Guarantee**: Only the holder of the private key can act as that agent.

**Mechanism**: ed25519 signatures

**Proof**:
```
Given:
- Agent ID = SHA-256(public_key)
- Request signed with private_key
- Signature verified with public_key

Then:
- Only private_key holder could have created signature
- Therefore, requester owns the identity
```

**Attack Resistance**:
- ❌ Cannot forge signatures without private key
- ❌ Cannot derive private key from public key
- ❌ Cannot brute-force ed25519 keys (2^256 space)

---

### 2. Memory Integrity

**Guarantee**: Memories cannot be tampered with after creation.

**Mechanism**: Immutable append-only storage

**Proof**:
```
Given:
- Memory created at timestamp T1
- Memory has ID = UUID
- Memory content = C

Then:
- Content C cannot be changed
- Timestamp T1 cannot be changed
- ID cannot be reused
```

**Attack Resistance**:
- ❌ Cannot modify existing memories
- ❌ Cannot backdate memories
- ❌ Cannot delete without trace

---

### 3. Verifiable Deletion

**Guarantee**: Deletion is cryptographically provable.

**Mechanism**: Signed deletion records

**Proof**:
```
Given:
- Memory M deleted at timestamp T
- Deletion signed by agent
- Deletion hash = H

Then:
- Deletion is logged with signature
- Hash H proves deletion occurred
- Signature proves agent authorized it
```

**Attack Resistance**:
- ❌ Cannot delete without agent signature
- ❌ Cannot hide deletion (audit trail)
- ❌ Cannot recover deleted memories

---

### 4. Auditability

**Guarantee**: All actions are logged and traceable.

**Mechanism**: Append-only audit log

**Proof**:
```
For every action A:
- Timestamp T recorded
- Agent ID recorded
- Action type recorded
- Result recorded

Then:
- Complete history exists
- Timeline is verifiable
- Actions are attributable
```

**Attack Resistance**:
- ❌ Cannot modify audit logs
- ❌ Cannot delete audit entries
- ❌ Cannot forge timestamps

---

## Security Properties

### Confidentiality

**What**: Agent memories are private by default

**Guarantee**: Only the owning agent can access private memories

**Mechanism**:
```sql
WHERE agent_id = :authenticated_agent_id
AND visibility IN ('private', 'public')
```

**Limitation**: Server operator can access database directly

**Mitigation**: Use end-to-end encryption (optional extension)

---

### Authenticity

**What**: Requests come from claimed agent

**Guarantee**: Signature verification proves identity

**Mechanism**: ed25519 signature verification

**Limitation**: Requires secure key storage

**Mitigation**: Use hardware security modules (HSM)

---

### Integrity

**What**: Data is not tampered with

**Guarantee**: Immutable storage prevents modification

**Mechanism**: Append-only database design

**Limitation**: Database admin could modify directly

**Mitigation**: Use cryptographic checksums (optional extension)

---

### Availability

**What**: Service is accessible

**Guarantee**: None (protocol-level)

**Mechanism**: Implementation-dependent

**Limitation**: Protocol doesn't mandate uptime

**Mitigation**: Use managed cloud services with SLA

---

## Non-Guarantees

### What the Protocol Does NOT Guarantee

#### 1. End-to-End Encryption

**Status**: Optional extension

**Why**: Protocol focuses on identity and integrity, not confidentiality

**Mitigation**: Implementations MAY add E2EE

#### 2. Perfect Forward Secrecy

**Status**: Not provided

**Why**: Agent keys are long-lived

**Mitigation**: Rotate keys periodically

#### 3. Anonymity

**Status**: Not provided

**Why**: Agent ID is public

**Mitigation**: Use separate identities for different contexts

#### 4. Uptime/Availability

**Status**: Implementation-dependent

**Why**: Protocol is infrastructure-agnostic

**Mitigation**: Choose reliable hosting

#### 5. Backup/Recovery

**Status**: User responsibility

**Why**: Private keys are not recoverable

**Mitigation**: Backup keys securely

---

## Cryptographic Assumptions

### Assumptions

1. **ed25519 is secure**: No practical attacks exist
2. **SHA-256 is collision-resistant**: No collisions found
3. **Random number generation is secure**: OS provides good entropy
4. **Private keys are kept secret**: Users protect their keys

### If Assumptions Break

| Assumption | Impact | Mitigation |
|------------|--------|------------|
| ed25519 broken | Identity theft | Migrate to new algorithm |
| SHA-256 broken | ID collisions | Use SHA-3 |
| RNG compromised | Weak keys | Re-generate keys |
| Key leaked | Account takeover | Rotate keys immediately |

---

## Trust Model

### What You Must Trust

1. **Your key storage**: Private keys are secure
2. **The server**: Doesn't modify data maliciously
3. **The network**: HTTPS protects transit
4. **The implementation**: Follows protocol correctly

### What You Don't Trust

1. **Other agents**: Cannot access your memories
2. **The protocol**: Open and auditable
3. **Vendor lock-in**: Data is exportable

---

## Verification Methods

### Verify Identity Ownership

```python
# Given agent_id and signature
public_key = get_public_key(agent_id)
message = reconstruct_signed_message(request)
is_valid = ed25519_verify(public_key, message, signature)

if is_valid:
    print("✅ Agent owns this identity")
else:
    print("❌ Invalid signature")
```

### Verify Memory Integrity

```python
# Check memory hasn't been modified
original_hash = compute_hash(memory)
stored_hash = get_stored_hash(memory.id)

if original_hash == stored_hash:
    print("✅ Memory is intact")
else:
    print("❌ Memory was tampered with")
```

### Verify Deletion

```python
# Check deletion was authorized
deletion_record = get_deletion_record(memory_id)
public_key = get_public_key(deletion_record.agent_id)
is_valid = ed25519_verify(
    public_key,
    deletion_record.message,
    deletion_record.signature
)

if is_valid:
    print("✅ Deletion was authorized")
else:
    print("❌ Unauthorized deletion")
```

---

## Compliance Guarantees

### GDPR

- ✅ **Right to Access**: Export endpoint provides all data
- ✅ **Right to Deletion**: Delete endpoint removes all data
- ✅ **Data Portability**: JSON export format
- ✅ **Consent**: Agent creates identity explicitly

### CCPA

- ✅ **Right to Know**: Export shows all collected data
- ✅ **Right to Delete**: Delete endpoint removes data
- ✅ **Right to Opt-Out**: Don't create identity

### SOC 2

- ✅ **Access Control**: Signature-based authentication
- ✅ **Audit Logging**: All actions logged
- ✅ **Data Integrity**: Immutable storage

---

## Performance Guarantees

### Latency

**Target**: < 50ms (p95) for recall

**Guarantee**: None (implementation-dependent)

**Typical**: 20-30ms

### Throughput

**Target**: 100 QPS per instance

**Guarantee**: None (implementation-dependent)

**Typical**: 50-200 QPS

### Storage

**Guarantee**: Memories are persisted durably

**Mechanism**: ACID-compliant database

---

## Failure Modes

### Key Loss

**Scenario**: Private key is lost

**Impact**: Identity is irrecoverable

**Mitigation**: Backup keys securely

### Server Compromise

**Scenario**: Server is hacked

**Impact**: Memories may be exposed

**Mitigation**: Use E2EE extension

### Database Corruption

**Scenario**: Database is corrupted

**Impact**: Memories may be lost

**Mitigation**: Regular backups

### Network Partition

**Scenario**: Network is unreliable

**Impact**: Requests may fail

**Mitigation**: Retry with exponential backoff

---

## Summary

### What memro GUARANTEES

✅ Identity ownership via cryptographic signatures  
✅ Memory integrity via immutable storage  
✅ Verifiable deletion via signed records  
✅ Auditability via append-only logs  
✅ Data portability via export functionality  

### What memro DOES NOT guarantee

❌ End-to-end encryption (optional)  
❌ Perfect forward secrecy  
❌ Anonymity  
❌ Uptime/availability  
❌ Key recovery  

### Trust Requirements

🔑 You must protect your private keys  
🔒 You must trust the server operator  
🌐 You must use HTTPS in production  
💾 You must backup your data  
