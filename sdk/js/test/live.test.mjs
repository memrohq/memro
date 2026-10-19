// End-to-end against a running server; skipped unless MEMRO_TEST_BASE_URL is set.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { Agent, MemroError } from '../dist/esm/index.js';

const BASE = process.env.MEMRO_TEST_BASE_URL;
const opts = { baseUrl: BASE };

test('signed agent end to end', { skip: !BASE && 'MEMRO_TEST_BASE_URL not set' }, async () => {
  const agent = await Agent.create(opts);
  try {
    const m = await agent.remember('The billing service is written in Go', { type: 'semantic' });
    await agent.remember('Ana prefers short answers', { type: 'profile' });
    assert.ok((await agent.recall('billing service', { mode: 'keyword' })).some((r) => r.memory_id === m.id));
    assert.ok((await agent.recallTemporal('billing service')).some((r) => r.memory_id === m.id));
    assert.ok((await agent.getRecent({ limit: 10 })).some((x) => x.id === m.id));
    assert.equal((await agent.stats()).total_memories, 2);
    assert.equal((await agent.export()).total, 2);
    assert.ok((await agent.reason('what is the billing service written in')).answer);
    assert.equal((await agent.timeline(m.id)).memory_id, m.id);
    assert.equal((await agent.memoryChain(m.id)).root_memory.memory_id, m.id);
    assert.ok((await agent.evolve()).memories_evolved >= 0);
    assert.equal((await agent.usage()).memory_nodes, 2);
    assert.ok((await agent.analyzeConflicts()).total_conflicts >= 0);

    const up = await agent.uploadFile({ name: 'notes.txt', data: new TextEncoder().encode('Quarterly planning starts in March'), type: 'text/plain' });
    assert.match(up.extracted_text ?? '', /Quarterly planning/);
    assert.ok(up.memory_id);

    await agent.addWebhook('https://hooks.example.com/memro');
    assert.deepEqual(await agent.webhooks(), ['https://hooks.example.com/memro']);
    assert.equal(await agent.removeWebhook('https://hooks.example.com/memro'), true);
    assert.deepEqual(await agent.webhooks(), []);

    assert.equal(await agent.deleteMemory(m.id), true);
    assert.equal(await agent.deleteMemory(m.id), false);

    // An API key minted by this agent acts as the same agent.
    const { key } = await agent.client.request('POST', '/api-keys', { json: { name: 'js sdk test' }, signer: { agentId: agent.agentId, privateKey: agent.toJSON().private_key } });
    const keyed = await Agent.fromApiKey(key, opts);
    assert.equal(keyed.agentId, agent.agentId);
    const auto = await keyed.remember('Deploys happen on Tuesdays');
    assert.ok(['episodic', 'semantic', 'procedural', 'profile'].includes(auto.memory_type));

    // Another agent sees none of it.
    const other = await Agent.create(opts);
    try {
      assert.ok(!(await other.recall('Tuesdays deploys', { mode: 'keyword' })).some((r) => r.content.includes('Tuesdays')));
      await assert.rejects(other.client.listMemories(agent.agentId, {}, { agentId: other.agentId, privateKey: other.toJSON().private_key }),
        (e) => e instanceof MemroError && e.status === 403);
    } finally {
      await other.deleteAgent();
    }
  } finally {
    await agent.deleteAgent();
  }
});
