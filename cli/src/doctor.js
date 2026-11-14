// `memro doctor`: for each tool, which Memro config it uses (project or
// global), where its key comes from, and whether the server accepts that key
// and lists its tools. It reads config files and talks to the Memro MCP server
// itself; it can't see what a running tool actually loaded.

import fs from "node:fs";

import { SERVER_NAME } from "./clients.js";
import { START, hasOwnMemroGuidance } from "./instructions.js";

const SERVER_KEYS = ["mcpServers", "servers", "mcp"];

/** The Memro entry in one tool's config file: { url, auth } or null. auth is
 * { key }, { env } (a variable to read the key from) or {} when there is none. */
export function readServer(file) {
  if (!fs.existsSync(file)) return null;
  const text = fs.readFileSync(file, "utf8");
  if (file.endsWith(".toml")) return readCodex(text);
  let cfg;
  try { cfg = JSON.parse(text); } catch { return null; }
  const entry = SERVER_KEYS.map((k) => cfg?.[k]?.[SERVER_NAME]).find(Boolean);
  if (!entry) return null;
  const url = entry.url || entry.serverUrl || entry.httpUrl || entry.args?.find((a) => /^https?:\/\//.test(a));
  return { url, auth: parseAuth(entry.headers?.Authorization || entry.env?.MEMRO_AUTH) };
}

function readCodex(text) {
  const lines = text.split("\n");
  const start = lines.findIndex((l) => l.trim() === `[mcp_servers.${SERVER_NAME}]`);
  if (start < 0) return null;
  const body = [];
  for (const l of lines.slice(start + 1)) {
    if (l.trim().startsWith("[")) break;
    body.push(l);
  }
  const value = (re) => body.join("\n").match(re)?.[1];
  const env = value(/^\s*bearer_token_env_var\s*=\s*"([^"]+)"/m);
  return {
    url: value(/^\s*url\s*=\s*"([^"]+)"/m),
    auth: env ? { env } : parseAuth(value(/Authorization\s*=\s*"([^"]+)"/)),
  };
}

const CODEX_UNTRUSTED = "Codex ignores ./.codex/config.toml until you open Codex in this folder and trust it";

/** Whether Codex's own config marks `dir` as trusted: a [projects."<dir>"]
 * table with trust_level = "trusted" (the path as given or resolved). */
export function codexTrusts(globalConfig, dir) {
  if (!fs.existsSync(globalConfig)) return false;
  const lines = fs.readFileSync(globalConfig, "utf8").split("\n");
  const dirs = new Set([dir]);
  try { dirs.add(fs.realpathSync(dir)); } catch { /* keep the path as given */ }
  return [...dirs].some((d) => {
    const start = lines.findIndex((l) => l.trim() === `[projects.${JSON.stringify(d)}]`);
    if (start < 0) return false;
    for (const l of lines.slice(start + 1)) {
      if (l.trim().startsWith("[")) return false;
      if (/^\s*trust_level\s*=\s*"trusted"\s*$/.test(l)) return true;
    }
    return false;
  });
}

const keySource = (auth) => (auth.env ? `$${auth.env}` : auth.key ? "in the file" : "none");

function readJSONQuiet(file) {
  try { return JSON.parse(fs.readFileSync(file, "utf8")); } catch { return null; }
}

function parseAuth(header) {
  if (!header) return {};
  const env = header.match(/\$\{([A-Za-z_][A-Za-z0-9_]*)\}/)?.[1];
  if (env) return { env };
  const key = header.replace(/^Bearer\s+/i, "").trim();
  return key ? { key } : {};
}

/** Ask the MCP server for its tools: { ok, status, tools } or { ok: false, error }. */
export async function checkServer(url, key) {
  let res;
  try {
    res = await fetch(url, {
      method: "POST",
      headers: { Authorization: `Bearer ${key}`, "Content-Type": "application/json", Accept: "application/json, text/event-stream" },
      body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/list" }),
      signal: AbortSignal.timeout(15_000),
    });
  } catch (err) {
    return { ok: false, error: `can't reach ${new URL(url).host} (${err.cause?.code || err.name})` };
  }
  if (res.status === 401 || res.status === 403) return { ok: false, status: res.status, error: "key rejected" };
  if (!res.ok) return { ok: false, status: res.status, error: `server returned ${res.status}` };
  // The hosted server answers as server-sent events; others may send plain JSON.
  const text = await res.text();
  const json = text.trimStart().startsWith("{") ? text : text.split("\n").filter((l) => l.startsWith("data:")).map((l) => l.slice(5)).pop();
  try {
    return { ok: true, tools: JSON.parse(json).result.tools.length };
  } catch {
    return { ok: false, error: "unexpected reply from the server" };
  }
}

/**
 * One row per tool: the config it uses (project overrides global), where the
 * key comes from, and the server's answer. Tools with no Memro config are
 * reported only when installed. `check` is checkServer, injectable for tests.
 */
export async function diagnose(clients, { cwd, inProject, env = process.env, check = checkServer }) {
  const cache = new Map();
  const probe = (url, key) => {
    const id = `${url}\n${key}`;
    if (!cache.has(id)) cache.set(id, check(url, key));
    return cache.get(id);
  };
  const rows = [];
  for (const c of clients) {
    const scopes = (inProject && c.supportsScope("project") ? ["project", "user"] : ["user"]);
    const found = scopes.map((scope) => ({ scope, file: c.file(scope, cwd) }))
      .map((s) => ({ ...s, server: readServer(s.file) }))
      .filter((s) => s.server);
    // Claude Code's private per-project servers (`--scope local`) sit in
    // ~/.claude.json under the project's path and beat .mcp.json.
    // Codex ignores a project's .codex/config.toml until the folder is trusted.
    let ignoredProject = null;
    if (c.id === "codex" && found[0]?.scope === "project" && !codexTrusts(c.file("user", cwd), cwd)) {
      const ignored = found.shift();
      ignoredProject = ignored.file;
      if (!found.length) {
        rows.push({ client: c, scope: "project", file: ignored.file, overridden: [], keySource: keySource(ignored.server.auth), problem: CODEX_UNTRUSTED });
        continue;
      }
    }
    const claudeState = c.id === "claude-code" ? readJSONQuiet(c.file("user", cwd))?.projects?.[cwd] : null;
    const local = claudeState?.mcpServers?.[SERVER_NAME];
    if (inProject && local) {
      found.unshift({ scope: "local", file: `${c.file("user", cwd)} (this project)`, server: { url: local.url, auth: parseAuth(local.headers?.Authorization) } });
    }
    if (!found.length) {
      if (c.detect({ scope: inProject ? "project" : "user", cwd })) rows.push({ client: c, problem: "not connected" });
      continue;
    }
    const [active, ...overridden] = found;
    const row = { client: c, scope: active.scope, file: active.file, overridden: overridden.map((s) => s.file) };
    const { url, auth } = active.server;
    const key = auth.key || (auth.env && env[auth.env]);
    row.keySource = keySource(auth);
    if (active.scope === "project" && claudeState?.disabledMcpjsonServers?.includes(SERVER_NAME)) {
      row.problem = "turned off for this project in Claude Code (/mcp to turn it on)";
    } else if (!url) row.problem = "no server URL";
    else if (auth.env && !env[auth.env]) row.problem = `$${auth.env} is not set`;
    else if (!key) row.problem = "no key";
    else {
      const res = await probe(url, key);
      if (res.ok) row.tools = res.tools;
      else row.problem = res.error;
    }
    if (ignoredProject && !row.problem) row.problem = `uses the global config: ${CODEX_UNTRUSTED}`;
    // Tools read their global instructions and the project's together.
    const ruleFiles = [...new Set((inProject ? ["project", "user"] : ["user"]).map((s) => c.rules(s, cwd)).filter(Boolean))];
    if (ruleFiles.length) {
      row.instructions = ruleFiles.some((f) => {
        const text = fs.existsSync(f) ? fs.readFileSync(f, "utf8") : "";
        return text.includes(START) || hasOwnMemroGuidance(text);
      });
    }
    rows.push(row);
  }
  return rows;
}
