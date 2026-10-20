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
  supportsScope = () => true,
}) {
  return {
    id,
    name,
    aliases,
    supportsScope,
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
  add({ mcpUrl, apiKey }, { scope = "user", cwd = process.cwd() } = {}) {
    const target = codex.file(scope, cwd);
    const current = fs.existsSync(target) ? stripCodexServer(fs.readFileSync(target, "utf8")) : "";
    const block = [
      CODEX_HEADER,
      `url = ${tomlString(mcpUrl)}`,
      `http_headers = { Authorization = ${tomlString(`Bearer ${apiKey}`)} }`,
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
  file: (scope = "user", cwd = process.cwd()) => {
    return scope === "project" ? path.join(cwd, ".claude.json") : path.join(home(), ".claude.json");
  },
  detect: (opts = {}) => {
    const scope = typeof opts === "string" ? opts : (opts?.scope || "user");
    const cwd = opts?.cwd || process.cwd();
    if (scope === "project") {
      return exists(cwd, ".claude.json") || exists(cwd, ".claude") || exists(cwd, "CLAUDE.md") || hasClaudeCLI() || exists(home(), ".claude.json") || exists(home(), ".claude");
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
        "--header", `Authorization: Bearer ${conn.apiKey}`], { cwd })) return;
    }
    const target = claudeCode.file(scope, cwd);
    const cfg = readJSON(target);
    cfg.mcpServers = { ...(cfg.mcpServers || {}), [SERVER_NAME]: { type: "http", url: conn.mcpUrl, headers: bearer(conn.apiKey) } };
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
  }),
  jsonClient({
    id: "antigravity",
    name: "Antigravity",
    aliases: ["agy"],
    file: (scope, cwd) => (scope === "project" ? path.join(cwd, ".gemini", "config", "mcp_config.json") : path.join(home(), ".gemini", "config", "mcp_config.json")),
    detect: (scope, cwd) => (scope === "project"
      ? exists(cwd, ".gemini") || exists(cwd, ".agents") || exists(home(), ".gemini", "config") || exists(home(), ".gemini", "antigravity-ide") || exists(home(), ".gemini", "antigravity")
      : exists(home(), ".gemini", "config") || exists(home(), ".gemini", "antigravity-ide") || exists(home(), ".gemini", "antigravity")),
    entry: ({ mcpUrl, apiKey }) => ({ serverUrl: mcpUrl, headers: bearer(apiKey) }),
    rules: (scope, cwd) => (scope === "project" ? path.join(cwd, ".agents", "rules", "memro.md") : path.join(home(), ".gemini", "config", "rules", "memro.md")),
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
  }),
  jsonClient({
    id: "windsurf",
    name: "Windsurf",
    aliases: [],
    file: (scope, cwd) => (scope === "project" ? path.join(cwd, ".codeium", "windsurf", "mcp_config.json") : path.join(home(), ".codeium", "windsurf", "mcp_config.json")),
    detect: (scope, cwd) => (scope === "project" ? exists(cwd, ".codeium") || exists(home(), ".codeium", "windsurf") : exists(home(), ".codeium", "windsurf")),
    entry: ({ mcpUrl, apiKey }) => ({ serverUrl: mcpUrl, headers: bearer(apiKey) }),
    rules: (scope, cwd) => (scope === "project" ? path.join(cwd, ".codeium", "windsurf", "memories", "global_rules.md") : path.join(home(), ".codeium", "windsurf", "memories", "global_rules.md")),
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
