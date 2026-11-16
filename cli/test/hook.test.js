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

const { handle, resourceFor, sessionOf, whereOf } = await import("../src/hook.js");
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
  const here = `/@${os.hostname()}`; // /x does not exist, so its checkout is the nearest existing folder
  serve(() => ({ granted: false, holder_session: "codex", holder_resource: "/x", intent: `refactor x [${here}]` }));
  const out = await handle(edit("/x/a.js"), cfg);
  assert.equal(out.hookSpecificOutput.permissionDecision, "deny");
  assert.match(out.hookSpecificOutput.permissionDecisionReason, /\/x\/a\.js \(inside \/x\) is being changed by codex \(refactor x\)\. It is released/);
});

test("a holder in another worktree only warns about a merge conflict", async () => {
  serve(() => ({ granted: false, holder_session: "codex", intent: "Codex editing a.js [/other/worktree@laptop]" }));
  const out = await handle(edit("/x/a.js"), cfg);
  assert.equal(out.hookSpecificOutput.permissionDecision, undefined);
  assert.match(out.hookSpecificOutput.additionalContext, /also changing \/x\/a\.js \(Codex editing a\.js\) in another checkout \(\/other\/worktree@laptop\)/);
});

test("a claim from the shared Claude Code MCP session doesn't block", async () => {
  serve(() => ({ granted: false, holder_session: "claude-code" }));
  assert.equal(await handle(edit("/x/a.js"), cfg), null);
});

test("other tools, missing keys and other events do nothing", async () => {
  serve(() => assert.fail("no call expected"));
  assert.equal(await handle(edit("/x/a.js", { tool_name: "Read" }), cfg), null);
  assert.equal(await handle(edit("/x/a.js"), null), null);
  assert.equal(await handle(edit("/x/a.js", { hook_event_name: "PostToolUse" }), cfg), null);
});

const inbox = (msgs) => (route) => (route.startsWith("/coordination/active") ? { intents: msgs } : { granted: true });
const ev = (hook_event_name, extra = {}) => ({ hook_event_name, session_id: "feedbeef99", cwd: "/w/memro", ...extra });

test("prompt, notification and stop set this window's status", async () => {
  for (const [event, state] of [["UserPromptSubmit", "working"], ["Notification", "waiting for you"], ["Stop", "idle"]]) {
    serve(inbox([]));
    await handle(ev(event), cfg);
    const claim = calls.find((c) => c.route === "/coordination/claim").body;
    assert.deepEqual([claim.resource, claim.intent], ["agents/claude-code:feedbeef", `${state} · memro`]);
    if (event === "Stop") assert.ok(calls.some((c) => c.route === "/coordination/active"), "stop also frees files");
  }
});

test("a stop with no messages releases this window's files but keeps its status", async () => {
  serve((route) => (route === "/coordination/active"
    ? { claims: [{ resource: "memro/a.js", session_id: "claude-code:feedbeef" }, { resource: "agents/claude-code:feedbeef", session_id: "claude-code:feedbeef" }, { resource: "memro/b.js", session_id: "codex" }] }
    : route.startsWith("/coordination/active") ? { intents: [] } : { granted: true }));
  await handle(ev("Stop"), cfg);
  assert.deepEqual(calls.filter((c) => c.route === "/coordination/release").map((c) => c.body.resource), ["memro/a.js"]);
});

test("inside herdr the status records the pane", async () => {
  Object.assign(process.env, { HERDR_ENV: "1", HERDR_SESSION: "s1", HERDR_PANE_ID: "w1:p2" });
  try {
    serve(inbox([]));
    await handle(ev("UserPromptSubmit"), cfg);
    const intent = calls.find((c) => c.route === "/coordination/claim").body.intent;
    assert.equal(whereOf(intent), `herdr s1 w1:p2@${os.hostname()}`);
  } finally {
    for (const k of ["HERDR_ENV", "HERDR_SESSION", "HERDR_PANE_ID"]) delete process.env[k];
  }
});

test("a message is added to the next prompt once, and wakes a stopping agent", async () => {
  serve(inbox([{ id: "m1", session_id: "codex", intent: "release memro/cli please" }]));
  const out = await handle(ev("UserPromptSubmit"), cfg);
  assert.match(out.hookSpecificOutput.additionalContext, /from codex: release memro\/cli please/);
  assert.match(out.hookSpecificOutput.additionalContext, /--from claude-code:feedbeef/);
  assert.equal(await handle(ev("Stop"), cfg), null, "already seen");

  serve(inbox([{ id: "m2", session_id: "user", intent: "check the build" }]));
  assert.equal((await handle(ev("Stop"), cfg)).decision, "block");
  serve(inbox([{ id: "m3", session_id: "codex", intent: "ping" }]));
  assert.equal(await handle(ev("Stop", { stop_hook_active: true }), cfg), null, "no wake loops");
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
  for (const event of ["UserPromptSubmit", "Notification", "Stop", "SessionEnd"]) assert.equal(cfgAfter.hooks[event].length, 1);
  assert.ok(fs.existsSync(HOOK_FILE()));

  assert.equal(removeHooks(), true);
  assert.deepEqual(JSON.parse(fs.readFileSync(file, "utf8")), { model: "opus", hooks: { PreToolUse: [mine] } });
  assert.equal(removeHooks(), false);
});
