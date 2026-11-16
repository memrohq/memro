// MCP clients `memro connect` knows how to configure. Each entry says where
// the client keeps its MCP config, how to tell it is installed, and how to add
// or remove the Memro server in that file's format.

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

export const SERVER_NAME = "memro";
export const MCP_REMOTE_VERSION = "0.15.1";

const home = () => os.homedir();
const appData = () => process.env.APPDATA || path.join(home(), "AppData", "Roaming");

/** Per-OS application-support directory (VS Code, Claude Desktop). */
function appSupport(...parts) {
  if (process.platform === "darwin") return path.join(home(), "Library", "Application Support", ...parts);
  if (process.platform === "win32") return path.join(appData(), ...parts);
  return path.join(process.env.XDG_CONFIG_HOME || path.join(home(), ".config"), ...parts);
}

const bearer = (key) => ({ Authorization: `Bearer ${key}` });

/** With --env-key, tools that can read the key from the environment get a
 * reference to this variable instead of the key, so the file is safe to commit. */
export const KEY_ENV = "MEMRO_API_KEY";
const authHeader = ({ apiKey, envKey }) => (envKey ? `Bearer \${${KEY_ENV}}` : `Bearer ${apiKey}`);

/** The MCP URL for one tool, tagged with ?session_id=<tool id>. The hosted MCP
 * is stateless, so this is how it tells tools on the same key apart when they
 * claim work (memro ADR 0019): each tool is its own lease holder. */
export function withSession(mcpUrl, sessionId) {
  const u = new URL(mcpUrl);
  if (!u.searchParams.has("session_id")) u.searchParams.set("session_id", sessionId);
  return u.toString();
}

// --- JSON helpers ----------------------------------------------------------------

/** Read a JSON config. Missing → {}. Unparseable (e.g. JSONC) → throws. */
export function readJSON(file) {
  if (!fs.existsSync(file)) return {};
  const text = fs.readFileSync(file, "utf8");
  if (!text.trim()) return {};
  try {
    return JSON.parse(text);
  } catch {
    throw new Error(`${file} is not plain JSON (comments?); add Memro to it by hand`);
  }
}

/** Write a config, keeping a one-time backup of the original and restricting
 * permissions, since the file now holds an API key. The new text goes to a
 * temporary file that replaces the config in one rename, so a crash or a
 * concurrent reader never sees a half-written file. */
export function writeFileSafely(file, text) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  if (fs.existsSync(file) && !fs.existsSync(`${file}.memro-backup`)) {
    fs.copyFileSync(file, `${file}.memro-backup`);
    try { fs.chmodSync(`${file}.memro-backup`, 0o600); } catch { /* not supported on this FS */ }
  }
  const tmp = `${file}.memro-tmp-${process.pid}`;
  try {
    fs.writeFileSync(tmp, text, { mode: 0o600 });
    try { fs.chmodSync(tmp, 0o600); } catch { /* not supported on this FS */ }
    fs.renameSync(tmp, file);
  } finally {
    fs.rmSync(tmp, { force: true });
  }
}

const writeJSON = (file, data) => writeFileSafely(file, JSON.stringify(data, null, 2) + "\n");

/** A client whose config is a JSON object with servers under `key`. */
function jsonClient({
  id,
  name,
  aliases = [],
  file,
  detect,
  key = "mcpServers",
  entry,
  rules,
  rulesFrontmatter,
  legacyFiles = () => [],
  supportsScope = () => true,
}) {
  /** Drop our server from a file an older memro-cli wrote to by mistake;
   * delete the file if nothing else is left in it. */
  const removeFrom = (target) => {
    if (!fs.existsSync(target)) return;
    let cfg;
    try { cfg = readJSON(target); } catch { return; }
    if (!cfg[key]?.[SERVER_NAME]) return;
    delete cfg[key][SERVER_NAME];
    if (!Object.keys(cfg[key]).length) delete cfg[key];
    if (Object.keys(cfg).length) writeJSON(target, cfg);
    else fs.rmSync(target);
  };
  return {
    id,
    name,
    aliases,
    supportsScope,
    rulesFrontmatter,
    file: (scope = "user", cwd = process.cwd()) => file(scope, cwd),
    detect: (opts = {}) => {
      const scope = typeof opts === "string" ? opts : (opts?.scope || "user");
      const cwd = opts?.cwd || process.cwd();
      return detect ? detect(scope, cwd) : false;
    },
    rules: rules ? (scope = "user", cwd = process.cwd()) => rules(scope, cwd) : () => null,
    add(conn, { scope = "user", cwd = process.cwd() } = {}) {
      const target = file(scope, cwd);
      const cfg = readJSON(target);
      cfg[key] = { ...(cfg[key] || {}), [SERVER_NAME]: entry(conn) };
      writeJSON(target, cfg);
      for (const old of legacyFiles(scope, cwd)) if (old !== target) removeFrom(old);
    },
    remove({ scope = "user", cwd = process.cwd() } = {}) {
      const target = file(scope, cwd);
      if (!fs.existsSync(target)) return false;
      const cfg = readJSON(target);
      if (!cfg[key]?.[SERVER_NAME]) return false;
      delete cfg[key][SERVER_NAME];
      writeJSON(target, cfg);
      return true;
    },
  };
}

const exists = (...p) => fs.existsSync(path.join(...p));

/** An executable called `cmd` on PATH (checked on disk, nothing is run). */
export function onPath(cmd) {
  const exts = process.platform === "win32" ? (process.env.PATHEXT || ".EXE;.CMD;.BAT").split(";") : [""];
  return (process.env.PATH || "").split(path.delimiter).filter(Boolean)
    .some((dir) => exts.some((ext) => fs.existsSync(path.join(dir, cmd + ext))));
}

/** A desktop app is installed: its app bundle or program folder exists, or
 * its command is on PATH. A leftover settings folder does not count. */
function appInstalled({ mac, win, linux = [], cmd }) {
  if (cmd && onPath(cmd)) return true;
  if (process.platform === "darwin") return ["/Applications", path.join(home(), "Applications")].some((d) => exists(d, `${mac}.app`));
  if (process.platform === "win32") return exists(process.env.LOCALAPPDATA || path.join(home(), "AppData", "Local"), "Programs", win);
  return linux.some((p) => fs.existsSync(p));
}

// --- Codex (TOML) ------------------------------------------------------------------

const CODEX_HEADER = `[mcp_servers.${SERVER_NAME}]`;

/** Remove the [mcp_servers.memro] table (and its sub-tables) from TOML text. */
export function stripCodexServer(text) {
  const lines = text.split("\n");
  const out = [];
  let skipping = false;
  for (const line of lines) {
    const t = line.trim();
    if (t.startsWith("[")) {
      skipping = t === CODEX_HEADER || t.startsWith(`[mcp_servers.${SERVER_NAME}.`);
    }
    if (!skipping) out.push(line);
  }
  return out.join("\n").replace(/\n{3,}/g, "\n\n").trimEnd();
}

const tomlString = (s) => JSON.stringify(s); // JSON strings are valid TOML basic strings

const codex = {
  id: "codex",
  name: "Codex",
  aliases: [],
  supportsScope: () => true,
  file: (scope = "user", cwd = process.cwd()) => {
    return scope === "project"
      ? path.join(cwd, ".codex", "config.toml")
      : path.join(process.env.CODEX_HOME || path.join(home(), ".codex"), "config.toml");
  },
  detect: (opts = {}) => {
    const scope = typeof opts === "string" ? opts : (opts?.scope || "user");
    const cwd = opts?.cwd || process.cwd();
    if (scope === "project") {
      return exists(cwd, ".codex") || exists(cwd, "AGENTS.md") || exists(process.env.CODEX_HOME || path.join(home(), ".codex"));
    }
    return exists(process.env.CODEX_HOME || path.join(home(), ".codex"));
  },
  rules: (scope = "user", cwd = process.cwd()) => {
    return scope === "project"
      ? path.join(cwd, "AGENTS.md")
      : path.join(process.env.CODEX_HOME || path.join(home(), ".codex"), "AGENTS.md");
  },
  supportsEnvKey: true,
  add({ mcpUrl, apiKey, envKey }, { scope = "user", cwd = process.cwd() } = {}) {
    const target = codex.file(scope, cwd);
    const current = fs.existsSync(target) ? stripCodexServer(fs.readFileSync(target, "utf8")) : "";
    const block = [
      CODEX_HEADER,
      `url = ${tomlString(mcpUrl)}`,
      envKey
        ? `bearer_token_env_var = ${tomlString(KEY_ENV)}`
        : `http_headers = { Authorization = ${tomlString(`Bearer ${apiKey}`)} }`,
    ].join("\n");
    writeFileSafely(target, (current ? current + "\n\n" : "") + block + "\n");
  },
  remove({ scope = "user", cwd = process.cwd() } = {}) {
    const target = codex.file(scope, cwd);
    if (!fs.existsSync(target)) return false;
    const text = fs.readFileSync(target, "utf8");
    if (!text.includes(CODEX_HEADER)) return false;
    writeFileSafely(target, stripCodexServer(text) + "\n");
    return true;
  },
};

// --- The catalogue --------------------------------------------------------------------

export function runClaudeCLI(args, { cwd } = {}) {
  try {
    execFileSync("claude", args, { stdio: "ignore", timeout: 30_000, shell: process.platform === "win32", cwd });
    return true;
  } catch {
    return false;
  }
}

const hasClaudeCLI = () => !process.env.MEMRO_NO_CLAUDE_CLI && runClaudeCLI(["--version"]);

const claudeCode = {
  id: "claude-code",
  name: "Claude Code",
  aliases: ["claude", "claudecode"],
  supportsScope: (scope) => scope === "user" || scope === "project",
  // Claude Code expands ${VAR} in .mcp.json and ~/.claude.json.
  supportsEnvKey: true,
  // Project-scoped servers live in the project's .mcp.json (what
  // `claude mcp add --scope project` writes); user scope in ~/.claude.json.
  file: (scope = "user", cwd = process.cwd()) => {
    return scope === "project" ? path.join(cwd, ".mcp.json") : path.join(home(), ".claude.json");
  },
  detect: (opts = {}) => {
    const scope = typeof opts === "string" ? opts : (opts?.scope || "user");
    const cwd = opts?.cwd || process.cwd();
    if (scope === "project") {
      return exists(cwd, ".mcp.json") || exists(cwd, ".claude") || exists(cwd, "CLAUDE.md") || hasClaudeCLI() || exists(home(), ".claude.json") || exists(home(), ".claude");
    }
    return exists(home(), ".claude.json") || exists(home(), ".claude") || hasClaudeCLI();
  },
  rules: (scope = "user", cwd = process.cwd()) => {
    return scope === "project" ? path.join(cwd, "CLAUDE.md") : path.join(home(), ".claude", "CLAUDE.md");
  },
  add(conn, { scope = "user", cwd = process.cwd() } = {}) {
    // On Windows the CLI runs through cmd.exe, which splits the header at its
    // spaces, so edit the file there instead.
    if (process.platform !== "win32" && hasClaudeCLI()) {
      runClaudeCLI(["mcp", "remove", "--scope", scope, SERVER_NAME], { cwd });
      if (runClaudeCLI(["mcp", "add", "--transport", "http", "--scope", scope, SERVER_NAME, conn.mcpUrl,
        "--header", `Authorization: ${authHeader(conn)}`], { cwd })) return;
    }
    const target = claudeCode.file(scope, cwd);
    const cfg = readJSON(target);
    cfg.mcpServers = { ...(cfg.mcpServers || {}), [SERVER_NAME]: { type: "http", url: conn.mcpUrl, headers: { Authorization: authHeader(conn) } } };
    writeJSON(target, cfg);
  },
  remove({ scope = "user", cwd = process.cwd() } = {}) {
    if (hasClaudeCLI()) {
      const ok = runClaudeCLI(["mcp", "remove", "--scope", scope, SERVER_NAME], { cwd });
      if (ok) return true;
    }
    const target = claudeCode.file(scope, cwd);
    if (!fs.existsSync(target)) return false;
    const cfg = readJSON(target);
    if (!cfg.mcpServers?.[SERVER_NAME]) return false;
    delete cfg.mcpServers[SERVER_NAME];
    writeJSON(target, cfg);
    return true;
  },
};

// --- Coordination hooks for Claude Code and Codex (src/hook.js) ---------------------------

export const HOOK_FILE = () => path.join(home(), ".memro", "hooks", "memro-hook.mjs");
const codexHome = () => process.env.CODEX_HOME || path.join(home(), ".codex");
// Our entries are the ones running a script from ~/.memro/hooks/ (memro-cli
// 0.1.8 called it claude-code.mjs).
const isOurs = (group) => group?.hooks?.some((h) => h.command?.includes(".memro/hooks/"));

/** Where each tool keeps its hooks and which events Memro hooks into. The
 * project Claude Code file is the local, uncommitted one because the command
 * names a path on this machine. */
export const HOOK_TARGETS = {
  "claude-code": {
    file: (scope, cwd) => (scope === "project" ? path.join(cwd, ".claude", "settings.local.json") : path.join(home(), ".claude", "settings.json")),
    events: { PreToolUse: "Edit|Write|MultiEdit|NotebookEdit|Bash", UserPromptSubmit: null, Notification: null, Stop: null, SessionEnd: null },
  },
  codex: {
    file: (scope, cwd) => path.join(scope === "project" ? path.join(cwd, ".codex") : codexHome(), "hooks.json"),
    // Codex has no Notification event, and gives SessionEnd at most 3 seconds.
    events: { PreToolUse: "^apply_patch$|^Bash$", UserPromptSubmit: null, Stop: null, SessionEnd: null },
    timeouts: { SessionEnd: 3 },
  },
};

export const hookSettingsFile = (scope = "user", cwd = process.cwd(), tool = "claude-code") => HOOK_TARGETS[tool].file(scope, cwd);

/** Copy the hook to ~/.memro/hooks/ and register it for the tool's coordination events. */
export function installHooks({ scope = "user", cwd = process.cwd(), tool = "claude-code" } = {}) {
  const { events, timeouts = {} } = HOOK_TARGETS[tool];
  fs.mkdirSync(path.dirname(HOOK_FILE()), { recursive: true });
  fs.copyFileSync(new URL("./hook.js", import.meta.url), HOOK_FILE());
  const target = hookSettingsFile(scope, cwd, tool);
  const cfg = readJSON(target);
  cfg.hooks ||= {};
  const command = `node ${JSON.stringify(HOOK_FILE().split(path.sep).join("/"))} ${tool}`;
  for (const [event, matcher] of Object.entries(events)) {
    // PreToolUse may wait up to 60 s for a held file (hook.js waitMs).
    const ours = { ...(matcher ? { matcher } : {}), hooks: [{ type: "command", command, timeout: timeouts[event] || (event === "PreToolUse" ? 90 : 10) }] };
    cfg.hooks[event] = [...(cfg.hooks[event] || []).filter((g) => !isOurs(g)), ours];
  }
  writeJSON(target, cfg);
  return target;
}

/** Unregister the hooks; true if any were there. The copied script stays for other tools. */
export function removeHooks({ scope = "user", cwd = process.cwd(), tool = "claude-code" } = {}) {
  const target = hookSettingsFile(scope, cwd, tool);
  if (!fs.existsSync(target)) return false;
  const cfg = readJSON(target);
  let removed = false;
  for (const event of Object.keys(cfg.hooks || {})) {
    const groups = cfg.hooks[event];
    if (!Array.isArray(groups) || !groups.some(isOurs)) continue;
    removed = true;
    cfg.hooks[event] = groups.filter((g) => !isOurs(g));
    if (!cfg.hooks[event].length) delete cfg.hooks[event];
  }
  if (cfg.hooks && !Object.keys(cfg.hooks).length) delete cfg.hooks;
  if (removed) writeJSON(target, cfg);
  return removed;
}

export const CLIENTS = [
  claudeCode,
  codex,
  jsonClient({
    id: "cursor",
    name: "Cursor",
    aliases: [],
    file: (scope, cwd) => (scope === "project" ? path.join(cwd, ".cursor", "mcp.json") : path.join(home(), ".cursor", "mcp.json")),
    detect: (scope, cwd) => (scope === "project" ? exists(cwd, ".cursor") || exists(home(), ".cursor") : exists(home(), ".cursor")),
    entry: ({ mcpUrl, apiKey }) => ({ url: mcpUrl, headers: bearer(apiKey) }),
    // Project rules only: Cursor's user rules live in its settings, not a file.
    rules: (scope, cwd) => (scope === "project" ? path.join(cwd, ".cursor", "rules", "memro.mdc") : null),
    rulesFrontmatter: "---\ndescription: Memro long-term memory\nalwaysApply: true\n---",
  }),
  jsonClient({
    id: "antigravity",
    name: "Antigravity",
    aliases: ["agy"],
    // Workspace servers: <project>/.agents/mcp_config.json; user-wide:
    // ~/.gemini/config/mcp_config.json.
    file: (scope, cwd) => (scope === "project" ? path.join(cwd, ".agents", "mcp_config.json") : path.join(home(), ".gemini", "config", "mcp_config.json")),
    // memro-cli ≤ 0.1.5 wrote the project config here, where Antigravity never looks.
    legacyFiles: (scope, cwd) => (scope === "project" ? [path.join(cwd, ".gemini", "config", "mcp_config.json")] : []),
    detect: (scope, cwd) => (scope === "project"
      ? exists(cwd, ".gemini") || exists(cwd, ".agents") || exists(home(), ".gemini", "config") || exists(home(), ".gemini", "antigravity-ide") || exists(home(), ".gemini", "antigravity")
      : exists(home(), ".gemini", "config") || exists(home(), ".gemini", "antigravity-ide") || exists(home(), ".gemini", "antigravity")),
    entry: ({ mcpUrl, apiKey }) => ({ serverUrl: mcpUrl, headers: bearer(apiKey) }),
    rules: (scope, cwd) => (scope === "project" ? path.join(cwd, ".agents", "rules", "memro.md") : path.join(home(), ".gemini", "config", "rules", "memro.md")),
    // A rules/*.md file without a valid `trigger` is silently discarded.
    rulesFrontmatter: "---\ntrigger: always_on\n---",
  }),
  jsonClient({
    id: "opencode",
    name: "OpenCode",
    aliases: [],
    file: (scope, cwd) => (scope === "project" ? path.join(cwd, "opencode.json") : path.join(process.env.XDG_CONFIG_HOME || path.join(home(), ".config"), "opencode", "opencode.json")),
    detect: (scope, cwd) => (scope === "project" ? exists(cwd, "opencode.json") || exists(process.env.XDG_CONFIG_HOME || path.join(home(), ".config"), "opencode") : exists(process.env.XDG_CONFIG_HOME || path.join(home(), ".config"), "opencode")),
    key: "mcp",
    entry: ({ mcpUrl, apiKey }) => ({ type: "remote", url: mcpUrl, enabled: true, headers: bearer(apiKey) }),
    rules: (scope, cwd) => (scope === "project" ? path.join(cwd, "AGENTS.md") : path.join(process.env.XDG_CONFIG_HOME || path.join(home(), ".config"), "opencode", "AGENTS.md")),
  }),
  jsonClient({
    id: "gemini-cli",
    name: "Gemini CLI",
    aliases: ["gemini"],
    file: (scope, cwd) => (scope === "project" ? path.join(cwd, ".gemini", "settings.json") : path.join(home(), ".gemini", "settings.json")),
    detect: (scope, cwd) => (scope === "project" ? exists(cwd, ".gemini") || exists(home(), ".gemini", "settings.json") : exists(home(), ".gemini", "settings.json")),
    entry: ({ mcpUrl, apiKey }) => ({ httpUrl: mcpUrl, headers: bearer(apiKey) }),
    rules: (scope, cwd) => (scope === "project" ? path.join(cwd, "GEMINI.md") : path.join(home(), ".gemini", "GEMINI.md")),
  }),
  jsonClient({
    id: "vscode",
    name: "VS Code",
    aliases: ["code"],
    file: (scope, cwd) => (scope === "project" ? path.join(cwd, ".vscode", "mcp.json") : appSupport("Code", "User", "mcp.json")),
    detect: (scope, cwd) => (scope === "project" ? exists(cwd, ".vscode") || fs.existsSync(appSupport("Code", "User")) : fs.existsSync(appSupport("Code", "User"))),
    key: "servers",
    entry: ({ mcpUrl, apiKey }) => ({ type: "http", url: mcpUrl, headers: bearer(apiKey) }),
    // Copilot Chat reads *.instructions.md from these default locations.
    rules: (scope, cwd) => (scope === "project"
      ? path.join(cwd, ".github", "instructions", "memro.instructions.md")
      : path.join(home(), ".copilot", "instructions", "memro.instructions.md")),
    rulesFrontmatter: "---\ndescription: Memro long-term memory\napplyTo: \"**\"\n---",
  }),
  jsonClient({
    id: "windsurf",
    name: "Windsurf",
    aliases: [],
    // Windsurf reads MCP servers from one user-wide file; it has no
    // per-project MCP config.
    supportsScope: (scope) => scope === "user",
    file: () => path.join(home(), ".codeium", "windsurf", "mcp_config.json"),
    // ~/.codeium/windsurf outlives an uninstall, so it is not proof.
    detect: () => appInstalled({ mac: "Windsurf", win: "Windsurf", cmd: "windsurf", linux: ["/usr/share/windsurf", "/opt/Windsurf", "/opt/windsurf"] }),
    entry: ({ mcpUrl, apiKey }) => ({ serverUrl: mcpUrl, headers: bearer(apiKey) }),
    rules: () => path.join(home(), ".codeium", "windsurf", "memories", "global_rules.md"),
  }),
  jsonClient({
    id: "claude-desktop",
    name: "Claude Desktop",
    aliases: ["desktop"],
    supportsScope: (scope) => scope === "user",
    file: () => appSupport("Claude", "claude_desktop_config.json"),
    detect: () => fs.existsSync(appSupport("Claude")),
    rules: () => null,
    // Claude Desktop only launches local servers; mcp-remote bridges to /mcp.
    entry: ({ mcpUrl, apiKey }) => ({
      command: "npx",
      // Pinned: npx would otherwise run whatever version is published next.
      args: ["-y", `mcp-remote@${MCP_REMOTE_VERSION}`, mcpUrl, "--header", "Authorization:${MEMRO_AUTH}"],
      env: { MEMRO_AUTH: `Bearer ${apiKey}` },
    }),
  }),
];

export function findClient(id) {
  if (!id) return undefined;
  const norm = id.toLowerCase().trim();
  return CLIENTS.find((c) => c.id === norm || c.aliases?.includes(norm));
}
