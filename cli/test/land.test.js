import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const TEST_DIR = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "memro-land-test-")));
process.env.HOME = path.join(TEST_DIR, "home");
process.env.MEMRO_LAND_WAIT_SECONDS = "0";
const { main } = await import("../src/cli.js");

const git = (cwd, ...args) => execFileSync("git", ["-c", "user.name=t", "-c", "user.email=t@t", ...args], { cwd, encoding: "utf8" }).trim();

/** A repo on main with two worktrees, a and b, each changing line 1 of f.txt to its own name. */
function setup(name) {
  const repo = path.join(TEST_DIR, name);
  fs.mkdirSync(repo);
  git(repo, "init", "-q", "-b", "main");
  fs.writeFileSync(path.join(repo, "f.txt"), "one\ntwo\n");
  git(repo, "add", "-A");
  git(repo, "commit", "-qm", "base");
  for (const b of ["a", "b"]) {
    git(repo, "worktree", "add", "-q", "-b", b, path.join(TEST_DIR, `${name}-${b}`));
    fs.writeFileSync(path.join(TEST_DIR, `${name}-${b}`, "f.txt"), `${b}\ntwo\n`);
    git(path.join(TEST_DIR, `${name}-${b}`), "commit", "-qam", `${b} change`);
  }
  return repo;
}

/** Run `land` in a folder against a fake Memro API; returns what it printed and the API calls. */
async function land(cwd, { granted = true } = {}) {
  const calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ route: new URL(url).pathname, body: JSON.parse(init.body || "{}") });
    const data = url.endsWith("/claim") ? { granted, holder_session: "land:other", intent: "landing other on main" } : {};
    return new Response(JSON.stringify(data), { status: 200 });
  };
  const out = [];
  const log = console.log;
  console.log = (s) => out.push(s);
  const was = process.cwd();
  process.chdir(cwd);
  try {
    await main(["land", "--key", "memro_sk_test", "--local"]);
  } finally {
    process.chdir(was);
    console.log = log;
  }
  const code = process.exitCode;
  process.exitCode = 0;
  return { out: out.join("\n"), calls, code };
}

test("land fast-forwards main and hands the lease off with a note", async () => {
  const repo = setup("ok");
  const r = await land(`${repo}-a`);
  assert.match(r.out, /Landed a on main/);
  assert.equal(git(repo, "log", "-1", "--format=%s"), "a change");
  assert.equal(r.calls[0].route, "/coordination/claim");
  assert.equal(r.calls[0].body.resource, "land/ok"); // the repo folder's name, as there's no origin
  assert.match(r.calls.at(-1).body.summary, /land:a landed a: a change/);
});

test("a conflict stops the rebase, keeps the lease and shows what landed", async () => {
  const repo = setup("conflict");
  await land(`${repo}-a`);
  const r = await land(`${repo}-b`);
  assert.equal(r.code, 1);
  assert.match(r.out, /Conflicts rebasing b onto main in: f.txt/);
  assert.match(r.out, /a change/);
  assert.ok(!r.calls.some((c) => c.route === "/coordination/handoff"));
  assert.equal(git(repo, "log", "-1", "--format=%s"), "a change"); // main untouched
});

test("waits its turn: another agent landing leaves the branch untouched", async () => {
  const repo = setup("busy");
  const r = await land(`${repo}-a`, { granted: false });
  assert.equal(r.code, 2);
  assert.match(r.out, /land:other is still landing/);
  assert.equal(git(repo, "log", "-1", "--format=%s"), "base");
});
