/**
 * The dashboard talks to the Memro API with the session token from email
 * sign-in. It never holds an agent's private key: tools use API keys, and
 * SDKs that sign requests keep their keys on their own machine.
 */
export const authHeaders = ({ sessionToken }) => (sessionToken ? { Authorization: `Bearer ${sessionToken}` } : {});

/** fetch wrapper that throws an Error carrying the server's message. */
export async function api(baseUrl, path, { method = "GET", auth, json, form } = {}) {
  const body = json !== undefined ? JSON.stringify(json) : form;
  const headers = { ...(auth ? authHeaders(auth) : {}) };
  if (json !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(`${baseUrl}${path}`, { method, headers, body });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    let message = text;
    try { message = JSON.parse(text).message || JSON.parse(text).error || text; } catch { /* plain text */ }
    const err = new Error(message || `${res.status} ${res.statusText}`);
    err.status = res.status;
    throw err;
  }
  if (res.status === 204) return null;
  const type = res.headers.get("content-type") || "";
  return type.includes("application/json") ? res.json() : res.text();
}

/** Hosted MCP endpoint for the environment this dashboard is served from. */
export function mcpUrl() {
  const host = window.location.hostname;
  if (host === "app.memro.co") return "https://mcp.memro.co/mcp";
  if (host.startsWith("app-staging.")) return `https://${host.replace(/^app-staging\./, "mcp-staging.")}/mcp`;
  return "http://localhost:8080/mcp";
}

/** `npx memro-cli connect` flags that target this dashboard's environment. */
export function connectFlags() {
  const host = window.location.hostname;
  if (host === "app.memro.co") return "";
  if (host.startsWith("app-staging.")) return " --staging";
  return " --local";
}
