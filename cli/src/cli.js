import { execFile, execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { CLIENTS, HOOK_TARGETS, KEY_ENV, findClient, installHooks, onPath, removeHooks, withSession } from "./clients.js";
import { RULES_HEADER, addRules, brokenRules, herdrOf, readRules, repoOf, rulesFile, rulesFor, untagged } from "./hook.js";
import { pick, pickOne, promptText } from "./picker.js";
import { codexTrusts, diagnose } from "./doctor.js";
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
  npx memro-cli init         Keep this repo's rules in .memro/rules.md, no account:
                             add the hooks that save the rules you state, show them
                             to every agent and warn when an edit breaks one
  npx memro-cli connect      Sign in and add Memro to Claude Code, Codex, Cursor,
                             Antigravity, OpenCode, Gemini CLI, VS Code, Windsurf
                             and Claude Desktop
  npx memro-cli status       Show the connected account and configured tools
  npx memro-cli doctor       Check each tool: which config it uses, its key, and
                             whether the Memro server accepts it
  npx memro-cli disconnect   Remove Memro from your tools and forget the key
  npx memro-cli agents       Show your agents: what each is doing and the files it holds
  npx memro-cli send <agent> <message>
                             Message an agent (a name from \`agents\`); Claude Code
                             reads it at its next prompt or when it stops
  npx memro-cli rules        List this repo's rules (.memro/rules.md), the ones every
                             agent here is shown at each prompt
  npx memro-cli rules edit <n> <text>
                             Replace rule <n> (a number from \`rules\`) with <text>
  npx memro-cli rules delete <n>
                             Delete rule <n>, so agents stop seeing it
  npx memro-cli rules import Move rules saved in your Memro account into .memro/rules.md
  npx memro-cli land         From a git worktree: rebase this branch on the main
                             checkout's branch and fast-forward it, one agent at
                             a time, showing what landed before you

Options
  --project                  Configure the current project instead of global tools
  --scope <user|project>     Configuration scope: "user" (default) or "project"
  --client <ids>             Only these tools, comma-separated (${CLIENTS.map((c) => c.id).join(", ")})
  --key <key>                Use a specific API key (memro_sk_...) instead of saved credentials
  --env-key                  Claude Code and Codex read the key from $MEMRO_API_KEY
                             instead of storing it in their config (safe to commit)
  --yes, -y                  Don't ask: use every detected tool
  --no-instructions          Don't add the "use Memro automatically" note to
                             the tools' instructions (CLAUDE.md, AGENTS.md…)
  --no-hooks                 Don't add the hooks that claim each file before it is
                             edited, so two agents never edit one file (Claude Code,
                             Codex, Cursor, Gemini CLI, OpenCode)
  --staging | --local        Use staging or a local server instead of Memro Cloud
  --api <url>                Use a specific Memro API (self-hosted)
  --from <agent>             With send: who the message is from (default: you)
`;

export function parseArgs(argv) {
  const opts = { command: null, clients: null, yes: false, api: null, instructions: true, hooks: true, scope: null, key: null, envKey: false, args: [], from: null };
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
    } else if (a === "--from") {
      opts.from = argv[++i];
      if (!opts.from || opts.from.startsWith("-")) throw new Error("--from needs an agent name");
    } else if (a === "--no-hooks") {
      opts.hooks = false;
    } else if (a === "--env-key") {
      opts.envKey = true;
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
    } else if (opts.command === "send" || opts.command === "rules") {
      opts.args.push(a);
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

/** Rules without an account: the hooks in every detected tool that has them,
 * and the repo's .memro/rules.md. */
async function init(opts) {
  const cwd = process.cwd();
  const scope = opts.scope || "user";
  const targets = (opts.clients ? selectClients(opts.clients, scope, cwd) : CLIENTS.filter((c) => c.supportsScope(scope) && c.detect({ scope, cwd })))
    .filter((c) => HOOK_TARGETS[c.id]);
  ui.intro("Memro init", "rules for your agents, no account");
  ui.gap();
  if (!targets.length) return ui.outro("No tool with hooks found (Claude Code, Codex, Cursor, Gemini CLI, OpenCode).");
  const width = Math.max(...targets.map((c) => c.name.length)) + 2;
  for (const c of targets) {
    try {
      ui.ok(c.name.padEnd(width), formatPath(installHooks({ scope, cwd, tool: c.id }), scope, cwd));
    } catch (err) {
      ui.fail(c.name.padEnd(width), err.message);
    }
  }
  const file = rulesFile(cwd);
  if (!fs.existsSync(file)) {
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, RULES_HEADER);
  }
  ui.gap();
  ui.outro(
    `Rules go in ${path.relative(cwd, file)}. Tell an agent one ("never…", "always…"), or add a line yourself.`,
    "Put the thing to avoid in backticks (never use `Intl` in cart code) and agents are warned when an edit uses it.",
    "Restart your tools to load the hooks. Commit the file to share the rules.",
    "★ Find Memro useful? A star helps others find it: https://github.com/memrohq/memro",
  );
}

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
        // Without a key in hand this used to cancel the whole setup, so say where
        // to get one and let Enter fall back to shared memory instead.
        ui.line(ui.c.dim("Create a key at app.memro.co → API keys, or press Enter to use shared memory."));
        const enteredKey = await promptText(`${ui.bar()}  Project API key (memro_sk_…): `);
        if (enteredKey) opts.key = enteredKey;
        else ui.answered("Memory", "Shared with your account");
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
  let usedEnvKey = false;
  for (const c of targets) {
    try {
      const envKey = Boolean(opts.envKey && c.supportsEnvKey);
      c.add({ ...conn, envKey, mcpUrl: withSession(conn.mcpUrl, c.id) }, { scope, cwd });
      ok++;
      usedEnvKey ||= envKey;
      const configFile = c.file(scope, cwd);
      let note = envKey ? `reads $${KEY_ENV}` : "";
      if (!envKey && gitTracked && ensureGitIgnored(configFile, cwd).added) note = "added to .gitignore (holds your key)";
      ui.ok(c.name.padEnd(width), `${formatPath(configFile, scope, cwd)}${note ? "  " + note : ""}`);
      if (c.id === "codex" && scope === "project" && !codexTrusts(c.file("user", cwd), cwd)) {
        ui.line(ui.c.dim(`${"".padEnd(width + 3)}Codex reads it once you open Codex in this folder and trust it`));
      }
      if (HOOK_TARGETS[c.id] && opts.hooks) {
        // The hook reads $MEMRO_API_KEY or the saved key, never a --key passed here.
        if (opts.key && !envKey) ui.line(ui.c.dim(`${"".padEnd(width + 3)}no edit hooks: they can't read a --key; add --env-key`));
        else {
          const file = installHooks({ scope, cwd, tool: c.id });
          // Codex skips a new hook until the user trusts it.
          ui.ok("".padEnd(width), `${formatPath(file, scope, cwd)}  claims each file before editing${c.id === "codex" ? " · trust it once in Codex: /hooks" : ""}`);
        }
      }
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
  if (usedEnvKey) {
    ui.line(`Set ${KEY_ENV} to your Memro key in your shell profile before starting those tools`);
    ui.line(ui.c.dim(`(the key is in ${tilde(CREDENTIALS())}, or create one in the dashboard → API keys).`));
    ui.gap();
  }
  ui.outro(
    ok ? `Done. Restart ${ok === 1 ? "the tool" : "your tools"} to load Memro.` : "Nothing was configured.",
    "Manage or revoke the key in the dashboard → API keys.",
    ...(ok ? ["★ Find Memro useful? A star helps others find it: https://github.com/memrohq/memro"] : []),
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

/** The key and API for account commands: --key / $MEMRO_API_KEY, else the saved sign-in. */
function account(opts) {
  const creds = loadCredentials();
  const key = opts.key || process.env.MEMRO_API_KEY || creds?.api_key;
  if (!key) throw new Error("Not connected. Run `npx memro-cli connect`.");
  return { api: creds?.api_url || opts.api, key };
}

const ago = (t) => {
  const s = Math.max(0, Math.round((Date.now() - new Date(t)) / 1000));
  return s < 60 ? `${s}s ago` : s < 3600 ? `${Math.round(s / 60)}m ago` : `${Math.round(s / 3600)}h ago`;
};

/** Every agent session with a status or a claim, like herdr's sidebar. */
async function agents(opts) {
  const { api, key } = account(opts);
  const res = await request(api, "/coordination/active", { key });
  if (!res.ok) throw new Error(`Memro API returned ${res.status}`);
  const bySession = new Map();
  const row = (s) => bySession.get(s) || bySession.set(s, { status: null, files: [] }).get(s);
  for (const c of res.data?.claims || []) {
    if (c.resource.startsWith("agents/")) row(c.session_id).status = c;
    else row(c.session_id).files.push(c.resource);
  }
  if (!bySession.size) return console.log("No agents are active right now.");
  const width = Math.max(...[...bySession.keys()].map((s) => s.length)) + 2;
  for (const [session, { status: st, files }] of bySession) {
    console.log(`${session.padEnd(width)}${st ? `${untagged(st.reason)}  ${ui.c.dim(ago(st.heartbeat_at))}` : ui.c.dim("no status (MCP tools only)")}`);
    for (const f of files) console.log(ui.c.dim(`${"".padEnd(width)}holds ${f}`));
  }
}

async function send(opts) {
  const [to, ...words] = opts.args;
  const message = words.join(" ").trim();
  if (!to || !message) throw new Error("Usage: npx memro-cli send <agent> <message>  (names from `npx memro-cli agents`)");
  const { api, key } = account(opts);
  const res = await request(api, "/coordination/intent", {
    method: "POST",
    key,
    json: { resource: `inbox/${to}`, intent: message, session_id: opts.from || "user", ttl_seconds: 7200 },
  });
  if (!res.ok) throw new Error(`Couldn't send: ${res.data?.message || res.status}`);
  if (await wakeInHerdr(api, key, to)) return console.log(`Sent to ${to}, and woke it in its herdr pane.`);
  console.log(`Sent to ${to}. It reads it at its next prompt or when it stops (within 2 hours).`);
}

/** List, edit or delete the repo's rules in .memro/rules.md, the ones every
 * agent here is shown at each prompt, and move rules saved in the Memro
 * account before the file existed into it. */
async function rules(opts) {
  const [action, n, ...words] = opts.args;
  const file = rulesFile(process.cwd());
  const shown = path.relative(process.cwd(), file);
  const local = readRules(process.cwd());
  const repo = repoOf();
  let cfg = null;
  try { const { api, key } = account(opts); cfg = { api, apiKey: key }; } catch { /* no account: the file alone */ }
  const older = cfg ? await rulesFor(cfg, repo).catch(() => []) : [];
  const unmoved = older.filter((r) => !local.includes(r.text));

  if (!action) {
    if (!local.length && !unmoved.length) return console.log(`No rules yet. Tell your agent one ("never…", "always…") and the hook adds it to ${shown}.`);
    if (local.length) console.log(ui.c.dim(`Rules in ${shown}:`));
    // Rules that name a term in backticks are checked on every edit.
    local.forEach((r, i) => console.log(`${String(i + 1).padStart(3)}. ${r}${brokenRules([r], r.match(/`([^`]+)`/)?.[1] || "").length ? ui.c.dim("  (checked on edits)") : ""}`));
    if (unmoved.length) {
      console.log(ui.c.dim(`\nSaved in your Memro account before ${shown} (agents still see them). Move them into the file with \`npx memro-cli rules import\`:`));
      for (const r of unmoved) console.log(`     ${r.text}`);
    }
    return;
  }
  if (action === "import") {
    if (!cfg) account(opts); // throws "Not connected…"
    if (!older.length) return console.log("Nothing to import: your Memro account holds no rules for this repo.");
    addRules(process.cwd(), older.map((r) => r.text));
    for (const r of older) {
      const del = await request(cfg.api, `/memory/${r.id}`, { method: "DELETE", key: cfg.apiKey });
      if (!del.ok) throw new Error(`Copied the rules into ${shown}, but couldn't remove one from your account: ${del.data?.message || del.status}`);
    }
    return console.log(`Moved ${older.length} rule${older.length === 1 ? "" : "s"} into ${shown}. Commit it to share them.`);
  }
  if (action !== "edit" && action !== "delete") throw new Error(`Unknown rules action "${action}". Use \`rules\`, \`rules edit <n> <text>\`, \`rules delete <n>\` or \`rules import\`.`);
  const rule = local[Number(n) - 1];
  if (!/^\d+$/.test(n || "") || !rule) throw new Error(`No rule ${n ?? ""} in ${shown}. Run \`npx memro-cli rules\` for the numbers.`);
  const text = words.join(" ").replace(/\s+/g, " ").trim();
  if (action === "edit" && !text) throw new Error("Usage: npx memro-cli rules edit <n> <new text>");
  const lines = fs.readFileSync(file, "utf8").split("\n");
  const at = lines.findIndex((l) => l.startsWith("- ") && l.slice(2).trim() === rule);
  if (action === "edit") lines[at] = `- ${text}`;
  else lines.splice(at, 1);
  fs.writeFileSync(file, lines.join("\n"));
  console.log(action === "edit" ? `Rule ${n} is now: ${text}` : `Deleted: ${rule}`);
}

/** Land this worktree's branch on the branch the main checkout has out, one
 * agent at a time: worktrees keep agents apart while they work, but every
 * branch meets at merge time (the launch benchmark: 4 of 5 branches
 * conflicted). Holding `land/<repo>` means each agent rebases onto what landed
 * before it and sees those commits when it has to resolve a conflict. */
async function land(opts) {
  const { api, key } = account(opts);
  const git = (...args) => execFileSync("git", args, { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
  const top = git("rev-parse", "--show-toplevel");
  const mainDir = git("worktree", "list", "--porcelain").match(/^worktree (.+)$/m)[1];
  if (path.resolve(mainDir) === path.resolve(top)) throw new Error("Run `land` from a worktree, not the main checkout.");
  if (fs.existsSync(git("rev-parse", "--git-path", "rebase-merge"))) {
    throw new Error("A rebase is still in progress: resolve the conflicts, `git add` the files, run `git -c core.editor=true rebase --continue`, then run `land` again.");
  }
  if (git("status", "--porcelain")) throw new Error("Commit your changes first (git add <files> && git commit -m \"...\"), then run `land` again.");
  const branch = git("rev-parse", "--abbrev-ref", "HEAD");
  const target = git("-C", mainDir, "rev-parse", "--abbrev-ref", "HEAD");
  const resource = `land/${repoOf(mainDir)}`; // every worktree names it alike, origin or not
  const session = opts.from || `land:${branch}`;

  // Below the 2-minute limit agents' shell tools put on one command.
  const until = Date.now() + Number(process.env.MEMRO_LAND_WAIT_SECONDS ?? 100) * 1000;
  let r;
  for (;;) {
    const res = await request(api, "/coordination/claim", {
      method: "POST", key, json: { resource, session_id: session, intent: `landing ${branch} on ${target}`, ttl_seconds: 600 },
    });
    if (!res.ok) throw new Error(`Memro API returned ${res.status}`);
    r = res.data;
    if (r.granted || Date.now() > until) break;
    await new Promise((done) => setTimeout(done, 3000));
  }
  if (!r.granted) {
    process.exitCode = 2;
    return console.log(`${r.holder_session || "Another agent"} is still landing (${untagged(r.intent || "")}). Run \`land\` again in a minute; your branch is untouched.`);
  }
  if (r.handoff_summary) console.log(`Landed before you: ${r.handoff_summary}`);

  const base = git("merge-base", branch, target);
  try {
    git("rebase", target);
  } catch {
    const files = git("diff", "--name-only", "--diff-filter=U").split("\n").filter(Boolean);
    process.exitCode = 1;
    // The lease stays ours (10 minutes, renewed when this runs again), so nobody lands under us while we resolve.
    console.log(`Conflicts rebasing ${branch} onto ${target} in: ${files.join(", ")}`);
    console.log(`What landed on ${target} since you branched, in those files:`);
    console.log(git("log", "--format=  %h %s", `${base}..${target}`, "--", ...files) || "  (nothing listed)");
    return console.log("Keep both their change and yours in each file, `git add` it, run `git -c core.editor=true rebase --continue`, " +
      "run the tests, then run `land` again.");
  }
  git("-C", mainDir, "merge", "--ff-only", branch);
  const summary = `${session} landed ${branch}: ${git("log", "-1", "--format=%s")}`;
  await request(api, "/coordination/handoff", { method: "POST", key, json: { resource, session_id: session, summary } });
  console.log(`Landed ${branch} on ${target}.`);
}

/** An idle agent runs no hooks, so it would only see the message at its next
 * prompt. If it sits in a herdr pane on this machine, prompt it there; its
 * prompt hook then shows it the message. */
export async function wakeInHerdr(api, key, to, run = execFileSync) {
  const res = await request(api, `/coordination/active?resource=${encodeURIComponent(`agents/${to}`)}`, { key });
  const status = res.data?.claims?.find((c) => c.resource === `agents/${to}`);
  const m = herdrOf(status?.reason).match(/^herdr (\S+) (\S+)@(.+)$/);
  if (!m || m[3] !== os.hostname() || !status.reason.startsWith("idle") || !onPath("herdr")) return false;
  try {
    run("herdr", ["--session", m[1], "agent", "prompt", m[2],
      "You have a new message from another agent in Memro; it is shown to you as context. Handle it if it fits the work you were given, then stop."],
      { stdio: "ignore", timeout: 15_000 });
    return true;
  } catch {
    return false; // the pane closed or the agent is blocked: the message still waits in its inbox
  }
}

async function doctor(opts) {
  const root = resolveProjectRoot(process.cwd());
  const inProject = !isHomeOrRoot(root);
  const cwd = inProject ? root : process.cwd();
  ui.intro("Memro doctor", inProject ? tilde(cwd) : "global setup");
  ui.gap();

  const creds = loadCredentials();
  if (creds) {
    const who = await request(creds.api_url, "/auth/whoami", { key: creds.api_key }).catch((err) => ({ error: err.message }));
    const state = who.error || (who.ok ? "key valid" : `key not valid (${who.status}): run \`npx memro-cli connect\``);
    ui.line(`Signed in  ${ui.c.dim(`agent ${creds.agent_id.slice(0, 12)}… · ${host(creds.api_url)} ·`)} ${state}`);
  } else {
    ui.line(`Signed in  ${ui.c.dim("no saved key in ~/.memro/credentials.json")}`);
  }
  ui.gap();

  const clients = opts.clients ? selectClients(opts.clients) : CLIENTS;
  const rows = await diagnose(clients, { cwd, inProject });
  if (!rows.length) {
    ui.warn("No tools have Memro, and none of the supported tools were found.");
    return ui.outro("Run `npx memro-cli connect` to set one up.");
  }
  const width = Math.max(...rows.map((r) => r.client.name.length)) + 2;
  for (const r of rows) {
    const name = r.client.name.padEnd(width);
    if (r.problem === "not connected") {
      ui.fail(name, `installed, Memro not added · npx memro-cli connect --client ${r.client.id}`);
      continue;
    }
    const where = `${r.scope === "user" ? "global" : r.scope} ${r.scope === "local" ? tilde(r.file) : formatPath(r.file, r.scope === "user" ? "user" : "project", cwd)}`;
    const detail = [where, `key ${r.keySource}`, r.tools != null && `${r.tools} tools`, r.problem].filter(Boolean).join(" · ");
    (r.problem ? ui.fail : ui.ok)(name, detail);
    for (const f of r.overridden) ui.line(ui.c.dim(`${"".padEnd(width + 3)}overrides ${tilde(f)}`));
    if (r.instructions === false) ui.line(ui.c.dim(`${"".padEnd(width + 3)}no Memro note in its instructions: it uses Memro only when asked`));
  }
  ui.gap();
  const broken = rows.filter((r) => r.problem && r.problem !== "not connected").length;
  process.exitCode = broken ? 1 : 0;
  ui.outro(
    broken ? `${broken} ${broken === 1 ? "tool needs" : "tools need"} attention.` : "Everything checks out.",
    "This reads each tool's config and asks the Memro server directly; restart a tool after changing its config.",
  );
}

async function disconnect(opts) {
  const scope = opts.scope || "user";
  const cwd = scope === "project" ? resolveProjectRoot(process.cwd()) : process.cwd();
  const targets = opts.clients ? selectClients(opts.clients, scope, cwd) : CLIENTS.filter((c) => c.supportsScope(scope));
  for (const c of targets) {
    try {
      if (c.remove({ scope, cwd })) console.log(`  ✔ Removed from ${c.name} (${scope})`);
      if (HOOK_TARGETS[c.id] && removeHooks({ scope, cwd, tool: c.id })) console.log(`  ✔ Removed the ${c.name} edit hooks (${scope})`);
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
    case "init": return init(opts);
    case "connect": return connect(opts);
    case "status": return status(opts);
    case "doctor": return doctor(opts);
    case "disconnect": return disconnect(opts);
    case "agents": return agents(opts);
    case "send": return send(opts);
    case "rules": return rules(opts);
    case "land": return land(opts);
    case null:
    case "help": console.log(HELP); return;
    default: throw new Error(`Unknown command "${opts.command}". Try \`npx memro-cli --help\`.`);
  }
}
