import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

// Point every client at a throwaway home before loading the module.
const HOME = fs.mkdtempSync(path.join(os.tmpdir(), "memro-cli-"));
process.env.HOME = HOME;
process.env.USERPROFILE = HOME;
process.env.APPDATA = path.join(HOME, "AppData");
process.env.XDG_CONFIG_HOME = path.join(HOME, ".config");
process.env.CODEX_HOME = path.join(HOME, ".codex");
process.env.MEMRO_NO_CLAUDE_CLI = "1";

const { CLIENTS, MCP_REMOTE_VERSION, findClient, stripCodexServer, withSession } = await import("../src/clients.js");
const conn = { mcpUrl: "https://mcp.memro.co/mcp", apiKey: "memro_sk_test" };

beforeEach(() => {
  for (const f of fs.readdirSync(HOME)) fs.rmSync(path.join(HOME, f), { recursive: true, force: true });
});

test("every client adds and removes Memro without touching other servers", () => {
  for (const c of CLIENTS) {
    if (c.id === "codex") continue;
    const file = c.file();
    fs.mkdirSync(path.dirname(file), { recursive: true });
    const key = c.id === "opencode" ? "mcp" : c.id === "vscode" ? "servers" : "mcpServers";
    fs.writeFileSync(file, JSON.stringify({ theme: "dark", [key]: { other: { url: "x" } } }));
    c.add(conn);
    const cfg = JSON.parse(fs.readFileSync(file, "utf8"));
    assert.equal(cfg.theme, "dark", c.id);
    assert.deepEqual(cfg[key].other, { url: "x" }, c.id);
    assert.ok(JSON.stringify(cfg[key].memro).includes("memro_sk_test"), c.id);
    assert.ok(fs.existsSync(`${file}.memro-backup`), `${c.id} backup`);
    if (process.platform !== "win32") assert.equal(fs.statSync(file).mode & 0o777, 0o600, `${c.id} mode`);
    assert.equal(c.remove(), true, c.id);
    assert.equal(JSON.parse(fs.readFileSync(file, "utf8"))[key].memro, undefined, c.id);
  }
});

test("client entries use each tool's format", () => {
  const entry = (id) => {
    const c = findClient(id);
    c.add(conn);
    const cfg = JSON.parse(fs.readFileSync(c.file(), "utf8"));
    return (cfg.mcpServers || cfg.servers || cfg.mcp).memro;
  };
  assert.deepEqual(entry("claude-code"), { type: "http", url: conn.mcpUrl, headers: { Authorization: "Bearer memro_sk_test" } });
  assert.equal(entry("antigravity").serverUrl, conn.mcpUrl);
  assert.equal(entry("windsurf").serverUrl, conn.mcpUrl);
  assert.equal(entry("gemini-cli").httpUrl, conn.mcpUrl);
  assert.equal(entry("opencode").type, "remote");
  assert.equal(entry("vscode").type, "http");
  assert.equal(entry("cursor").url, conn.mcpUrl);
  assert.equal(entry("claude-desktop").command, "npx");
});

test("codex config.toml keeps other settings and replaces an old Memro block", () => {
  const c = findClient("codex");
  fs.mkdirSync(path.dirname(c.file()), { recursive: true });
  fs.writeFileSync(c.file(), 'model = "o4"\n\n[mcp_servers.memro]\nurl = "old"\n\n[mcp_servers.memro.env]\nA = "1"\n\n[mcp_servers.other]\ncommand = "x"\n');
  c.add(conn);
  const text = fs.readFileSync(c.file(), "utf8");
  assert.match(text, /model = "o4"/);
  assert.match(text, /\[mcp_servers\.other\]\ncommand = "x"/);
  assert.doesNotMatch(text, /old|A = "1"/);
  assert.equal(text.match(/\[mcp_servers\.memro\]/g).length, 1);
  assert.match(text, /http_headers = \{ Authorization = "Bearer memro_sk_test" \}/);
  assert.equal(c.remove(), true);
  assert.doesNotMatch(fs.readFileSync(c.file(), "utf8"), /memro/);
});

test("stripCodexServer leaves unrelated tables alone", () => {
  assert.equal(stripCodexServer('[a]\nx = 1\n[mcp_servers.memro]\nurl = "u"\n[b]\ny = 2'), "[a]\nx = 1\n[b]\ny = 2");
});

test("JSONC configs are refused rather than overwritten", () => {
  const c = findClient("opencode");
  fs.mkdirSync(path.dirname(c.file()), { recursive: true });
  fs.writeFileSync(c.file(), '{ // comment\n "mcp": {} }');
  assert.throws(() => c.add(conn), /not plain JSON/);
  assert.match(fs.readFileSync(c.file(), "utf8"), /comment/);
});

test("detection only reports installed tools", () => {
  assert.deepEqual(CLIENTS.filter((c) => c.detect()).map((c) => c.id), []);
  fs.mkdirSync(path.join(HOME, ".cursor"));
  fs.mkdirSync(path.join(HOME, ".gemini", "config"), { recursive: true });
  assert.deepEqual(CLIENTS.filter((c) => c.detect()).map((c) => c.id).sort(), ["antigravity", "cursor"]);
});

test("writes replace the file in one step and leave no temporary files", () => {
  const c = findClient("cursor");
  c.add(conn);
  const dir = path.dirname(c.file());
  assert.deepEqual(fs.readdirSync(dir).filter((f) => f.includes("memro-tmp")), []);
});

test("Claude Desktop runs a pinned mcp-remote", () => {
  const c = findClient("claude-desktop");
  c.add(conn);
  const { args } = JSON.parse(fs.readFileSync(c.file(), "utf8")).mcpServers.memro;
  assert.equal(args[1], `mcp-remote@${MCP_REMOTE_VERSION}`);
  assert.ok(!JSON.stringify(args).includes("memro_sk_"), "key stays out of argv");
});

test("API URLs must use https unless they point at this machine", async () => {
  const { checkURL } = await import("../src/cli.js");
  assert.equal(checkURL("https://api.memro.co", "API"), "https://api.memro.co");
  assert.equal(checkURL("http://localhost:8081", "API"), "http://localhost:8081");
  assert.equal(checkURL("http://127.0.0.1:8081", "API"), "http://127.0.0.1:8081");
  assert.throws(() => checkURL("http://memro.example.com", "API"), /https/);
  assert.throws(() => checkURL("http://localhost.evil.com", "API"), /https/);
  assert.throws(() => checkURL("not a url", "API"), /Invalid/);
});

// Regression: a leftover ~/.codeium/windsurf (Windsurf uninstalled, or
// created by another tool) made Windsurf look installed.
test("Windsurf counts as installed only when the app or its command exists", () => {
  const windsurf = findClient("windsurf");
  fs.mkdirSync(path.join(HOME, ".codeium", "windsurf"), { recursive: true });
  const savedPath = process.env.PATH;
  try {
    process.env.PATH = path.join(HOME, "bin");
    assert.equal(windsurf.detect(), false, "settings folder alone is not an install");
    fs.mkdirSync(path.join(HOME, "bin"));
    const exe = path.join(HOME, "bin", process.platform === "win32" ? "windsurf.EXE" : "windsurf");
    fs.writeFileSync(exe, "", { mode: 0o755 });
    assert.equal(windsurf.detect(), true, "windsurf command on PATH");
  } finally {
    process.env.PATH = savedPath;
  }
  assert.equal(windsurf.supportsScope("project"), false, "Windsurf has no per-project MCP config");
});

test("each tool's MCP URL carries its own coordination session", () => {
  assert.equal(withSession("https://mcp.memro.co/mcp", "antigravity"), "https://mcp.memro.co/mcp?session_id=antigravity");
  assert.equal(withSession("http://127.0.0.1:8080/mcp?x=1", "claude-code"), "http://127.0.0.1:8080/mcp?x=1&session_id=claude-code");
  // A session the user set by hand is kept.
  assert.equal(withSession("https://mcp.memro.co/mcp?session_id=mine", "cursor"), "https://mcp.memro.co/mcp?session_id=mine");
  findClient("antigravity").add({ ...conn, mcpUrl: withSession(conn.mcpUrl, "antigravity") });
  const cfg = JSON.parse(fs.readFileSync(findClient("antigravity").file(), "utf8"));
  assert.equal(cfg.mcpServers.memro.serverUrl, "https://mcp.memro.co/mcp?session_id=antigravity");
});
