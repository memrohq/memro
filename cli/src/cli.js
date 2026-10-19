import { execFile } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { CLIENTS, findClient } from "./clients.js";
import { pick } from "./picker.js";
import { installInstructions, uninstallInstructions } from "./instructions.js";

const ENVIRONMENTS = {
  prod: "https://api.memro.co",
  staging: "https://api-staging.memro.co",
  local: "http://localhost:8081",
};

const CREDENTIALS = () => path.join(os.homedir(), ".memro", "credentials.json");

const HELP = `memro-cli — connect your AI tools to Memro memory

Usage
  npx memro-cli connect      Sign in and add Memro to Claude Code, Codex, Cursor,
                             Antigravity, OpenCode, Gemini CLI, VS Code, Windsurf
                             and Claude Desktop
  npx memro-cli status       Show the connected account and configured tools
  npx memro-cli disconnect   Remove Memro from your tools and forget the key

Options
  --client <ids>             Only these tools, comma-separated, skipping the list (${CLIENTS.map((c) => c.id).join(", ")})
  --yes, -y                  Don't ask: use every detected tool
  --no-instructions          Don't add the "use Memro automatically" note to
                             the tools' global instructions (CLAUDE.md, AGENTS.md…)
  --staging | --local        Use staging or a local server instead of Memro Cloud
  --api <url>                Use a specific Memro API (self-hosted)
`;

function parseArgs(argv) {
  const opts = { command: null, clients: null, yes: false, api: null, instructions: true };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--client" || a === "--clients") opts.clients = (argv[++i] || "").split(",").map((s) => s.trim()).filter(Boolean);
    else if (a === "--yes" || a === "-y") opts.yes = true;
    else if (a === "--no-instructions") opts.instructions = false;
    else if (a === "--staging") opts.api = ENVIRONMENTS.staging;
    else if (a === "--local") opts.api = ENVIRONMENTS.local;
    else if (a === "--api") {
      opts.api = argv[++i];
      if (!opts.api || opts.api.startsWith("-")) throw new Error("--api needs a URL, e.g. --api https://memro.example.com");
    }
    else if (a === "--help" || a === "-h") opts.command = "help";
    else if (!opts.command) opts.command = a;
    else throw new Error(`Unexpected argument: ${a}`);
  }
  opts.api = checkURL((opts.api || process.env.MEMRO_API_URL || ENVIRONMENTS.prod).replace(/\/+$/, ""), "API");
  return opts;
}

const LOCAL_HOSTS = new Set(["localhost", "127.0.0.1", "[::1]"]);

/** API keys travel with every request, so remote servers must use HTTPS. */
export function checkURL(url, what) {
  let u;
  try { u = new URL(url); } catch { throw new Error(`Invalid ${what} URL: ${url}`); }
  if (u.protocol === "https:" || (u.protocol === "http:" && LOCAL_HOSTS.has(u.hostname))) return url;
  throw new Error(`The ${what} URL must use https:// (plain http is only allowed for localhost): ${url}`);
}

async function request(api, route, { method = "GET", json, key } = {}) {
  let res;
  try {
    res = await fetch(api + route, {
      method,
      headers: { ...(json ? { "Content-Type": "application/json" } : {}), ...(key ? { Authorization: `Bearer ${key}` } : {}) },
      body: json ? JSON.stringify(json) : undefined,
    });
  } catch (err) {
    throw new Error(`Couldn't reach ${api} (${err.cause?.code || err.message})`);
  }
  const text = await res.text();
  let data;
  try { data = text ? JSON.parse(text) : null; } catch { data = { message: text }; }
  return { ok: res.ok, status: res.status, data };
}

function openBrowser(url) {
  if (process.env.MEMRO_NO_BROWSER) return;
  const [cmd, args] =
    process.platform === "darwin" ? ["open", [url]]
    : process.platform === "win32" ? ["cmd", ["/c", "start", "", url]]
    : ["xdg-open", [url]];
  execFile(cmd, args, () => { /* if it fails the URL is printed anyway */ });
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/** RFC 8628 device login: returns { api_key, agent_id, api_url, mcp_url }. */
async function deviceLogin(api) {
  const client_name = `memro connect on ${os.hostname().replace(/\.local$/, "")}`;
  const start = await request(api, "/auth/device/code", { method: "POST", json: { client_name } });
  if (!start.ok) throw new Error(`Couldn't start sign-in: ${start.data?.message || start.status}`);
  const { device_code, user_code, verification_uri_complete, expires_in, interval } = start.data;

  console.log(`\n  Your code: \x1b[1m${user_code}\x1b[0m`);
  console.log(`  Opening ${verification_uri_complete}`);
  console.log("  Sign in there and press Connect. (If the browser didn't open, visit the link.)\n");
  openBrowser(verification_uri_complete);

  const deadline = Date.now() + (expires_in || 600) * 1000;
  let wait = (interval || 3) * 1000;
  process.stdout.write("  Waiting for approval…");
  while (Date.now() < deadline) {
    await sleep(wait);
    let res;
    try {
      res = await request(api, "/auth/device/token", { method: "POST", json: { device_code } });
    } catch {
      continue; // a dropped connection: keep polling until the code expires
    }
    if (res.ok) {
      process.stdout.write(" approved.\n");
      checkURL(res.data.mcp_url, "MCP");
      return res.data;
    }
    const code = res.data?.error;
    if (code === "authorization_pending") continue;
    if (code === "slow_down") { wait += 2000; continue; }
    if (res.status >= 500 || res.status === 429) { wait = Math.min(wait * 2, 30_000); continue; }
    process.stdout.write("\n");
    if (code === "expired_token") throw new Error("The code expired. Run `npx memro-cli connect` again.");
    throw new Error(`Sign-in failed: ${res.data?.message || code || res.status}`);
  }
  process.stdout.write("\n");
  throw new Error("Timed out waiting for approval. Run `npx memro-cli connect` again.");
}

function saveCredentials(creds) {
  const file = CREDENTIALS();
  fs.mkdirSync(path.dirname(file), { recursive: true, mode: 0o700 });
  fs.writeFileSync(file, JSON.stringify(creds, null, 2) + "\n", { mode: 0o600 });
  fs.chmodSync(file, 0o600);
}

function loadCredentials() {
  try { return JSON.parse(fs.readFileSync(CREDENTIALS(), "utf8")); } catch { return null; }
}

function selectClients(ids) {
  if (!ids) return CLIENTS.filter((c) => c.detect());
  return ids.map((id) => {
    const c = findClient(id);
    if (!c) throw new Error(`Unknown client "${id}". Known: ${CLIENTS.map((x) => x.id).join(", ")}`);
    return c;
  });
}

/**
 * Which tools to configure: --client wins; --yes takes every detected tool;
 * otherwise the user picks in a list with detected tools pre-selected.
 * Returns null when the user cancels or nothing can be asked.
 */
async function chooseTargets(opts) {
  if (opts.clients) return selectClients(opts.clients);
  const detected = new Set(CLIENTS.filter((c) => c.detect()));
  if (opts.yes) return [...detected];
  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    console.log("  Not a terminal: pass --yes (every detected tool) or --client <ids>.");
    return null;
  }
  console.log("");
  const picked = await pick(
    [
      ...CLIENTS.map((c) => ({ label: c.name, hint: detected.has(c) ? "" : "not detected", checked: detected.has(c), client: c })),
      { label: "Teach these tools to use Memro automatically", hint: "adds a short note to CLAUDE.md, AGENTS.md…",
        checked: opts.instructions, option: "instructions" },
    ],
    { title: "Which tools should use Memro?" },
  );
  if (!picked) return null;
  opts.instructions = picked.some((p) => p.option === "instructions");
  return picked.filter((p) => p.client).map((p) => p.client);
}

async function connect(opts) {
  console.log(`\n◆ Memro connect  (${opts.api})`);
  // Choose first, so cancelling never creates an API key.
  const targets = await chooseTargets(opts);
  if (!targets) {
    console.log("  Nothing changed.");
    return;
  }
  if (targets.length === 0 && !opts.yes) {
    console.log("  No tools selected. Nothing changed.");
    return;
  }

  let creds = loadCredentials();
  if (creds?.api_url === opts.api) {
    const who = await request(opts.api, "/auth/whoami", { key: creds.api_key });
    if (who.status === 401 || who.status === 403) creds = null;
    // Anything else (an outage, a rate limit) says nothing about the key;
    // signing in again would mint a second one.
    else if (!who.ok) throw new Error(`Memro API returned ${who.status}. Try again in a moment.`);
    else console.log(`  Already signed in as agent ${creds.agent_id.slice(0, 12)}…`);
  } else {
    creds = null;
  }
  if (!creds) {
    const res = await deviceLogin(opts.api);
    creds = { api_url: opts.api, mcp_url: res.mcp_url, agent_id: res.agent_id, api_key: res.api_key };
    saveCredentials(creds);
  }

  if (targets.length === 0) {
    console.log("\n  No supported tools found on this machine.");
    printManual(creds);
    return;
  }
  console.log(`\n  Adding Memro to: ${targets.map((c) => c.name).join(", ")}`);

  const conn = { mcpUrl: creds.mcp_url, apiKey: creds.api_key };
  let ok = 0;
  for (const c of targets) {
    try {
      c.add(conn);
      ok++;
      console.log(`  \x1b[32m✔\x1b[0m ${c.name.padEnd(15)} ${tilde(c.file())}`);
      if (opts.instructions && c.rules) {
        const result = installInstructions(c.rules());
        const note = result === "kept-own" ? "already tells it to use Memro; left as is" : "use Memro automatically";
        console.log(`  \x1b[32m✔\x1b[0m ${"".padEnd(15)} ${tilde(c.rules())} (${note})`);
      }
    } catch (err) {
      console.log(`  \x1b[33m!\x1b[0m ${c.name.padEnd(15)} ${err.message}`);
    }
  }
  console.log(`\n  Done. Restart ${ok === 1 ? "the tool" : "your tools"} to load Memro's memory tools.`);
  console.log("  Manage or revoke the key anytime in the dashboard → API keys.\n");
}

function printManual(creds) {
  console.log("\n  Add this MCP server to your tool manually:");
  console.log(`    URL:     ${creds.mcp_url}`);
  console.log("    Header:  Authorization: Bearer <the key in ~/.memro/credentials.json>\n");
}

async function status(opts) {
  const creds = loadCredentials();
  if (!creds) {
    console.log("Not connected. Run `npx memro-cli connect`.");
    return;
  }
  const who = await request(creds.api_url, "/auth/whoami", { key: creds.api_key });
  console.log(`API:    ${creds.api_url}`);
  console.log(`MCP:    ${creds.mcp_url}`);
  console.log(`Agent:  ${creds.agent_id}`);
  console.log(`Key:    ${who.ok ? "valid" : `not valid (${who.status}) — run \`npx memro-cli connect\``}`);
  const configured = CLIENTS.filter((c) => {
    try {
      const text = fs.existsSync(c.file()) ? fs.readFileSync(c.file(), "utf8") : "";
      return text.includes(creds.mcp_url);
    } catch { return false; }
  });
  console.log(`Tools:  ${configured.map((c) => c.name).join(", ") || "none"}`);
  void opts;
}

async function disconnect(opts) {
  const targets = opts.clients ? selectClients(opts.clients) : CLIENTS;
  for (const c of targets) {
    try {
      if (c.remove()) console.log(`  ✔ Removed from ${c.name}`);
      if (c.rules && uninstallInstructions(c.rules())) console.log(`  ✔ Removed the Memro note from ${tilde(c.rules())}`);
    } catch (err) {
      console.log(`  ! ${c.name}: ${err.message}`);
    }
  }
  if (!opts.clients && fs.existsSync(CREDENTIALS())) {
    fs.rmSync(CREDENTIALS());
    console.log("  ✔ Forgot the local key. Revoke it in the dashboard → API keys.");
  }
}

const tilde = (p) => (p.startsWith(os.homedir()) ? "~" + p.slice(os.homedir().length) : p);

export async function main(argv) {
  const opts = parseArgs(argv);
  switch (opts.command) {
    case "connect": return connect(opts);
    case "status": return status(opts);
    case "disconnect": return disconnect(opts);
    case null:
    case "help": console.log(HELP); return;
    default: throw new Error(`Unknown command "${opts.command}". Try \`npx memro-cli --help\`.`);
  }
}
