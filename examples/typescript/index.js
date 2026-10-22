import { Agent } from '../../sdk/js/dist/esm/index.js';

async function main() {
  console.log('=== Memro JavaScript/TypeScript SDK: Register -> Remember -> Recall ===');

  const baseUrl = process.env.MEMRO_BASE_URL || 'https://api.memro.co';
  const apiKey = process.env.MEMRO_API_KEY;

  let agent;
  if (apiKey) {
    console.log(`Connecting to Memro (${baseUrl}) using MEMRO_API_KEY...`);
    agent = await Agent.fromApiKey(apiKey, { baseUrl });
  } else {
    console.log(`Registering autonomous sovereign agent on ${baseUrl} (proof-of-work)...`);
    agent = await Agent.create({ baseUrl });
  }

  console.log(`Agent ID (Ed25519 Public Key): ${agent.agentId.slice(0, 16)}...${agent.agentId.slice(-8)}`);

  // 2. Remember a typed procedural memory
  const observation = 'Always run database migrations before deploying billing services.';
  console.log(`\nStoring observation: "${observation}" [type: procedural]`);
  await agent.remember(observation, { type: 'procedural' });
  console.log('Memory stored successfully.');

  // 3. Recall verified context before prompting
  const query = 'deploying billing service';
  console.log(`\nRecalling context for query: "${query}"...`);
  const results = await agent.recall(query, { limit: 5 });

  console.log(`Retrieved ${results.length} relevant memories:`);
  results.forEach((r, idx) => {
    const scoreStr = typeof r.score === 'number' ? ` (score: ${r.score.toFixed(3)})` : '';
    console.log(`  ${idx + 1}. ${r.content}${scoreStr}`);
  });

  console.log('\n✓ Run completed successfully in < 5 seconds.');
}

main().catch((err) => {
  console.error('Run failed:', err);
  process.exit(1);
});
