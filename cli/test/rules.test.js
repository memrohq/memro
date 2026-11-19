import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const TEST_DIR = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "memro-rules-test-")));
process.env.HOME = path.join(TEST_DIR, "home");
const { main } = await import("../src/cli.js");
const { repoOf, rulePrefix } = await import("../src/hook.js");

/** Run `rules …` in a plain folder against a fake Memro API holding two rules. */
async function rules(...args) {
  const repo = repoOf(TEST_DIR);
  const calls = [];
  globalThis.fetch = async (url, init = {}) => {
    const u = new URL(url);
    calls.push({ method: init.method || "GET", route: u.pathname, query: u.searchParams.get("repo"), body: JSON.parse(init.body || "{}") });
    const data = u.pathname === "/rules"
      ? [{ id: "new-id", content: rulePrefix(repo) + "never use Intl in cart code" }, { id: "old-id", content: rulePrefix(repo) + "always run the tests" }]
      : {};
    return new Response(JSON.stringify(data), { status: 200 });
  };
  const out = [];
  const log = console.log;
  console.log = (s) => out.push(s);
  const was = process.cwd();
  process.chdir(TEST_DIR);
  try {
    await main(["rules", ...args, "--key", "memro_sk_test", "--local"]);
  } finally {
    process.chdir(was);
    console.log = log;
  }
  return { out: out.join("\n"), calls, repo };
}

test("rules lists this repo's rules without the prefix, numbered newest first", async () => {
  const r = await rules();
  assert.equal(r.calls[0].route, "/rules");
  assert.equal(r.calls[0].query, r.repo);
  assert.match(r.out, /1\. never use Intl in cart code/);
  assert.match(r.out, /2\. always run the tests/);
  assert.doesNotMatch(r.out, /Rule for/);
});

test("rules delete <n> deletes that rule", async () => {
  const r = await rules("delete", "2");
  assert.deepEqual(r.calls.at(-1), { method: "DELETE", route: "/memory/old-id", query: null, body: {} });
  assert.match(r.out, /Deleted: always run the tests/);
});

test("rules edit <n> keeps the repo prefix so the hook still shows it", async () => {
  const r = await rules("edit", "1", "never", "use", "Intl", "anywhere");
  const put = r.calls.at(-1);
  assert.equal(put.method, "PUT");
  assert.equal(put.route, "/memory/new-id");
  assert.equal(put.body.content, rulePrefix(r.repo) + "never use Intl anywhere");
});

test("rules rejects a number that isn't in the list", async () => {
  await assert.rejects(rules("delete", "3"), /No rule 3/);
  await assert.rejects(rules("delete", "x"), /No rule x/);
});
