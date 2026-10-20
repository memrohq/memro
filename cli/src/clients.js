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
function jsonClient({ id, name, file, detect, key = "mcpServers", entry, rules }) {
  return {
    id,
    name,
    file,
    detect,
    rules,
    add(conn) {
      const cfg = readJSON(file());
      cfg[key] = { ...(cfg[key] || {}), [SERVER_NAME]: entry(conn) };
      writeJSON(file(), cfg);
    },
    remove() {
      if (!fs.existsSync(file())) return false;
      const cfg = readJSON(file());
      if (!cfg[key]?.[SERVER_NAME]) return false;
      delete cfg[key][SERVER_NAME];
      writeJSON(file(), cfg);
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
  file: () => path.join(process.env.CODEX_HOME || path.join(home(), ".codex"), "config.toml"),
  detect: () => exists(process.env.CODEX_HOME || path.join(home(), ".codex")),
  rules: () => path.join(process.env.CODEX_HOME || path.join(home(), ".codex"), "AGENTS.md"),
  add({ mcpUrl, apiKey }) {
    const file = codex.file();
    const current = fs.existsSync(file) ? stripCodexServer(fs.readFileSync(file, "utf8")) : "";
    const block = [
      CODEX_HEADER,
      `url = ${tomlString(mcpUrl)}`,
      `http_headers = { Authorization = ${tomlString(`Bearer ${apiKey}`)} }`,
    ].join("\n");
    writeFileSafely(file, (current ? current + "\n\n" : "") + block + "\n");
  },
  remove() {
    const file = codex.file();
    if (!fs.existsSync(file)) return false;
    const text = fs.readFileSync(file, "utf8");
    if (!text.includes(CODEX_HEADER)) return false;
    writeFileSafely(file, stripCodexServer(text) + "\n");
    return true;
  },
};

// --- The catalogue --------------------------------------------------------------------

// Claude Code rewrites ~/.claude.json while it runs, so prefer its own CLI
// and only edit the file when `claude` is not on PATH.
const claudeJSON = jsonClient({
  id: "claude-code",
  name: "Claude Code",
  file: () => path.join(home(), ".claude.json"),
  detect: () => exists(home(), ".claude.json") || exists(home(), ".claude"),
  entry: ({ mcpUrl, apiKey }) => ({ type: "http", url: mcpUrl, headers: bearer(apiKey) }),
  rules: () => path.join(home(), ".claude", "CLAUDE.md"),
});

function claudeCLI(...args) {
  try {
    execFileSync("claude", args, { stdio: "ignore", timeout: 30_000, shell: process.platform === "win32" });
    return true;
  } catch {
    return false;
  }
}

const hasClaudeCLI = () => !process.env.MEMRO_NO_CLAUDE_CLI && claudeCLI("--version");

const claudeCode = {
  ...claudeJSON,
  add(conn) {
    // On Windows the CLI runs through cmd.exe, which splits the header at its
    // spaces, so edit the file there instead.
    if (process.platform !== "win32" && hasClaudeCLI()) {
      claudeCLI("mcp", "remove", "--scope", "user", SERVER_NAME);
      if (claudeCLI("mcp", "add", "--transport", "http", "--scope", "user", SERVER_NAME, conn.mcpUrl,
        "--header", `Authorization: Bearer ${conn.apiKey}`)) return;
    }
    claudeJSON.add(conn);
  },
  remove() {
    if (hasClaudeCLI()) return claudeCLI("mcp", "remove", "--scope", "user", SERVER_NAME);
    return claudeJSON.remove();
  },
};

export const CLIENTS = [
  claudeCode,
  codex,
  jsonClient({
    id: "cursor",
    name: "Cursor",
    file: () => path.join(home(), ".cursor", "mcp.json"),
    detect: () => exists(home(), ".cursor"),
    entry: ({ mcpUrl, apiKey }) => ({ url: mcpUrl, headers: bearer(apiKey) }),
  }),
  jsonClient({
    id: "antigravity",
    name: "Antigravity",
    file: () => path.join(home(), ".gemini", "config", "mcp_config.json"),
    detect: () => exists(home(), ".gemini", "config") || exists(home(), ".gemini", "antigravity-ide") || exists(home(), ".gemini", "antigravity"),
    entry: ({ mcpUrl, apiKey }) => ({ serverUrl: mcpUrl, headers: bearer(apiKey) }),
    rules: () => path.join(home(), ".gemini", "config", "rules", "memro.md"),
  }),
  jsonClient({
    id: "opencode",
    name: "OpenCode",
    file: () => path.join(process.env.XDG_CONFIG_HOME || path.join(home(), ".config"), "opencode", "opencode.json"),
    detect: () => exists(process.env.XDG_CONFIG_HOME || path.join(home(), ".config"), "opencode"),
    key: "mcp",
    entry: ({ mcpUrl, apiKey }) => ({ type: "remote", url: mcpUrl, enabled: true, headers: bearer(apiKey) }),
    rules: () => path.join(process.env.XDG_CONFIG_HOME || path.join(home(), ".config"), "opencode", "AGENTS.md"),
  }),
  jsonClient({
    id: "gemini-cli",
    name: "Gemini CLI",
    file: () => path.join(home(), ".gemini", "settings.json"),
    detect: () => exists(home(), ".gemini", "settings.json"),
    entry: ({ mcpUrl, apiKey }) => ({ httpUrl: mcpUrl, headers: bearer(apiKey) }),
    rules: () => path.join(home(), ".gemini", "GEMINI.md"),
  }),
  jsonClient({
    id: "vscode",
    name: "VS Code",
    file: () => appSupport("Code", "User", "mcp.json"),
    detect: () => fs.existsSync(appSupport("Code", "User")),
    key: "servers",
    entry: ({ mcpUrl, apiKey }) => ({ type: "http", url: mcpUrl, headers: bearer(apiKey) }),
  }),
  jsonClient({
    id: "windsurf",
    name: "Windsurf",
    file: () => path.join(home(), ".codeium", "windsurf", "mcp_config.json"),
    detect: () => exists(home(), ".codeium", "windsurf"),
    entry: ({ mcpUrl, apiKey }) => ({ serverUrl: mcpUrl, headers: bearer(apiKey) }),
    rules: () => path.join(home(), ".codeium", "windsurf", "memories", "global_rules.md"),
  }),
  jsonClient({
    id: "claude-desktop",
    name: "Claude Desktop",
    file: () => appSupport("Claude", "claude_desktop_config.json"),
    detect: () => fs.existsSync(appSupport("Claude")),
    // Claude Desktop only launches local servers; mcp-remote bridges to /mcp.
    entry: ({ mcpUrl, apiKey }) => ({
      command: "npx",
      // Pinned: npx would otherwise run whatever version is published next.
      args: ["-y", `mcp-remote@${MCP_REMOTE_VERSION}`, mcpUrl, "--header", "Authorization:${MEMRO_AUTH}"],
      env: { MEMRO_AUTH: `Bearer ${apiKey}` },
    }),
  }),
];

export const findClient = (id) => CLIENTS.find((c) => c.id === id);
