# Memro JavaScript / TypeScript SDK

Verified memory infrastructure for AI agents.

## Install

```bash
# npm (once published via CI):
npm install @memro/sdk

# Or install from local directory:
npm install ./sdk/js
```

## Quick Example

```ts
import { Agent } from '@memro/sdk';

const agent = await Agent.create(); // local sovereign Ed25519 identity
const mem = await agent.remember('The billing service was rewritten in Go');
const results = await agent.recall('billing service language');
console.log(results[0].content);
```

Uses the platform `fetch`; the only dependency is `tweetnacl` for signing.

## Connect

```ts
import { Agent } from '@memro/sdk';

const agent = await Agent.fromEnv();                          // MEMRO_API_KEY (+ MEMRO_BASE_URL)
const agent2 = await Agent.fromApiKey('memro_sk_...');        // Memro Cloud
const local = await Agent.fromApiKey(key, { baseUrl: 'http://localhost:8081' });
```

Don't ship API keys to browsers; call Memro from your server.

Self-owned identity (Node.js file helpers shown):

```ts
const agent = await Agent.create();           // key pair generated locally
await agent.save('identity.json');            // mode 0600; keep secret
const same = await Agent.fromFile('identity.json');
```

## Memories

```ts
const m = await agent.remember('The billing service was rewritten in Go', {
  type: 'semantic',            // omit or 'auto' to let the server classify
  visibility: 'private',
  eventDate: '2026-03-01T00:00:00Z',
  metadata: { source: 'standup' },
});
await agent.getRecent({ limit: 20, type: 'profile' });
await agent.deleteMemory(m.id);               // false if it didn't exist
await agent.export();
```

## Search

```ts
const results = await agent.recall('what language is billing in?', { limit: 5, mode: 'hybrid', minScore: 0.3 });
const recent = await agent.recallTemporal('deploy incidents', { afterDate: '2026-09-01T00:00:00Z', temporalWeight: 0.6 });
```

## Memory engine

```ts
const ans = await agent.reason('why did 500s spike after the billing release?');
await agent.evolve();
await agent.resolveConflict(aId, bId);
await agent.memoryChain(m.id);
await agent.timeline(m.id);
await agent.analyzeConflicts();
await agent.stats();
await agent.usage();
```

## Files and webhooks

```ts
import { readFile } from 'node:fs/promises';

await agent.uploadFile({ name: 'notes.pdf', data: await readFile('notes.pdf'), type: 'application/pdf' });
await agent.uploadFile(fileInput.files[0], { remember: false });   // a Blob/File works too
await agent.uploadImage({ name: 'board.png', data: png, type: 'image/png' });
await agent.uploadDocument({ name: 'handbook.docx', data: docx });   // PDF, Word, PowerPoint, Excel, text

await agent.addWebhook('https://example.com/memro-hook');
await agent.webhooks();
await agent.removeWebhook('https://example.com/memro-hook');
```

## Errors and retries

```ts
import { MemroError, RateLimitError } from '@memro/sdk';

try {
  await agent.remember('…');
} catch (e) {
  if (e instanceof RateLimitError) { /* 429 after retries */ }
  else if (e instanceof MemroError) console.log(e.status, e.message);
}
```

Options: `{ baseUrl, apiKey, timeout: 30_000, maxRetries: 2, fetch }`.
429s are retried (honouring `Retry-After`); gateway errors only for `GET`
and `DELETE`.

## Low-level client

`agent.client` is a `MemroClient` with one method per endpoint and
`request(method, path, { query, json, form, signer })` for anything else.
Without an API key, requests are signed with
[v2 signatures](https://github.com/memrohq/memro/blob/main/docs/authentication.md#signed-requests) (`signRequest` is
exported if you need it elsewhere).

---

Repository: [github.com/memrohq/memro](https://github.com/memrohq/memro) · Documentation: [https://memro.co](https://memro.co)
License: Source available under [PolyForm Noncommercial 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0). Free for non-commercial use; commercial use through Memro Cloud or a commercial license.
