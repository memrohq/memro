# memro Protocol Specification

**Version**: 1.0.0  
**Status**: Draft  
**Last Updated**: 2026-02-01

---

## Overview

memro is an **open protocol** for AI agent memory infrastructure. It defines:

- How agents prove their identity
- How memories are structured and stored
- How memories are recalled
- What guarantees the system provides

**This specification can be implemented by anyone.** memro (the Rust implementation) is one reference implementation, but the protocol is not owned by any single entity.

---

## Design Principles

1. **Identity Sovereignty**: Agents own their cryptographic keys
2. **Data Portability**: Memories can be exported and moved
3. **Verifiable Deletion**: Deletion is cryptographically provable
4. **Open Protocol**: Anyone can implement this spec
5. **Self-Hostable**: No vendor lock-in

---

## Core Concepts

### Agent Identity

An agent is identified by:
- **Agent ID**: SHA-256 hash of ed25519 public key
- **Public Key**: ed25519 public key (32 bytes)
- **Private Key**: ed25519 private key (32 bytes, kept secret)

### Memory

A memory is a piece of information associated with an agent:
- **ID**: UUID v4
- **Agent ID**: Owner of this memory
- **Content**: The actual memory text
- **Type**: episodic, semantic, procedural, or profile
- **Visibility**: private, shared, or public
- **Created At**: ISO 8601 timestamp

### Embedding

A vector representation of memory content:
- **Memory ID**: Links to memory
- **Vector**: 1536-dimensional float array (OpenAI text-embedding-3-small)
- **Model**: Embedding model used
- **Created At**: When embedding was generated

---

## API Specification

See [api/openapi.yaml](api/openapi.yaml) for full OpenAPI 3.0 specification.

### Core Endpoints

#### Identity

```
POST /identity
GET /identity/:id
DELETE /identity/:id
```

#### Memory

```
POST /memory
GET /memory/:agent_id
POST /upload
POST /memory/from-file
```

#### Recall

```
GET /search?agent_id=xxx&query=xxx&limit=10
```

---

## Security Model

See [security/](security/) for detailed threat model and guarantees.

### Request Signing

All mutating requests MUST be signed with the agent's private key using ed25519.

### Guarantees

1. **Identity Ownership**: Only the holder of the private key can act as that agent
2. **Memory Integrity**: Memories cannot be tampered with
3. **Verifiable Deletion**: Deletion leaves cryptographic proof
4. **Auditability**: All actions are logged

---

## Implementation Requirements

Any memro-compatible implementation MUST:

1. Support ed25519 identity generation
2. Implement all core API endpoints
3. Store memories with proper metadata
4. Support semantic search via embeddings
5. Provide data export functionality
6. Support verifiable deletion

---

## Extensions

Implementations MAY add:

- Additional embedding models
- Custom memory types
- Advanced search features
- Multi-modal support (images, audio, video)

---

## Versioning

This specification uses semantic versioning:
- **Major**: Breaking changes to protocol
- **Minor**: Backward-compatible additions
- **Patch**: Clarifications and fixes

---

## License

This specification is released under **CC0 1.0 Universal** (public domain).

Anyone can implement this protocol without permission.
