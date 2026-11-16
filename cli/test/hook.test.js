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

process.env.MEMRO_HOOK_WAIT_SECONDS = "0"; // the wait is tested on its own below
const { briefing, handle, handoffNote, herdrOf, lastAssistantText, taskOf, patchFiles, resourceFor, sessionOf, shellFiles, whereOf } = await import("../src/hook.js");
const { HOOK_FILE, hookSettingsFile, installHooks, removeHooks } = await import("../src/clients.js");

const cfg = { api: "https://api.test", apiKey: "memro_sk_test" };
// How the hook names a file outside git: resolved against the agent's folder
// (on Windows that can be another drive than this process's), forward slashes.
// The checkout of /x/a.js is the root: /x doesn't exist, so the nearest folder.
const P = (p, cwd = TEST_DIR) => path.resolve(cwd, p).split(path.sep).join("/");
const rootOf = (cwd) => `${P("/", cwd)}@${os.hostname()}`;
const ROOT_CHECKOUT = rootOf(TEST_DIR);
const A = P("/x/a.js");
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
  assert.equal(resourceFor(outside), P(outside));
});

const edit = (file, extra = {}) => ({
  hook_event_name: "PreToolUse", session_id: "abcdef123456", cwd: TEST_DIR, tool_name: "Edit", tool_input: { file_path: file }, ...extra,
});

test("an edit claims the file for this window and passes when granted", async () => {
  serve(() => ({ granted: true }));
  assert.equal(await handle(edit("/x/a.js"), cfg), null);
  assert.equal(calls[0].route, "/coordination/claim");
  assert.equal(calls[0].body.resource, A);
  assert.equal(calls[0].body.session_id, "claude-code:abcdef12");
  assert.equal(sessionOf({ session_id: "abcdef123456" }), "claude-code:abcdef12");
});

test("an edit to a file another session holds is denied, naming the holder", async () => {
  serve(() => ({ granted: false, holder_session: "codex", holder_resource: "/x", intent: `refactor x [${ROOT_CHECKOUT}]` }));
  const out = await handle(edit("/x/a.js"), cfg);
  assert.equal(out.hookSpecificOutput.permissionDecision, "deny");
  assert.ok(out.hookSpecificOutput.permissionDecisionReason.includes(`${A} (inside /x) is being changed by codex (refactor x). It is released`));
});

test("a held file in the same checkout is waited for, then claimed", async () => {
  process.env.MEMRO_HOOK_WAIT_SECONDS = "5";
  try {
    let n = 0;
    serve(() => (++n === 1 ? { granted: false, holder_session: "codex:1", intent: `x [${ROOT_CHECKOUT}]` } : { granted: true }));
    assert.equal(await handle(edit("/x/a.js"), cfg), null);
    assert.equal(n, 2);
  } finally {
    process.env.MEMRO_HOOK_WAIT_SECONDS = "0";
  }
});

test("a holder in another worktree only warns about a merge conflict", async () => {
  serve(() => ({ granted: false, holder_session: "codex", intent: "Codex editing a.js [/other/worktree@laptop]" }));
  const out = await handle(edit("/x/a.js"), cfg);
  assert.equal(out.hookSpecificOutput.permissionDecision, undefined);
  assert.ok(out.hookSpecificOutput.additionalContext.includes(`also changing ${A} (Codex editing a.js) in another checkout (/other/worktree@laptop)`));
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

const inbox = (msgs) => (route) => (route.startsWith("/coordination/active")
  ? { intents: msgs.map((m) => ({ resource: "inbox/claude-code:feedbeef", ...m })) }
  : { granted: true });
const ev = (hook_event_name, extra = {}) => ({ hook_event_name, session_id: "feedbeef99", cwd: "/w/memro", ...extra });

test("prompt, notification and stop set this window's status", async () => {
  for (const [event, state] of [["UserPromptSubmit", "working"], ["Notification", "waiting for you"], ["Stop", "idle"]]) {
    serve(inbox([]));
    await handle(ev(event), cfg);
    const claim = calls.find((c) => c.route === "/coordination/claim").body;
    assert.equal(claim.resource, "agents/claude-code:feedbeef");
    // Outside git the repo is the folder itself.
    assert.ok(claim.intent.startsWith(`${state} · ${P("/w/memro", "/w/memro")}`), claim.intent);
    assert.equal(whereOf(claim.intent), rootOf("/w/memro"), "the status says which checkout it works in");
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
    assert.equal(herdrOf(intent), `herdr s1 w1:p2@${os.hostname()}`);
    assert.equal(whereOf(intent), rootOf("/w/memro"));
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

const PATCH = `*** Begin Patch
*** Update File: src/a.js
@@
-x
+y
*** Add File: docs/new.md
+hi
*** Update File: old/name.js
*** Move to: new/name.js
*** End Patch`;

test("patchFiles reads every file a Codex apply_patch touches", () => {
  assert.deepEqual(patchFiles(PATCH), ["src/a.js", "docs/new.md", "old/name.js", "new/name.js"]);
  assert.deepEqual(patchFiles(""), []);
});

test("Codex: apply_patch claims each file as codex:<session> and is denied if one is held", async () => {
  const input = { hook_event_name: "PreToolUse", session_id: "c0dex123456", cwd: "/w", tool_name: "apply_patch", tool_input: { command: PATCH } };
  serve(() => ({ granted: true }));
  assert.equal(await handle(input, cfg, "codex"), null);
  assert.deepEqual(calls.map((c) => c.body.resource), ["/w/src/a.js", "/w/docs/new.md", "/w/old/name.js", "/w/new/name.js"].map((f) => P(f, "/w")));
  assert.equal(calls[0].body.session_id, "codex:c0dex123");
  assert.match(calls[0].body.intent, /^Codex editing a\.js/);

  serve((route, body) => (body.resource === P("/w/docs/new.md", "/w") ? { granted: false, holder_session: "claude-code:aaaa1111", intent: `x [${rootOf("/w")}]` } : { granted: true }));
  const out = await handle(input, cfg, "codex");
  assert.ok(out.hookSpecificOutput.permissionDecisionReason.includes(`${P("/w/docs/new.md", "/w")} is being changed by claude-code:aaaa1111`));
  // Claude Code's tools mean nothing to Codex, and vice versa.
  serve(() => assert.fail("no call expected"));
  assert.equal(await handle({ ...input, tool_name: "Bash" }, cfg, "codex"), null);
  assert.equal(await handle({ ...input, tool_name: "apply_patch" }, cfg, "nope"), null);
});

test("Codex hooks go to hooks.json with a 3-second SessionEnd and no Notification", () => {
  process.env.CODEX_HOME = path.join(HOME, ".codex");
  const file = installHooks({ tool: "codex" });
  assert.equal(file, path.join(HOME, ".codex", "hooks.json"));
  const { hooks } = JSON.parse(fs.readFileSync(file, "utf8"));
  assert.deepEqual(Object.keys(hooks), ["PreToolUse", "UserPromptSubmit", "Stop", "SessionEnd"]);
  assert.equal(hooks.PreToolUse[0].matcher, "^apply_patch$|^Bash$|^mcp__memro__");
  assert.match(hooks.Stop[0].hooks[0].command, /memro-hook\.mjs" codex$/);
  assert.equal(hooks.SessionEnd[0].hooks[0].timeout, 3);
  assert.equal(removeHooks({ tool: "codex" }), true);
  assert.equal(fs.existsSync(file), true);
});

test("shellFiles finds what a command writes or deletes", () => {
  assert.deepEqual(shellFiles("echo hi > a.txt && cat b >> 'c d.txt' 2>/dev/null"), ["a.txt", "c d.txt"]);
  assert.deepEqual(shellFiles("npm test | tee out.log"), ["out.log"]);
  assert.deepEqual(shellFiles("sed -i '' 's/a/b/' src/x.js src/y.js"), ["src/x.js", "src/y.js"]);
  assert.deepEqual(shellFiles("sed -i.bak -e s/a/b/ f.txt"), ["f.txt"]);
  assert.deepEqual(shellFiles("rm -rf build/out.js; ls > /dev/null"), ["build/out.js"]);
  assert.deepEqual(shellFiles("ls -la && node --test 2>&1 && echo $HOME > *.txt"), []);
});

const bash = (command, extra = {}) => ({ hook_event_name: "PreToolUse", session_id: "abcdef123456", cwd: "/w/memro", tool_name: "Bash", tool_input: { command }, ...extra });

test("a shell write claims the file like an edit; plain commands make no call", async () => {
  serve(() => ({ granted: true }));
  assert.equal(await handle(bash("node --test && git status"), cfg), null);
  assert.equal(calls.length, 0);
  await handle(bash("echo x > notes.txt"), cfg);
  assert.deepEqual(calls.map((c) => c.body.resource), [P("notes.txt", "/w/memro")]);
});

test("destructive git is refused while another agent works in this checkout", async () => {
  const here = rootOf("/w/memro");
  const others = [
    { resource: "agents/codex:1", session_id: "codex:1", reason: `idle · memro [${here}]` },
    { resource: "memro/src/a.js", session_id: "claude-code:2", reason: `Claude Code editing a.js [${here}]` },
    { resource: "memro/src/b.js", session_id: "claude-code:3", reason: "Claude Code editing b.js [/elsewhere@laptop]" },
  ];
  serve(() => ({ claims: others }));
  for (const cmd of ["git stash", "git stash push -u", "git checkout main", "git checkout -- src/a.js", "git checkout .", "git reset --hard HEAD", "git clean -fd", "git restore src", "git switch dev", "cd x && git -C . stash && ls"]) {
    const out = await handle(bash(cmd), cfg);
    assert.equal(out?.hookSpecificOutput.permissionDecision, "deny", cmd);
  }
  const reason = (await handle(bash("git stash"), cfg)).hookSpecificOutput.permissionDecisionReason;
  assert.ok(reason.includes("codex:1 (idle · memro)") && reason.includes("claude-code:2 (editing memro/src/a.js)"), reason);
  assert.ok(!reason.includes("claude-code:3"), "another checkout is not at risk");

  serve(() => assert.fail("no call expected"));
  for (const cmd of ["git stash list", "git checkout -b feature", "git status", "git commit -am x", "git reset HEAD~1"]) {
    assert.equal(await handle(bash(cmd), cfg), null, cmd);
  }
});

test("destructive git is allowed when nobody else works in this checkout", async () => {
  serve(() => ({ claims: [{ resource: "agents/claude-code:abcdef12", session_id: "claude-code:abcdef12", reason: `working · memro [${rootOf("/w/memro")}]` }] }));
  assert.equal(await handle(bash("git stash"), cfg), null);
});

test("waits, refusals and warnings are logged for the benchmark", async () => {
  const logFile = path.join(HOME, ".memro", "hooks", "log.jsonl");
  serve(() => ({ granted: false, holder_session: "codex", intent: "x [/other@laptop]" }));
  await handle(edit("/x/a.js"), cfg);
  const last = JSON.parse(fs.readFileSync(logFile, "utf8").trim().split("\n").pop());
  assert.deepEqual([last.outcome, last.holder, last.resource], ["warn", "codex", A]);
});

test("Memro's MCP coordination calls get this window as their holder; other MCP calls are left alone", async () => {
  serve(() => assert.fail("no call expected"));
  const mcp = (tool_name, tool_input) => ({ hook_event_name: "PreToolUse", session_id: "abcdef123456", cwd: "/w", tool_name, tool_input });
  const out = await handle(mcp("mcp__memro__claim_task", { resource: "memro/cli", intent: "x" }), cfg);
  assert.deepEqual(out.hookSpecificOutput.updatedInput, { resource: "memro/cli", intent: "x", holder_session: "claude-code:abcdef12" });
  assert.equal(out.hookSpecificOutput.permissionDecision, "allow");
  assert.equal((await handle(mcp("mcp__memro__release_task", { resource: "r" }), cfg, "codex")).hookSpecificOutput.updatedInput.holder_session, "codex:abcdef12");
  assert.equal(await handle(mcp("mcp__memro__recall", { query: "q" }), cfg), null);
  assert.equal(await handle(mcp("mcp__other__claim_task", {}), cfg), null);
});

test("the status names the task from the prompt, and keeps it after the turn", async () => {
  serve(inbox([]));
  await handle(ev("UserPromptSubmit", { prompt: "\n  Add   discount support to total()\nmore detail" }), cfg);
  assert.match(calls.find((c) => c.route === "/coordination/claim").body.intent, / — Add discount support to total\(\) \[/);
  serve(inbox([]));
  await handle(ev("Stop"), cfg);
  assert.match(calls.find((c) => c.route === "/coordination/claim").body.intent, /^idle · .* — Add discount support/);
  assert.equal(taskOf("x".repeat(100)).length, 80);
});

test("a prompt is briefed on other agents in the same repo, with clashes flagged", () => {
  const claims = [
    { resource: "agents/codex:1", session_id: "codex:1", reason: "working · shop — refactor cart [/a@h]" },
    { resource: "shop/src/cart.js", session_id: "codex:1", reason: "Codex editing cart.js [/a@h]" },
    { resource: "agents/claude-code:2", session_id: "claude-code:2", reason: "idle · other-repo — x [/b@h]" },
    { resource: "other-repo/a.js", session_id: "claude-code:2", reason: "x [/b@h]" },
    { resource: "agents/claude-code:me", session_id: "claude-code:me", reason: "working · shop — mine [/a@h]" },
  ];
  const note = briefing(claims, "claude-code:me", "shop", "Add a discount to src/cart.js");
  assert.match(note, /other agents are working in shop/);
  assert.match(note, /- codex:1: working · shop — refactor cart; holds shop\/src\/cart\.js/);
  assert.match(note, /⚠ Your task mentions shop\/src\/cart\.js, which codex:1 holds/);
  assert.ok(!note.includes("claude-code:2") && !note.includes("claude-code:me"));
  assert.equal(briefing(claims.slice(2), "claude-code:me", "shop", ""), null);
});

test("a turn hands its files off with a note: who, the task and what it said", async () => {
  serve((route) => (route === "/coordination/active"
    ? { claims: [{ resource: "shop/a.js", session_id: "claude-code:feedbeef" }, { resource: "agents/claude-code:feedbeef", session_id: "claude-code:feedbeef" }], intents: [] }
    : { granted: true }));
  await handle(ev("UserPromptSubmit", { prompt: "Add coupons" }), cfg);
  serve((route) => (route === "/coordination/active"
    ? { claims: [{ resource: "shop/a.js", session_id: "claude-code:feedbeef" }, { resource: "agents/claude-code:feedbeef", session_id: "claude-code:feedbeef" }], intents: [] }
    : { granted: true }));
  await handle(ev("Stop", { last_assistant_message: "Added  applyCoupon() to a.js;\nkept total() unchanged." }), cfg);
  const handoffs = calls.filter((c) => c.route === "/coordination/handoff").map((c) => [c.body.resource, c.body.summary]);
  assert.deepEqual(handoffs, [["shop/a.js", "claude-code:feedbeef (task: Add coupons): Added applyCoupon() to a.js; kept total() unchanged."]]);
  assert.equal(handoffNote("s", "", ""), "");
});

test("the next agent to claim a handed-off file sees the note once; its own notes are skipped", async () => {
  const note = "codex:11112222 (task: Add coupons): Added applyCoupon()";
  serve(() => ({ granted: true, handoff_summary: note }));
  const first = await handle(edit("/x/n.js"), cfg);
  assert.ok(first.hookSpecificOutput.additionalContext.startsWith(`Memro: the last change to ${P("/x/n.js")} was by ${note} (Memro names files`));
  assert.equal(await handle(edit("/x/n.js"), cfg), null, "shown once");
  serve(() => ({ granted: true, handoff_summary: "claude-code:abcdef12 (task: mine): did it" }));
  assert.equal(await handle(edit("/x/m.js"), cfg), null, "own note");
});

test("lastAssistantText reads the final assistant message from a transcript", () => {
  const f = path.join(TEST_DIR, "t.jsonl");
  fs.writeFileSync(f, [
    JSON.stringify({ type: "assistant", message: { content: [{ type: "text", text: "first" }] } }),
    JSON.stringify({ type: "user", message: { content: "ok" } }),
    JSON.stringify({ type: "assistant", message: { content: [{ type: "tool_use", name: "Edit" }, { type: "text", text: "Done: renamed x." }] } }),
    "",
  ].join("\n"));
  assert.equal(lastAssistantText(f), "Done: renamed x.");
  assert.equal(lastAssistantText(path.join(TEST_DIR, "missing.jsonl")), "");
});
