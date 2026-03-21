# Agent Identity

## Overview

Every agent in the memro protocol has a **cryptographic identity** based on ed25519 key pairs.

---

## Identity Structure

```json
{
  "agent_id": "5bd80dd5331ebe5a02d267e03c3c5cb57fcb91b1f12f5c08773e65718717976e",
  "public_key": "a1b2c3d4...",
  "private_key": "x1y2z3..." // NEVER share this
}
```

### Fields

- **agent_id**: SHA-256 hash of the public key (hex encoded)
- **public_key**: ed25519 public key (hex encoded, 64 chars)
- **private_key**: ed25519 private key (hex encoded, 64 chars)

---

## Key Generation

1. Generate ed25519 key pair
2. Hash public key with SHA-256
3. Hex encode the hash → agent_id

### Example (Python)

```python
from nacl.signing import SigningKey
import hashlib

# Generate key pair
private_key = SigningKey.generate()
public_key = private_key.verify_key

# Compute agent_id
agent_id = hashlib.sha256(bytes(public_key)).hexdigest()
```

---

## Identity Ownership

**The holder of the private key IS the agent.**

- No username/password
- No email verification
- No central authority

The private key is the **only** proof of identity.

---

## Security Considerations

### Private Key Storage

- NEVER transmit private key over network
- Store encrypted at rest
- Use secure key management (HSM, keychain, etc.)

### Key Rotation

Agents can generate new identities, but:
- Old agent_id becomes invalid
- Memories must be migrated manually
- No automatic key rotation (by design)

---

## Identity Lifecycle

### Creation

```
POST /identity
→ Returns { agent_id, public_key, private_key }
```

### Verification

Any request can be verified by:
1. Extract agent_id from request
2. Fetch public_key from database
3. Verify signature using public_key

### Deletion

```
DELETE /identity/:id
→ Marks identity as deleted
→ Memories remain but are orphaned
```

---

## Guarantees

1. **Uniqueness**: agent_id is deterministic from public_key
2. **Ownership**: Only private key holder can sign requests
3. **Verifiability**: Anyone can verify signatures with public_key
4. **Portability**: Keys can be exported and imported

---

## Implementation Notes

- Use **ed25519** (not RSA or ECDSA)
- Hash with **SHA-256** (not SHA-1 or MD5)
- Hex encode for transport (not base64)
