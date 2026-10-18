/**
 * ed25519 cryptographic operations for the memro protocol.
 * Uses tweetnacl (pure JS, works in Node.js and browsers).
 */
import nacl from 'tweetnacl';

function toHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

function fromHex(hex: string): Uint8Array {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < hex.length; i += 2) {
    bytes[i / 2] = parseInt(hex.slice(i, i + 2), 16);
  }
  return bytes;
}

/**
 * Accepts a hex 32-byte seed (Python SDK, dashboard) or a 64-byte nacl
 * secret key (this SDK) and returns the 64-byte key nacl signs with.
 */
function secretKey(privateKeyHex: string): Uint8Array {
  const bytes = fromHex(privateKeyHex);
  if (bytes.length === nacl.sign.seedLength) {
    return nacl.sign.keyPair.fromSeed(bytes).secretKey;
  }
  if (bytes.length === nacl.sign.secretKeyLength) {
    return bytes;
  }
  throw new Error(
    `memro: invalid private key length ${bytes.length} bytes (want ${nacl.sign.seedLength} or ${nacl.sign.secretKeyLength})`
  );
}

export interface Keypair {
  publicKey: string;  // hex-encoded — this is the agent_id
  privateKey: string; // hex-encoded — keep secret
}

export function generateKeypair(): Keypair {
  const kp = nacl.sign.keyPair();
  return {
    publicKey: toHex(kp.publicKey),
    privateKey: toHex(kp.secretKey),
  };
}

export interface SignedHeaders {
  'X-Agent-Id': string;
  'X-Signature': string;
  'X-Timestamp': string;
  'Content-Type': string;
}

export function signBody(
  agentId: string,
  privateKeyHex: string,
  body: Uint8Array
): SignedHeaders {
  const timestamp = Math.floor(Date.now() / 1000).toString();

  // The signature covers the timestamp followed by the exact body bytes sent
  // (empty for GET and DELETE).
  const ts = new TextEncoder().encode(timestamp);
  const payload = new Uint8Array(ts.length + body.length);
  payload.set(ts, 0);
  payload.set(body, ts.length);
  const signature = nacl.sign.detached(payload, secretKey(privateKeyHex));

  return {
    'X-Agent-Id': agentId,
    'X-Signature': toHex(signature),
    'X-Timestamp': timestamp,
    'Content-Type': 'application/json',
  };
}
