/**
 * ed25519 cryptographic operations for the memro protocol.
 * Uses tweetnacl (pure JS, works in Node.js and browsers).
 */
import nacl from 'tweetnacl';
import { encodeUTF8, decodeUTF8, encodeBase64 } from 'tweetnacl-util';

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
  const privateKeyBytes = fromHex(privateKeyHex);
  const timestamp = Math.floor(Date.now() / 1000).toString();

  // ed25519 sign the raw body bytes
  const signature = nacl.sign.detached(body, privateKeyBytes);

  return {
    'X-Agent-Id': agentId,
    'X-Signature': toHex(signature),
    'X-Timestamp': timestamp,
    'Content-Type': 'application/json',
  };
}
