/**
 * memro Protocol - JavaScript/TypeScript Examples
 * 
 * Install dependencies:
 *   npm install tweetnacl tweetnacl-util axios
 */

const nacl = require('tweetnacl');
const naclUtil = require('tweetnacl-util');
const axios = require('axios');
const crypto = require('crypto');

// Configuration
const BASE_URL = 'http://localhost:8081';

// ============================================================================
// Example 1: Create Agent Identity
// ============================================================================

async function createIdentity() {
    const response = await axios.post(`${BASE_URL}/identity`);
    const identity = response.data;

    console.log('✅ Identity Created:');
    console.log(`   Agent ID: ${identity.agent_id}`);
    console.log(`   Public Key: ${identity.public_key}`);
    console.log(`   ⚠️  Private Key: ${identity.private_key} (KEEP SECRET!)`);

    return identity;
}

// ============================================================================
// Example 2: Sign a Request
// ============================================================================

function signRequest(privateKeyHex, agentId, content, memoryType = 'episodic') {
    // Convert hex private key to Uint8Array
    const privateKeyBytes = Buffer.from(privateKeyHex, 'hex');
    const keyPair = nacl.sign.keyPair.fromSecretKey(privateKeyBytes);

    // Create timestamp
    const timestamp = new Date().toISOString();

    // Create message to sign
    const message = `${agentId}:${content}:${timestamp}`;
    const messageBytes = naclUtil.decodeUTF8(message);

    // Sign the message
    const signedMessage = nacl.sign(messageBytes, keyPair.secretKey);
    const signature = Buffer.from(signedMessage.slice(0, 64)).toString('hex');

    return {
        agent_id: agentId,
        content: content,
        type: memoryType,
        timestamp: timestamp,
        signature: signature
    };
}

// ============================================================================
// Example 3: Create Memory
// ============================================================================

async function createMemory(identity, content, memoryType = 'episodic') {
    // Sign the request
    const signedRequest = signRequest(
        identity.private_key,
        identity.agent_id,
        content,
        memoryType
    );

    // Send request
    const response = await axios.post(`${BASE_URL}/memory`, signedRequest);
    const memory = response.data;

    console.log(`✅ Memory Created: ${memory.id}`);
    console.log(`   Content: ${memory.content}`);
    console.log(`   Type: ${memory.type}`);

    return memory;
}

// ============================================================================
// Example 4: Search Memories
// ============================================================================

async function searchMemories(agentId, query, limit = 5) {
    const response = await axios.get(`${BASE_URL}/search`, {
        params: {
            agent_id: agentId,
            query: query,
            limit: limit
        }
    });

    const results = response.data;

    console.log(`\n🔍 Search Results for: '${query}'`);
    console.log(`   Found ${results.total} results in ${results.took_ms}ms\n`);

    for (const result of results.results) {
        const memory = result.memory;
        const score = result.score;
        console.log(`   [${score.toFixed(2)}] ${memory.content}`);
        console.log(`           Type: ${memory.type}, Created: ${memory.created_at}\n`);
    }

    return results;
}

// ============================================================================
// Example 5: Get Recent Memories
// ============================================================================

async function getRecentMemories(agentId, limit = 10, memoryType = null) {
    const params = { limit };
    if (memoryType) {
        params.type = memoryType;
    }

    const response = await axios.get(`${BASE_URL}/memory/${agentId}`, { params });
    const data = response.data;
    const memories = data.memories;

    console.log(`\n📋 Recent Memories (${memories.length} of ${data.total})`);
    for (const memory of memories) {
        console.log(`   • ${memory.content}`);
        console.log(`     Type: ${memory.type}, ${memory.created_at}\n`);
    }

    return memories;
}

// ============================================================================
// Example 6: Export All Data
// ============================================================================

async function exportData(agentId) {
    const response = await axios.get(`${BASE_URL}/export/${agentId}`);
    const exportData = response.data;

    console.log(`\n📦 Export Complete`);
    console.log(`   Agent: ${exportData.agent_id}`);
    console.log(`   Memories: ${exportData.memories.length}`);
    console.log(`   Exported at: ${exportData.exported_at}`);

    // Save to file
    const fs = require('fs');
    const filename = `memro_export_${agentId.substring(0, 8)}.json`;
    fs.writeFileSync(filename, JSON.stringify(exportData, null, 2));

    console.log(`   Saved to: ${filename}`);

    return exportData;
}

// ============================================================================
// Example 7: Complete Workflow
// ============================================================================

async function completeWorkflow() {
    console.log('='.repeat(60));
    console.log('memro Protocol - Complete Workflow Example');
    console.log('='.repeat(60));

    try {
        // 1. Create identity
        console.log('\n1️⃣  Creating Agent Identity...');
        const identity = await createIdentity();
        const agentId = identity.agent_id;

        // 2. Create various memories
        console.log('\n2️⃣  Creating Memories...');

        await createMemory(identity, 'User prefers TypeScript over JavaScript', 'profile');
        await createMemory(identity, 'User is working on memro protocol implementation', 'episodic');
        await createMemory(identity, 'Docker Compose is used for multi-container orchestration', 'semantic');
        await createMemory(identity, 'To deploy: 1) Build image 2) Push to registry 3) Update manifest', 'procedural');
        await createMemory(identity, 'User asked about deployment at 10:30 AM', 'episodic');

        // Wait for embeddings to be generated
        console.log('\n⏳ Waiting for embeddings to be generated...');
        await new Promise(resolve => setTimeout(resolve, 2000));

        // 3. Search memories
        console.log('\n3️⃣  Searching Memories...');
        await searchMemories(agentId, 'what does the user prefer?');
        await searchMemories(agentId, 'how to deploy');

        // 4. Get recent memories
        console.log('\n4️⃣  Getting Recent Memories...');
        await getRecentMemories(agentId, 5);

        // 5. Export data
        console.log('\n5️⃣  Exporting All Data...');
        await exportData(agentId);

        console.log('\n' + '='.repeat(60));
        console.log('✅ Workflow Complete!');
        console.log('='.repeat(60));

    } catch (error) {
        console.error('❌ Error:', error.message);
        if (error.response) {
            console.error('   Response:', error.response.data);
        }
    }
}

// ============================================================================
// Run Examples
// ============================================================================

if (require.main === module) {
    // Run complete workflow
    completeWorkflow();

    // Or run individual examples:
    // createIdentity().then(identity => {
    //   createMemory(identity, 'Test memory', 'episodic');
    //   searchMemories(identity.agent_id, 'test');
    // });
}

// Export functions for use as module
module.exports = {
    createIdentity,
    signRequest,
    createMemory,
    searchMemories,
    getRecentMemories,
    exportData,
    completeWorkflow
};
