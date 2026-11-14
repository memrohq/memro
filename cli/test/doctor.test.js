import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

// Isolate HOME so tests never touch real configs.
const TEST_DIR = fs.mkdtempSync(path.join(os.tmpdir(), "memro-doctor-test-"));
const HOME = path.join(TEST_DIR, "home");
const PROJECT = path.join(TEST_DIR, "project");
process.env.HOME = HOME;
process.env.USERPROFILE = HOME;
process.env.APPDATA = path.join(HOME, "AppData");
process.env.XDG_CONFIG_HOME = path.join(HOME, ".config");
process.env.CODEX_HOME = path.join(HOME, ".codex");
process.env.MEMRO_NO_CLAUDE_CLI = "1";

const { findClient } = await import("../src/clients.js");
const { readServer, diagnose } = await import("../src/doctor.js");
const { installInstructions } = await import("../src/instructions.js");

const URL_ = "https://mcp.memro.co/mcp";
const GOOD = "memro_sk_good";
// A fake server: GOOD is accepted with 25 tools, anything else is rejected.
const check = async (url, key) => (key === GOOD ? { ok: true, tools: 25 } : { ok: false, status: 401, error: "key rejected" });
const claude = findClient("claude-code");
const codex = findClient("codex");

beforeEach(() => {
  fs.rmSync(TEST_DIR, { recursive: true, force: true });
  fs.mkdirSync(HOME, { recursive: true });
  fs.mkdirSync(PROJECT, { recursive: true });
});

test("readServer finds the key, or the variable it comes from, in each format", () => {
  claude.add({ mcpUrl: URL_, apiKey: GOOD }, { scope: "project", cwd: PROJECT });
  assert.deepEqual(readServer(claude.file("project", PROJECT)), { url: URL_, auth: { key: GOOD } });

  claude.add({ mcpUrl: URL_, apiKey: GOOD, envKey: true }, { scope: "project", cwd: PROJECT });
  assert.deepEqual(readServer(claude.file("project", PROJECT)).auth, { env: "MEMRO_API_KEY" });

  codex.add({ mcpUrl: URL_, apiKey: GOOD }, { scope: "project", cwd: PROJECT });
  assert.deepEqual(readServer(codex.file("project", PROJECT)), { url: URL_, auth: { key: GOOD } });
  codex.add({ mcpUrl: URL_, apiKey: GOOD, envKey: true }, { scope: "project", cwd: PROJECT });
  assert.deepEqual(readServer(codex.file("project", PROJECT)).auth, { env: "MEMRO_API_KEY" });

  const desktop = findClient("claude-desktop");
  desktop.add({ mcpUrl: URL_, apiKey: GOOD });
  assert.deepEqual(readServer(desktop.file()), { url: URL_, auth: { key: GOOD } });

  assert.equal(readServer(path.join(PROJECT, "missing.json")), null);
});

test("the project config wins over the global one, and its key is what gets checked", async () => {
  claude.add({ mcpUrl: URL_, apiKey: GOOD }, { scope: "user" });
  claude.add({ mcpUrl: URL_, apiKey: "memro_sk_bad" }, { scope: "project", cwd: PROJECT });
  const [row] = await diagnose([claude], { cwd: PROJECT, inProject: true, check });
  assert.equal(row.scope, "project");
  assert.deepEqual(row.overridden, [claude.file("user")]);
  assert.equal(row.problem, "key rejected", "a good global key doesn't hide a bad project key");

  // Outside the project only the global config counts.
  const [globalRow] = await diagnose([claude], { cwd: HOME, inProject: false, check });
  assert.equal(globalRow.scope, "user");
  assert.equal(globalRow.tools, 25);
});

test("--env-key setups: a missing variable is reported before asking the server", async () => {
  codex.add({ mcpUrl: URL_, apiKey: GOOD, envKey: true }, { scope: "project", cwd: PROJECT });
  let asked = false;
  const [missing] = await diagnose([codex], { cwd: PROJECT, inProject: true, env: {}, check: async () => { asked = true; return {}; } });
  assert.equal(missing.problem, "$MEMRO_API_KEY is not set");
  assert.equal(asked, false);

  const [set] = await diagnose([codex], { cwd: PROJECT, inProject: true, env: { MEMRO_API_KEY: GOOD }, check });
  assert.equal(set.keySource, "$MEMRO_API_KEY");
  assert.equal(set.tools, 25);
});

test("Claude Code: a private per-project server beats .mcp.json, and a turned-off .mcp.json server is reported", async () => {
  claude.add({ mcpUrl: URL_, apiKey: "memro_sk_bad" }, { scope: "project", cwd: PROJECT });
  const state = path.join(HOME, ".claude.json");
  fs.writeFileSync(state, JSON.stringify({ projects: { [PROJECT]: { mcpServers: { memro: { type: "http", url: URL_, headers: { Authorization: `Bearer ${GOOD}` } } } } } }));
  const [local] = await diagnose([claude], { cwd: PROJECT, inProject: true, check });
  assert.equal(local.scope, "local");
  assert.equal(local.tools, 25);

  fs.writeFileSync(state, JSON.stringify({ projects: { [PROJECT]: { disabledMcpjsonServers: ["memro"] } } }));
  const [off] = await diagnose([claude], { cwd: PROJECT, inProject: true, check });
  assert.match(off.problem, /turned off for this project/);
});

test("the Memro note counts from the global or the project instructions file", async () => {
  claude.add({ mcpUrl: URL_, apiKey: GOOD }, { scope: "project", cwd: PROJECT });
  let [row] = await diagnose([claude], { cwd: PROJECT, inProject: true, check });
  assert.equal(row.instructions, false);

  installInstructions(claude.rules("user"), { scope: "user" });
  [row] = await diagnose([claude], { cwd: PROJECT, inProject: true, check });
  assert.equal(row.instructions, true, "Claude Code also reads ~/.claude/CLAUDE.md in a project");
});

test("tools without Memro are listed only when installed; one server check per key", async () => {
  const cursor = findClient("cursor");
  assert.deepEqual(await diagnose([cursor], { cwd: HOME, inProject: false, check }), []);
  fs.mkdirSync(path.join(HOME, ".cursor"));
  const [row] = await diagnose([cursor], { cwd: HOME, inProject: false, check });
  assert.equal(row.problem, "not connected");

  let calls = 0;
  const counting = async (...a) => { calls++; return check(...a); };
  claude.add({ mcpUrl: URL_, apiKey: GOOD }, { scope: "user" });
  codex.add({ mcpUrl: URL_, apiKey: GOOD }, { scope: "user" });
  await diagnose([claude, codex], { cwd: HOME, inProject: false, check: counting });
  assert.equal(calls, 1);
});
