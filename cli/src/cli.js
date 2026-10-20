import { execFile, execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { CLIENTS, findClient } from "./clients.js";
import { pick, pickOne, promptText } from "./picker.js";
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
  --project                  Configure the current project instead of global tools
  --scope <user|project>     Configuration scope: "user" (default) or "project"
  --client <ids>             Only these tools, comma-separated (${CLIENTS.map((c) => c.id).join(", ")})
  --key <key>                Use a specific API key (memro_sk_...) instead of saved credentials
  --yes, -y                  Don't ask: use every detected tool
  --no-instructions          Don't add the "use Memro automatically" note to
                             the tools' instructions (CLAUDE.md, AGENTS.md…)
  --staging | --local        Use staging or a local server instead of Memro Cloud
  --api <url>                Use a specific Memro API (self-hosted)
`;

export function parseArgs(argv) {
  const opts = { command: null, clients: null, yes: false, api: null, instructions: true, scope: null, key: null };
  let projectFlag = false;
  let scopeFlagValue = null;

  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--client" || a === "--clients") {
      opts.clients = (argv[++i] || "").split(",").map((s) => s.trim()).filter(Boolean);
    } else if (a === "--yes" || a === "-y") {
      opts.yes = true;
    } else if (a === "--no-instructions") {
      opts.instructions = false;
    } else if (a === "--project") {
      projectFlag = true;
    } else if (a === "--scope") {
      const val = (argv[++i] || "").toLowerCase().trim();
      if (!val || val.startsWith("-")) throw new Error("--scope requires a value ('user' or 'project')");
      if (val !== "user" && val !== "project") throw new Error(`Invalid scope "${val}". Must be "user" or "project".`);
      if (scopeFlagValue && scopeFlagValue !== val) {
        throw new Error(`Conflicting scope arguments: specified both "--scope ${scopeFlagValue}" and "--scope ${val}".`);
      }
      scopeFlagValue = val;
    } else if (a === "--key") {
      opts.key = argv[++i];
      if (!opts.key || opts.key.startsWith("-")) throw new Error("--key requires an API key (memro_sk_...)");
    } else if (a === "--staging") {
      opts.api = ENVIRONMENTS.staging;
    } else if (a === "--local") {
      opts.api = ENVIRONMENTS.local;
    } else if (a === "--api") {
      opts.api = argv[++i];
      if (!opts.api || opts.api.startsWith("-")) throw new Error("--api needs a URL, e.g. --api https://memro.example.com");
    } else if (a === "--help" || a === "-h") {
      opts.command = "help";
    } else if (!opts.command) {
      opts.command = a;
    } else {
      throw new Error(`Unexpected argument: ${a}`);
    }
  }

  if (projectFlag && scopeFlagValue === "user") {
    throw new Error('Conflicting scope arguments: cannot combine "--project" with "--scope user".');
  }

  opts.scopeExplicit = projectFlag || Boolean(scopeFlagValue);
  opts.scope = projectFlag ? "project" : (scopeFlagValue || "user");
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

export function isGitTracked(dir = process.cwd()) {
  try {
    return fs.existsSync(path.join(dir, ".git")) ||
      execFileSync("git", ["rev-parse", "--is-inside-work-tree"], { cwd: dir, stdio: "ignore" }) !== null;
  } catch {
    return false;
  }
}

export function resolveProjectRoot(startDir = process.cwd()) {
  try {
    const toplevel = execFileSync("git", ["rev-parse", "--show-toplevel"], {
      cwd: startDir,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
    if (toplevel && fs.existsSync(toplevel)) {
      return toplevel;
    }
  } catch {
    // Git not available or not inside a work tree
  }

  // Walk up checking for project boundary markers
  let curr = path.resolve(startDir);
  while (true) {
    if (
      fs.existsSync(path.join(curr, ".git")) ||
      fs.existsSync(path.join(curr, ".claude")) ||
      fs.existsSync(path.join(curr, ".cursor")) ||
      fs.existsSync(path.join(curr, "CLAUDE.md")) ||
      fs.existsSync(path.join(curr, "package.json")) ||
      fs.existsSync(path.join(curr, "go.mod")) ||
      fs.existsSync(path.join(curr, "Cargo.toml")) ||
      fs.existsSync(path.join(curr, "pyproject.toml"))
    ) {
      return curr;
    }
    const parent = path.dirname(curr);
    if (parent === curr) break;
    curr = parent;
  }

  return path.resolve(startDir);
}

export function ensureGitIgnored(filePath, projectRoot) {
  const rel = path.relative(projectRoot, filePath);
  const pattern = "/" + rel.split(path.sep).join("/");

  // Check if git considers it ignored already
  try {
    execFileSync("git", ["check-ignore", "-q", rel], { cwd: projectRoot, stdio: "ignore" });
    return { ignored: true, added: false };
  } catch {
    // Not ignored or git not found
  }

  // Check if .gitignore already has this pattern
  const gitignorePath = path.join(projectRoot, ".gitignore");
  try {
    const text = fs.existsSync(gitignorePath) ? fs.readFileSync(gitignorePath, "utf8") : "";
    const lines = text.split("\n").map((l) => l.trim());
    if (lines.includes(pattern) || lines.includes(pattern.slice(1)) || lines.includes(pattern + "/") || lines.includes(pattern.slice(1) + "/")) {
      return { ignored: true, added: false };
    }
    const dirPart = rel.split(path.sep)[0];
    if (lines.includes(`/${dirPart}/`) || lines.includes(`/${dirPart}`) || lines.includes(`${dirPart}/`) || lines.includes(dirPart)) {
      return { ignored: true, added: false };
    }

    const addition = (text && !text.endsWith("\n") ? "\n" : "") +
      `# Memro MCP configuration (contains API key)\n${pattern}\n`;
    fs.writeFileSync(gitignorePath, text + addition);
    return { ignored: true, added: true };
  } catch {
    return { ignored: false, added: false };
  }
}

function selectClients(ids, scope = "user", cwd = process.cwd()) {
  if (!ids) return CLIENTS.filter((c) => c.detect({ scope, cwd }) && c.supportsScope(scope));
  return ids.map((id) => {
    const c = findClient(id);
    if (!c) throw new Error(`Unknown client "${id}". Known: ${CLIENTS.map((x) => x.id).join(", ")}`);
    if (!c.supportsScope(scope)) throw new Error(`${c.name} does not support ${scope} scope.`);
    return c;
  });
}

/**
 * Which tools to configure: --client wins; --yes takes every detected tool;
 * otherwise the user picks in a list with detected tools pre-selected.
 * Returns null when the user cancels or nothing can be asked.
 */
async function chooseTargets(opts, cwd = process.cwd()) {
  const scope = opts.scope || "user";
  const supported = CLIENTS.filter((c) => c.supportsScope(scope));
  if (opts.clients) return selectClients(opts.clients, scope, cwd);
  const detected = new Set(supported.filter((c) => c.detect({ scope, cwd })));
  if (opts.yes) return [...detected];
  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    console.log("  Not a terminal: pass --yes (every detected tool) or --client <ids>.");
    return null;
  }
  console.log("");
  const picked = await pick(
    [
      ...supported.map((c) => ({ label: c.name, hint: detected.has(c) ? "" : "not detected", checked: detected.has(c), client: c })),
      {
        label: "Teach these tools to use Memro automatically",
        hint: scope === "project" ? "adds a note to ./CLAUDE.md, ./AGENTS.md…" : "adds a short note to CLAUDE.md, AGENTS.md…",
        checked: opts.instructions,
        option: "instructions",
      },
    ],
    { title: scope === "project" ? "Which tools should use Memro for this project?" : "Which tools should use Memro?" },
  );
  if (!picked) return null;
  opts.instructions = picked.some((p) => p.option === "instructions");
  return picked.filter((p) => p.client).map((p) => p.client);
}

async function connect(opts) {
  // If scope was not explicitly provided on CLI, ask interactively in a TTY terminal
  if (!opts.scopeExplicit && process.stdin.isTTY && process.stdout.isTTY && !opts.yes && !opts.clients) {
    console.log(`\n◆ Memro connect  (${opts.api})`);
    const scopeChoice = await pickOne(
      [
        {
          label: "Global (user-wide across all projects)",
          hint: "Connect tools everywhere on this machine (recommended for personal dev)",
          value: "user",
        },
        {
          label: "Current project (this repository only)",
          hint: "Configure ./.cursor, ./.claude.json, and ./CLAUDE.md in current directory",
          value: "project",
        },
      ],
      { title: "Where would you like to configure Memro?" },
    );
    if (!scopeChoice) {
      console.log("  Cancelled.");
      return;
    }
    opts.scope = scopeChoice.value;
  }

  const scope = opts.scope || "user";
  const cwd = scope === "project" ? resolveProjectRoot(process.cwd()) : process.cwd();

  if (opts.scopeExplicit || !process.stdin.isTTY) {
    console.log(`\n◆ Memro connect  (${opts.api})  [scope: ${scope}]`);
  }

  if (scope === "project") {
    if (cwd !== process.cwd()) {
      console.log(`  Project root: ${cwd}`);
    }
    console.log("  ℹ Project scope configures MCP and instructions for this directory only.");
    console.log("    Memories are tied to your Agent ID / API key, not this folder.");
    console.log("    To isolate memories between projects, use a separate API key or Agent ID.\n");
    if (isGitTracked(cwd)) {
      console.log("  \x1b[33m⚠\x1b[0m Note: Current directory appears to be tracked by Git.");
      console.log("    Configuration files containing your API key (such as .cursor/mcp.json, .claude.json)");
      console.log("    will be protected or added to .gitignore.\n");
    }

    // Ask about memory isolation if no key was passed and terminal is interactive
    if (!opts.key && !process.env.MEMRO_API_KEY && process.stdin.isTTY && process.stdout.isTTY && !opts.yes) {
      const memoryChoice = await pickOne(
        [
          {
            label: "Shared account memory (re-use your default credentials)",
            hint: "Claude Code, Cursor, and other tools share the same memory pool",
            value: "shared",
          },
          {
            label: "Isolated project memory (enter a dedicated API key)",
            hint: "Strict memory boundary: only tools in this project access this memory",
            value: "isolated",
          },
        ],
        { title: "Memory space for this project:" },
      );
      if (!memoryChoice) {
        console.log("  Cancelled.");
        return;
      }
      if (memoryChoice.value === "isolated") {
        const enteredKey = await promptText("  Enter project API key (memro_sk_...): ");
        if (!enteredKey) {
          console.log("  No key entered. Cancelled.");
          return;
        }
        opts.key = enteredKey;
      }
    }
  }

  // Choose first, so cancelling never creates an API key.
  const targets = await chooseTargets(opts, cwd);
  if (!targets) {
    console.log("  Nothing changed.");
    return;
  }
  if (targets.length === 0 && !opts.yes) {
    console.log("  No tools selected. Nothing changed.");
    return;
  }

  let creds = null;
  const providedKey = opts.key || process.env.MEMRO_API_KEY;
  if (providedKey) {
    const who = await request(opts.api, "/auth/whoami", { key: providedKey });
    if (!who.ok) {
      throw new Error(`The provided API key is not valid (${who.status}) on ${opts.api}`);
    }
    const agentId = who.data?.agent_id || who.data?.id || "unknown";
    const mcpUrl = opts.api === ENVIRONMENTS.prod ? "https://mcp.memro.co/mcp" : `${opts.api}/mcp`;
    creds = { api_url: opts.api, mcp_url: mcpUrl, agent_id: agentId, api_key: providedKey };
    console.log(`  Using provided API key for agent ${agentId.slice(0, 12)}…`);
  } else {
    creds = loadCredentials();
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
  }

  if (targets.length === 0) {
    console.log("\n  No supported tools found for this configuration.");
    printManual(creds);
    return;
  }
  console.log(`\n  Adding Memro to: ${targets.map((c) => c.name).join(", ")}`);

  const conn = { mcpUrl: creds.mcp_url, apiKey: creds.api_key };
  let ok = 0;
  for (const c of targets) {
    try {
      c.add(conn, { scope, cwd });
      ok++;
      const configFile = c.file(scope, cwd);
      console.log(`  \x1b[32m✔\x1b[0m ${c.name.padEnd(15)} ${formatPath(configFile, scope, cwd)}`);
      if (scope === "project" && isGitTracked(cwd)) {
        const gitStatus = ensureGitIgnored(configFile, cwd);
        if (gitStatus.added) {
          console.log(`  \x1b[32m✔\x1b[0m ${"".padEnd(15)} Added ${path.relative(cwd, configFile)} to .gitignore (protected API key)`);
        }
      }
      if (opts.instructions && c.rules) {
        const rulesFile = c.rules(scope, cwd);
        if (rulesFile) {
          const result = installInstructions(rulesFile, { scope });
          const note = result === "kept-own" ? "already tells it to use Memro; left as is" : "use Memro automatically";
          console.log(`  \x1b[32m✔\x1b[0m ${"".padEnd(15)} ${formatPath(rulesFile, scope, cwd)} (${note})`);
        }
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
  const scope = opts.scope || "user";
  const cwd = scope === "project" ? resolveProjectRoot(process.cwd()) : process.cwd();
  const creds = loadCredentials();
  const activeKey = opts.key || process.env.MEMRO_API_KEY || creds?.api_key;
  if (!creds && !activeKey) {
    console.log("Not connected. Run `npx memro-cli connect`.");
    return;
  }
  const apiUrl = creds?.api_url || opts.api;
  const who = await request(apiUrl, "/auth/whoami", { key: activeKey });
  console.log(`API:    ${apiUrl}`);
  console.log(`MCP:    ${creds?.mcp_url || (apiUrl === ENVIRONMENTS.prod ? "https://mcp.memro.co/mcp" : `${apiUrl}/mcp`)}`);
  console.log(`Agent:  ${who.data?.agent_id || creds?.agent_id || "unknown"}`);
  console.log(`Key:    ${who.ok ? "valid" : `not valid (${who.status}) — run \`npx memro-cli connect\``}`);
  console.log(`Scope:  ${scope}`);
  if (scope === "project") {
    console.log(`Root:   ${cwd}`);
  }
  const supported = CLIENTS.filter((c) => c.supportsScope(scope));
  const configured = supported.filter((c) => {
    try {
      const targetFile = c.file(scope, cwd);
      const text = fs.existsSync(targetFile) ? fs.readFileSync(targetFile, "utf8") : "";
      return text.includes("memro");
    } catch { return false; }
  });
  console.log(`Tools:  ${configured.map((c) => c.name).join(", ") || "none"}`);
}

async function disconnect(opts) {
  const scope = opts.scope || "user";
  const cwd = scope === "project" ? resolveProjectRoot(process.cwd()) : process.cwd();
  const targets = opts.clients ? selectClients(opts.clients, scope, cwd) : CLIENTS.filter((c) => c.supportsScope(scope));
  for (const c of targets) {
    try {
      if (c.remove({ scope, cwd })) console.log(`  ✔ Removed from ${c.name} (${scope})`);
      if (c.rules) {
        const rulesFile = c.rules(scope, cwd);
        if (rulesFile && uninstallInstructions(rulesFile)) {
          console.log(`  ✔ Removed the Memro note from ${formatPath(rulesFile, scope, cwd)}`);
        }
      }
    } catch (err) {
      console.log(`  ! ${c.name}: ${err.message}`);
    }
  }
  if (scope === "user" && !opts.clients && fs.existsSync(CREDENTIALS())) {
    fs.rmSync(CREDENTIALS());
    console.log("  ✔ Forgot the local key. Revoke it in the dashboard → API keys.");
  }
}

const tilde = (p) => (p.startsWith(os.homedir()) ? "~" + p.slice(os.homedir().length) : p);

function formatPath(filePath, scope, cwd) {
  if (scope === "project") {
    const rel = path.relative(cwd, filePath);
    return rel.startsWith(".") ? rel : `./${rel}`;
  }
  return tilde(filePath);
}

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
