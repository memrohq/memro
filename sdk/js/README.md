# memro JavaScript/TypeScript SDK

Official JavaScript/TypeScript SDK for the memro protocol - AI agent memory infrastructure.

## Installation

```bash
npm install @memro/sdk
```

**With yarn:**
```bash
yarn add @memro/sdk
```

## Quick Start

### JavaScript

```javascript
const { Agent } = require('@memro/sdk');

// Create a new agent identity
const agent = await Agent.create();
console.log(`Agent ID: ${agent.agentId}`);

// Store a memory
await agent.remember('User prefers TypeScript over JavaScript', { type: 'profile' });

// Recall memories semantically
const results = await agent.recall('what does the user prefer?', { limit: 5 });
results.forEach(result => {
  console.log(`[${result.score.toFixed(2)}] ${result.memory.content}`);
});

// Get recent memories
const recent = await agent.getRecent({ limit: 10, type: 'episodic' });
recent.forEach(memory => {
  console.log(`• ${memory.content}`);
});

// Export all data
const exportData = await agent.export();
console.log(`Exported ${exportData.memories.length} memories`);
```

### TypeScript

```typescript
import { Agent, Memory, SearchResult } from '@memro/sdk';

// Create a new agent identity
const agent = await Agent.create();
console.log(`Agent ID: ${agent.agentId}`);

// Store a memory with type safety
await agent.remember('User prefers TypeScript', {
  type: 'profile',
  visibility: 'private'
});

// Recall with type safety
const results: SearchResult[] = await agent.recall('user preferences', {
  limit: 5,
  type: 'profile'
});

// Get recent memories
const recent: Memory[] = await agent.getRecent({
  limit: 10,
  type: 'episodic'
});
```

## Features

- ✅ **Cryptographic Identity**: ed25519 key generation and management
- ✅ **Memory Storage**: Create and manage episodic, semantic, procedural, and profile memories
- ✅ **Semantic Search**: Vector-based similarity search
- ✅ **Request Signing**: Automatic signature generation for authenticated requests
- ✅ **Data Export**: Full data portability
- ✅ **TypeScript Support**: Full type definitions included
- ✅ **Browser & Node.js**: Works in both environments

## Configuration

### Connect to memro server

```typescript
import { Agent, MemroClient } from '@memro/sdk';

// Connect to custom server
const client = new MemroClient({ baseUrl: 'https://api.memro.co' });
const agent = await Agent.create({ client });

// Or use environment variable
// MEMRO_BASE_URL=https://api.memro.co
const agent = await Agent.create();
```

### Load existing identity

```typescript
// From file
const agent = await Agent.fromFile('agent_identity.json');

// From environment variables
// MEMRO_AGENT_ID=...
// MEMRO_PRIVATE_KEY=...
const agent = await Agent.fromEnv();

// From object
const identity = {
  agentId: '5bd80dd5...',
  privateKey: 'x1y2z3...'
};
const agent = await Agent.fromIdentity(identity);
```

## API Reference

### Agent

#### `Agent.create(options?)`
Create a new agent identity.

**Options:**
- `client?: MemroClient` - Custom client instance

**Returns:** `Promise<Agent>`

#### `Agent.fromFile(path)`
Load agent from JSON file.

#### `Agent.fromEnv()`
Load agent from environment variables.

#### `agent.remember(content, options?)`
Store a new memory.

**Parameters:**
- `content: string` - Memory content (max 10,000 chars)
- `options.type?: MemoryType` - "episodic" | "semantic" | "procedural" | "profile"
- `options.visibility?: Visibility` - "private" | "shared" | "public"
- `options.metadata?: Record<string, any>` - Optional metadata

**Returns:** `Promise<Memory>`

#### `agent.recall(query, options?)`
Search memories semantically.

**Parameters:**
- `query: string` - Search query
- `options.limit?: number` - Max results (default: 10, max: 100)
- `options.type?: MemoryType` - Filter by memory type
- `options.minScore?: number` - Minimum similarity score (0-1)

**Returns:** `Promise<SearchResult[]>`

#### `agent.getRecent(options?)`
Get recent memories without semantic search.

**Parameters:**
- `options.limit?: number` - Max results
- `options.type?: MemoryType` - Filter by memory type
- `options.visibility?: Visibility` - Filter by visibility

**Returns:** `Promise<Memory[]>`

#### `agent.deleteMemory(memoryId)`
Delete a specific memory.

**Returns:** `Promise<DeletionProof>`

#### `agent.export(format?)`
Export all agent data.

**Returns:** `Promise<ExportData>`

#### `agent.save(path)`
Save agent identity to file.

### Types

```typescript
interface Memory {
  id: string;
  agentId: string;
  content: string;
  type: MemoryType;
  visibility: Visibility;
  createdAt: Date;
  metadata?: Record<string, any>;
}

interface SearchResult {
  memory: Memory;
  score: number;
  rank: number;
}

type MemoryType = 'episodic' | 'semantic' | 'procedural' | 'profile';
type Visibility = 'private' | 'shared' | 'public';
```

## Examples

See [examples/](./examples/) directory for complete examples:
- `basic.js` - Basic usage
- `typescript.ts` - TypeScript usage
- `advanced.js` - Advanced features
- `browser.html` - Browser usage

## Development

```bash
# Clone repository
git clone https://github.com/memrohq/memro
cd memro/sdk/js

# Install dependencies
npm install

# Build
npm run build

# Run tests
npm test

# Run linter
npm run lint

# Format code
npm run format
```

## Requirements

- Node.js 16+ or modern browser
- Dependencies:
  - `tweetnacl` - ed25519 signatures
  - `axios` - HTTP client

## Browser Support

The SDK works in modern browsers with ES6+ support:

```html
<script type="module">
  import { Agent } from 'https://cdn.jsdelivr.net/npm/@memro/sdk/dist/index.esm.js';
  
  const agent = await Agent.create();
  console.log(agent.agentId);
</script>
```

## License

Apache License 2.0 - see [LICENSE](../../LICENSE)

## Links

- **Documentation**: https://docs.memro.co/sdk/javascript
- **Protocol Spec**: https://github.com/memrohq/memro/tree/main/docs/spec
- **Issues**: https://github.com/memrohq/memro/issues
- **npm**: https://www.npmjs.com/package/@memro/sdk

## Contributing

Contributions welcome! Please read CONTRIBUTING.md first.

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request
