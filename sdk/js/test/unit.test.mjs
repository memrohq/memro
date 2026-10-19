import { test } from 'node:test';
import assert from 'node:assert/strict';
import nacl from 'tweetnacl';
import { Agent, MemroClient, MemroError, RateLimitError, generateKeypair } from '../dist/esm/index.js';

const fromHex = (h) => Uint8Array.from(h.match(/../g).map((b) => parseInt(b, 16)));

/** A fetch stand-in that records requests and replays canned responses. */
function fakeFetch(...responses) {
  const calls = [];
  const fn = async (url, init) => {
    calls.push({ url, ...init, body: init.body ? new Uint8Array(init.body) : new Uint8Array(0) });
    const [status, body, headers] = responses[Math.min(calls.length, responses.length) - 1];
    return new Response(status === 204 ? null : body, { status, headers });
  };
  fn.calls = calls;
  return fn;
}

function verifyV2(agentId, call, pathAndQuery) {
  const h = call.headers;
  assert.equal(h['X-Signature-Version'], '2');
  assert.equal(h['X-Agent-Id'], agentId);
  const head = new TextEncoder().encode(`${h['X-Timestamp']}\n${call.method}\n${pathAndQuery}\n`);
  const payload = new Uint8Array([...head, ...call.body]);
  assert.ok(nacl.sign.detached.verify(payload, fromHex(h['X-Signature']), fromHex(agentId)), 'signature verifies');
}

const memory = (agentId) => JSON.stringify({ id: 'm1', agent_id: agentId, content: 'x', memory_type: 'semantic', visibility: 'private', created_at: 'now' });

test('signed remember covers method, path and JSON body', async () => {
  const kp = generateKeypair();
  const fetch = fakeFetch([200, memory(kp.publicKey)]);
  const agent = Agent.fromIdentity({ agent_id: kp.publicKey, private_key: kp.privateKey }, { baseUrl: 'http://x.test', fetch });
  await agent.remember('hello', { type: 'semantic' });
  verifyV2(kp.publicKey, fetch.calls[0], '/memory');
  assert.equal(JSON.parse(new TextDecoder().decode(fetch.calls[0].body)).content, 'hello');
});

test('signature covers the exact query string, behind a path prefix', async () => {
  const kp = generateKeypair();
  const fetch = fakeFetch([200, '{"results":[],"mode":"keyword","latency_ms":1,"total":0}']);
  const agent = Agent.fromIdentity({ agent_id: kp.publicKey, private_key: kp.privateKey }, { baseUrl: 'http://x.test/api', fetch });
  await agent.recall('what & why?', { mode: 'keyword', limit: 5 });
  const url = fetch.calls[0].url;
  assert.ok(url.startsWith('http://x.test/api/search?'));
  verifyV2(kp.publicKey, fetch.calls[0], url.slice('http://x.test/api'.length));
});

test('multipart uploads are signed over the bytes sent', async () => {
  const kp = generateKeypair();
  const fetch = fakeFetch([200, '{"file_id":"f","file_name":"a.txt","file_type":"text","file_size":2,"extracted_text":"hi"}']);
  const agent = Agent.fromIdentity({ agent_id: kp.publicKey, private_key: kp.privateKey }, { baseUrl: 'http://x.test', fetch });
  await agent.uploadFile({ name: 'a.txt', data: new TextEncoder().encode('hi'), type: 'text/plain' }, { remember: false });
  assert.match(fetch.calls[0].headers['Content-Type'], /^multipart\/form-data; boundary=/);
  verifyV2(kp.publicKey, fetch.calls[0], '/upload');
});

test('API keys are sent as a bearer token, never signed', async () => {
  const fetch = fakeFetch([200, '{"agent_id":"abc","tenant_id":"t","user_id":null,"auth":"api_key"}']);
  const agent = await Agent.fromApiKey('memro_sk_test', { baseUrl: 'http://x.test', fetch });
  assert.equal(agent.agentId, 'abc');
  assert.equal(fetch.calls[0].headers.Authorization, 'Bearer memro_sk_test');
  assert.equal(fetch.calls[0].headers['X-Signature'], undefined);
});

test('rejects keys that are not API keys, defaults to Memro Cloud', () => {
  assert.throws(() => new MemroClient({ apiKey: 'a'.repeat(64) }));
  delete process.env.MEMRO_BASE_URL;
  assert.equal(new MemroClient({ apiKey: 'memro_sk_x' }).baseUrl, 'https://api.memro.co');
});

test('429 is retried; POST is not retried after a gateway error', async () => {
  const ok = '{"agent_id":"abc","tenant_id":"t","user_id":null,"auth":"api_key"}';
  let fetch = fakeFetch([429, 'Rate limit exceeded', { 'Retry-After': '0' }], [200, ok]);
  const c = new MemroClient({ baseUrl: 'http://x.test', apiKey: 'memro_sk_x', fetch });
  assert.equal((await c.whoami()).agent_id, 'abc');
  assert.equal(fetch.calls.length, 2);

  fetch = fakeFetch([502, 'bad gateway']);
  const c2 = new MemroClient({ baseUrl: 'http://x.test', apiKey: 'memro_sk_x', fetch });
  await assert.rejects(c2.createMemory({ content: 'x' }), (e) => e instanceof MemroError && e.status === 502);
  assert.equal(fetch.calls.length, 1);

  fetch = fakeFetch([429, 'slow down']);
  const c3 = new MemroClient({ baseUrl: 'http://x.test', apiKey: 'memro_sk_x', fetch, maxRetries: 0 });
  await assert.rejects(c3.whoami(), RateLimitError);
});

test('deleteMemory returns false for a missing memory', async () => {
  const fetch = fakeFetch([404, 'Memory not found']);
  const c = new MemroClient({ baseUrl: 'http://x.test', apiKey: 'memro_sk_x', fetch });
  assert.equal(await c.deleteMemory('m1'), false);
});
