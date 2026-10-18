import nacl from "tweetnacl";

const toHex = (bytes) => Array.from(bytes).map((b) => b.toString(16).padStart(2, "0")).join("");

export const fromHex = (hex) => {
  if (!hex || typeof hex !== "string" || hex.length % 2 !== 0) return new Uint8Array(0);
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < hex.length; i += 2) {
    const val = parseInt(hex.slice(i, i + 2), 16);
    if (isNaN(val)) return new Uint8Array(0);
    bytes[i / 2] = val;
  }
  return bytes;
};

/**
 * Auth headers for the Memro API: the dashboard session token when signed
 * in, otherwise an ed25519 signature over timestamp + body.
 */
export const authHeaders = ({ agentId, privateKey, sessionToken }, body = "") => {
  if (sessionToken) {
    return { Authorization: `Bearer ${sessionToken}` };
  }
  let key = fromHex(privateKey);
  if (key.length === 32) key = nacl.sign.keyPair.fromSeed(key).secretKey;
  if (key.length !== 64) throw new Error("No valid private key for this agent");
  const timestamp = Math.floor(Date.now() / 1000).toString();
  const signature = nacl.sign.detached(new TextEncoder().encode(timestamp + body), key);
  return { "X-Agent-Id": agentId, "X-Signature": toHex(signature), "X-Timestamp": timestamp };
};

export const newKeypair = () => {
  const kp = nacl.sign.keyPair();
  return { publicKey: toHex(kp.publicKey), privateKey: toHex(kp.secretKey) };
};

/** fetch wrapper that throws an Error carrying the server's message. */
export async function api(baseUrl, path, { method = "GET", auth, json, form } = {}) {
  const body = json !== undefined ? JSON.stringify(json) : form;
  const headers = { ...(auth ? authHeaders(auth, typeof body === "string" ? body : "") : {}) };
  if (json !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(`${baseUrl}${path}`, { method, headers, body });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    const err = new Error(text || `${res.status} ${res.statusText}`);
    err.status = res.status;
    throw err;
  }
  if (res.status === 204) return null;
  const type = res.headers.get("content-type") || "";
  return type.includes("application/json") ? res.json() : res.text();
}
