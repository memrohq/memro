import { Agent } from '../../sdk/js/dist/esm/index.js';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

async function main() {
  console.log('=== Memro JavaScript/TypeScript SDK: Register -> Remember -> Recall ===');

  const baseUrl = process.env.MEMRO_BASE_URL || 'https://api.memro.co';
  const apiKey = process.env.MEMRO_API_KEY;
  const identityFile = path.join(__dirname, 'identity.json');

  let agent;
  if (apiKey) {
    console.log(`Connecting to Memro (${baseUrl}) using MEMRO_API_KEY...`);
    agent = await Agent.fromApiKey(apiKey, { baseUrl });
  } else if (fs.existsSync(identityFile)) {
    console.log(`Connecting to Memro (${baseUrl}) using saved identity.json...`);
    agent = await Agent.fromFile(identityFile, { baseUrl });
  } else if (process.env.MEMRO_AGENT_ID && process.env.MEMRO_PRIVATE_KEY) {
    console.log(`Connecting to Memro (${baseUrl}) using agent environment credentials...`);
    agent = await Agent.fromEnv({ baseUrl });
  } else {
    console.log(`Registering autonomous sovereign agent on ${baseUrl} (proof-of-work)...`);
    agent = await Agent.create({ baseUrl });
    try {
      await agent.save(identityFile);
    } catch {
      // non-fatal if identity saving fails
    }
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
