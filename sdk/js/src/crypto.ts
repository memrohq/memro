/**
 * ed25519 signing for Memro agents that own their key (tweetnacl: pure JS,
 * works in Node.js, Deno, Bun and browsers). Agents that use an API key do
 * not need any of this.
 */
import nacl from 'tweetnacl';
import { sha256 } from '@noble/hashes/sha2';

export function toHex(bytes: Uint8Array): string {
  let out = '';
  for (const b of bytes) out += b.toString(16).padStart(2, '0');
  return out;
}

export function fromHex(hex: string): Uint8Array {
  if (hex.length % 2 !== 0 || /[^0-9a-f]/i.test(hex)) throw new Error('memro: invalid hex string');
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < bytes.length; i++) bytes[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return bytes;
}

/**
 * Accepts a 32-byte seed (what the Python SDK and `Agent.create` produce) or a
 * 64-byte nacl secret key, as hex.
 */
function secretKey(privateKeyHex: string): Uint8Array {
  const bytes = fromHex(privateKeyHex);
  if (bytes.length === nacl.sign.seedLength) return nacl.sign.keyPair.fromSeed(bytes).secretKey;
  if (bytes.length === nacl.sign.secretKeyLength) return bytes;
  throw new Error(`memro: private key must be 32 or 64 bytes, got ${bytes.length}`);
}

export interface Keypair {
  /** Hex public key; this is the agent ID. */
  publicKey: string;
  /** Hex 32-byte seed. Keep it secret. */
  privateKey: string;
}

export function generateKeypair(): Keypair {
  const seed = nacl.randomBytes(nacl.sign.seedLength);
  const kp = nacl.sign.keyPair.fromSeed(seed);
  return { publicKey: toHex(kp.publicKey), privateKey: toHex(seed) };
}

/**
 * v2 request signature: ed25519 over
 * "<timestamp>\n<METHOD>\n<path?query>\n<body bytes>".
 */
export function signRequest(
  agentId: string,
  privateKeyHex: string,
  method: string,
  pathAndQuery: string,
  body: Uint8Array,
  timestamp: number = Math.floor(Date.now() / 1000),
): Record<string, string> {
  const head = new TextEncoder().encode(`${timestamp}\n${method.toUpperCase()}\n${pathAndQuery}\n`);
  const payload = new Uint8Array(head.length + body.length);
  payload.set(head, 0);
  payload.set(body, head.length);
  return {
    'X-Agent-Id': agentId,
    'X-Timestamp': String(timestamp),
    'X-Signature': toHex(nacl.sign.detached(payload, secretKey(privateKeyHex))),
    'X-Signature-Version': '2',
  };
}

const POW_PREFIX = 'memro-identity-v1';

function leadingZeroBits(digest: Uint8Array): number {
  let n = 0;
  for (const b of digest) {
    if (b === 0) { n += 8; continue; }
    return n + Math.clz32(b) - 24;
  }
  return n;
}

/**
 * Proof-of-work for registering an agent (ADR 0013): a nonce whose SHA-256
 * over `memro-identity-v1:<publicKey>:<timestamp>:<nonce>` starts with
 * `difficulty` zero bits. Difficulty 20 takes about a second.
 */
export function solvePow(publicKey: string, difficulty: number, timestamp = Math.floor(Date.now() / 1000)): { timestamp: number; nonce: string } {
  const enc = new TextEncoder();
  const prefix = enc.encode(`${POW_PREFIX}:${publicKey}:${timestamp}:`);
  for (let i = 0; ; i++) {
    const nonce = i.toString(16);
    const msg = new Uint8Array(prefix.length + nonce.length);
    msg.set(prefix);
    for (let k = 0; k < nonce.length; k++) msg[prefix.length + k] = nonce.charCodeAt(k);
    if (leadingZeroBits(sha256(msg)) >= difficulty) return { timestamp, nonce };
  }
}
