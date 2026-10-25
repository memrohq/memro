import { execFile, execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { CLIENTS, findClient, withSession } from "./clients.js";
import { pick, pickOne, promptText } from "./picker.js";
import { installInstructions, uninstallInstructions } from "./instructions.js";
import * as ui from "./ui.js";

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

  ui.info("Sign in to Memro");
  ui.line(`Your code: ${ui.c.bold(user_code)}`);
  ui.line(ui.c.dim(`Opening ${verification_uri_complete}`));
  ui.line(ui.c.dim("Approve it there. If the browser didn't open, visit the link."));
  openBrowser(verification_uri_complete);

  const deadline = Date.now() + (expires_in || 600) * 1000;
  let wait = (interval || 3) * 1000;
  process.stdout.write(`${ui.bar()}  ${ui.c.dim("Waiting for approval…")}`);
  while (Date.now() < deadline) {
    await sleep(wait);
    let res;
    try {
      res = await request(api, "/auth/device/token", { method: "POST", json: { device_code } });
    } catch {
      continue; // a dropped connection: keep polling until the code expires
    }
    if (res.ok) {
      process.stdout.write(` ${ui.c.green("approved")}\n`);
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

/** The home folder or the filesystem root: never a project. */
export function isHomeOrRoot(dir) {
  // Compare real paths: git reports /private/var/… where HOME says /var/…,
  // and C:/Users/name where Windows may give the 8.3 C:\Users\NAME~1. The
  // native realpath expands short names; Windows paths ignore case.
  const real = (p) => {
    let r;
    try { r = fs.realpathSync.native(path.resolve(p)); } catch { r = path.resolve(p); }
    return process.platform === "win32" ? r.toLowerCase() : r;
  };
  const d = real(dir);
  return d === real(os.homedir()) || d === real(path.parse(d).root);
}

export function resolveProjectRoot(startDir = process.cwd()) {
  try {
    const toplevel = execFileSync("git", ["rev-parse", "--show-toplevel"], {
      cwd: startDir,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
    if (toplevel && fs.existsSync(toplevel) && !isHomeOrRoot(toplevel)) {
      return toplevel;
    }
  } catch {
    // Git not available or not inside a work tree
  }

  // Walk up checking for project boundary markers
  let curr = path.resolve(startDir);
  const homeDir = path.resolve(os.homedir());
  while (true) {
    // Stop if we reach user's home directory: home folder is never a project root
    if (curr === homeDir) break;

    if (
      fs.existsSync(path.join(curr, ".git")) ||
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
  if (!interactive()) {
    ui.warn("Not a terminal: pass --yes (every detected tool) or --client <ids>.");
    return null;
  }
  // Installed tools first, then the rest, each in catalogue order.
  const ordered = [...supported.filter((c) => detected.has(c)), ...supported.filter((c) => !detected.has(c))];
  const picked = await pick(
    [
      ...ordered.map((c) => ({ label: c.name, hint: detected.has(c) ? "" : "not installed", checked: detected.has(c), client: c })),
      {
        label: "Teach tools to use Memro automatically",
        hint: "notes in CLAUDE.md, AGENTS.md…",
        summary: "+ instructions",
        checked: opts.instructions,
        option: "instructions",
      },
    ],
    { title: scope === "project" ? "Which tools should use Memro in this project?" : "Which tools should use Memro?" },
  );
  if (!picked) return null;
  opts.instructions = picked.some((p) => p.option === "instructions");
  return picked.filter((p) => p.client).map((p) => p.client);
}

const interactive = () => Boolean(process.stdin.isTTY && process.stdout.isTTY);
const host = (url) => url.replace(/^https?:\/\//, "");

async function connect(opts) {
  ui.intro("Memro connect", host(opts.api));
  ui.gap();

  // Ask where to set up unless a flag already said so.
  if (!opts.scopeExplicit && interactive() && !opts.yes && !opts.clients) {
    const here = resolveProjectRoot(process.cwd());
    const scopeChoice = await pickOne(
      [
        { label: "All my projects", hint: "Adds Memro to your tools globally", summary: "All projects (global)", value: "user" },
        {
          label: "Only this project",
          hint: isHomeOrRoot(here) ? "Run this inside a project folder to use it" : tilde(here),
          summary: `This project · ${tilde(here)}`,
          value: "project",
        },
      ],
      { title: "Where should Memro be set up?" },
    );
    if (!scopeChoice) return ui.cancelled();
    opts.scope = scopeChoice.value;
  }

  const scope = opts.scope || "user";
  const cwd = scope === "project" ? resolveProjectRoot(process.cwd()) : process.cwd();

  if (scope === "project") {
    if (isHomeOrRoot(cwd)) {
      ui.warn(`${tilde(cwd)} is not a project folder.`);
      ui.line("Run this inside a project, or choose “All my projects”.");
      return ui.outro("Nothing changed.");
    }
    if (opts.scopeExplicit) ui.answered("Project", tilde(cwd));

    // Ask about memory isolation if no key was passed and terminal is interactive
    if (!opts.key && !process.env.MEMRO_API_KEY && interactive() && !opts.yes) {
      const memoryChoice = await pickOne(
        [
          {
            label: "Shared with my account",
            hint: "Same memory as your other projects and tools",
            summary: "Shared with your account",
            value: "shared",
          },
          {
            label: "Separate for this project",
            hint: "Use a dedicated API key; only this project sees its memory",
            summary: "Separate (project API key)",
            value: "isolated",
          },
        ],
        { title: "Which memory should this project use?" },
      );
      if (!memoryChoice) return ui.cancelled();
      if (memoryChoice.value === "isolated") {
        const enteredKey = await promptText(`${ui.bar()}  Project API key (memro_sk_…): `);
        if (!enteredKey) return ui.cancelled("No key entered. Nothing changed.");
        opts.key = enteredKey;
        ui.gap();
      }
    }
  }

  // Choose first, so cancelling never creates an API key.
  const targets = await chooseTargets(opts, cwd);
  if (!targets) return ui.cancelled();
  if (targets.length === 0 && !opts.yes) return ui.cancelled("No tools selected. Nothing changed.");

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
    ui.info(`Using the project key ${ui.c.dim(`(agent ${agentId.slice(0, 12)}…)`)}`);
  } else {
    creds = loadCredentials();
    if (creds?.api_url === opts.api) {
      const who = await request(opts.api, "/auth/whoami", { key: creds.api_key });
      if (who.status === 401 || who.status === 403) creds = null;
      // Anything else (an outage, a rate limit) says nothing about the key;
      // signing in again would mint a second one.
      else if (!who.ok) throw new Error(`Memro API returned ${who.status}. Try again in a moment.`);
      else ui.info(`Signed in ${ui.c.dim(`(agent ${creds.agent_id.slice(0, 12)}…)`)}`);
    } else {
      creds = null;
    }
    if (!creds) {
      const res = await deviceLogin(opts.api);
      creds = { api_url: opts.api, mcp_url: res.mcp_url, agent_id: res.agent_id, api_key: res.api_key };
      saveCredentials(creds);
    }
  }
  ui.gap();

  if (targets.length === 0) {
    ui.warn("No supported tools found for this setup.");
    printManual(creds);
    return ui.outro("Nothing else changed.");
  }

  ui.info(scope === "project" ? `Adding Memro to this project` : "Adding Memro to your tools");
  const conn = { mcpUrl: creds.mcp_url, apiKey: creds.api_key };
  const width = Math.max(...targets.map((c) => c.name.length)) + 2;
  const gitTracked = scope === "project" && isGitTracked(cwd);
  let ok = 0;
  for (const c of targets) {
    try {
      c.add({ ...conn, mcpUrl: withSession(conn.mcpUrl, c.id) }, { scope, cwd });
      ok++;
      const configFile = c.file(scope, cwd);
      let note = "";
      if (gitTracked && ensureGitIgnored(configFile, cwd).added) note = "added to .gitignore (holds your key)";
      ui.ok(c.name.padEnd(width), `${formatPath(configFile, scope, cwd)}${note ? "  " + note : ""}`);
      if (opts.instructions && c.rules) {
        const rulesFile = c.rules(scope, cwd);
        if (rulesFile) {
          const result = installInstructions(rulesFile, { scope, frontmatter: c.rulesFrontmatter });
          const what = result === "kept-own" ? "already mentions Memro" : "use Memro automatically";
          ui.ok("".padEnd(width), `${formatPath(rulesFile, scope, cwd)}  ${what}`);
        }
      }
    } catch (err) {
      ui.fail(c.name.padEnd(width), err.message);
    }
  }
  ui.gap();
  ui.outro(
    ok ? `Done. Restart ${ok === 1 ? "the tool" : "your tools"} to load Memro.` : "Nothing was configured.",
    "Manage or revoke the key in the dashboard → API keys.",
  );
}

function printManual(creds) {
  ui.line("Add this MCP server to your tool by hand:");
  ui.line(`URL     ${creds.mcp_url}`);
  ui.line("Header  Authorization: Bearer <the key in ~/.memro/credentials.json>");
  ui.gap();
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
        if (rulesFile && uninstallInstructions(rulesFile, { frontmatter: c.rulesFrontmatter })) {
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
    return rel.startsWith("..") || path.isAbsolute(rel) ? tilde(filePath) : `./${rel}`;
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
