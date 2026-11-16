import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

// Isolate HOME so tests never touch real configs.
const TEST_DIR = fs.mkdtempSync(path.join(os.tmpdir(), "memro-hook-test-"));
const HOME = path.join(TEST_DIR, "home");
process.env.HOME = HOME;
process.env.USERPROFILE = HOME;

const { handle, resourceFor, sessionOf } = await import("../src/hook.js");
const { HOOK_FILE, hookSettingsFile, installHooks, removeHooks } = await import("../src/clients.js");

const cfg = { api: "https://api.test", apiKey: "memro_sk_test" };
let calls;
/** A fake Memro API: records each call and answers with reply(route, body). */
const serve = (reply) => {
  calls = [];
  globalThis.fetch = async (url, init) => {
    const route = url.slice(cfg.api.length);
    const body = init.body ? JSON.parse(init.body) : null;
    calls.push({ route, body });
    return { ok: true, json: async () => reply(route, body) };
  };
};

beforeEach(() => {
  fs.rmSync(HOME, { recursive: true, force: true });
  fs.mkdirSync(HOME, { recursive: true });
});

test("resourceFor names a file by its repo's origin, so worktrees and clones agree", () => {
  const repo = path.join(TEST_DIR, "checkout-folder");
  fs.mkdirSync(path.join(repo, "src"), { recursive: true });
  execFileSync("git", ["init", "-q"], { cwd: repo });
  execFileSync("git", ["remote", "add", "origin", "git@github.com:memrohq/memro.git"], { cwd: repo });
  assert.equal(resourceFor("src/cli.js", repo), "memro/src/cli.js");
  // Write may target a folder that doesn't exist yet.
  assert.equal(resourceFor(path.join(repo, "new", "deep", "a.go")), "memro/new/deep/a.go");
  const outside = path.join(TEST_DIR, "loose.txt");
  assert.equal(resourceFor(outside), outside);
});

const edit = (file, extra = {}) => ({
  hook_event_name: "PreToolUse", session_id: "abcdef123456", cwd: TEST_DIR, tool_name: "Edit", tool_input: { file_path: file }, ...extra,
});

test("an edit claims the file for this window and passes when granted", async () => {
  serve(() => ({ granted: true }));
  assert.equal(await handle(edit("/x/a.js"), cfg), null);
  assert.equal(calls[0].route, "/coordination/claim");
  assert.equal(calls[0].body.resource, "/x/a.js");
  assert.equal(calls[0].body.session_id, "claude-code:abcdef12");
  assert.equal(sessionOf({ session_id: "abcdef123456" }), "claude-code:abcdef12");
});

test("an edit to a file another session holds is denied, naming the holder", async () => {
  serve(() => ({ granted: false, holder_session: "codex", holder_resource: "/x", intent: "refactor x" }));
  const out = await handle(edit("/x/a.js"), cfg);
  assert.equal(out.hookSpecificOutput.permissionDecision, "deny");
  assert.match(out.hookSpecificOutput.permissionDecisionReason, /\/x\/a\.js \(inside \/x\) is claimed by codex \(refactor x\)/);
});

test("a claim from the shared Claude Code MCP session doesn't block", async () => {
  serve(() => ({ granted: false, holder_session: "claude-code" }));
  assert.equal(await handle(edit("/x/a.js"), cfg), null);
});

test("other tools, missing keys and other events do nothing", async () => {
  serve(() => assert.fail("no call expected"));
  assert.equal(await handle(edit("/x/a.js", { tool_name: "Read" }), cfg), null);
  assert.equal(await handle(edit("/x/a.js"), null), null);
  assert.equal(await handle(edit("/x/a.js", { hook_event_name: "Stop" }), cfg), null);
});

test("session end releases only this window's leases", async () => {
  serve((route) => (route === "/coordination/active"
    ? { claims: [{ resource: "a", session_id: "claude-code:abcdef12" }, { resource: "b", session_id: "codex" }] }
    : { released: true }));
  await handle({ hook_event_name: "SessionEnd", session_id: "abcdef123456" }, cfg);
  assert.deepEqual(calls.slice(1).map((c) => [c.route, c.body.resource, c.body.session_id]), [["/coordination/release", "a", "claude-code:abcdef12"]]);
});

test("installHooks is idempotent, keeps the user's own hooks, and removeHooks undoes it", () => {
  const file = hookSettingsFile("user");
  fs.mkdirSync(path.dirname(file), { recursive: true });
  const mine = { matcher: "Bash", hooks: [{ type: "command", command: "echo hi" }] };
  fs.writeFileSync(file, JSON.stringify({ model: "opus", hooks: { PreToolUse: [mine] } }));

  installHooks();
  installHooks();
  const cfgAfter = JSON.parse(fs.readFileSync(file, "utf8"));
  assert.equal(cfgAfter.hooks.PreToolUse.length, 2);
  assert.deepEqual(cfgAfter.hooks.PreToolUse[0], mine);
  assert.equal(cfgAfter.hooks.SessionEnd.length, 1);
  assert.ok(fs.existsSync(HOOK_FILE()));

  assert.equal(removeHooks(), true);
  assert.deepEqual(JSON.parse(fs.readFileSync(file, "utf8")), { model: "opus", hooks: { PreToolUse: [mine] } });
  assert.equal(removeHooks(), false);
});
