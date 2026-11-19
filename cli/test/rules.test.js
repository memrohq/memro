import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const TEST_DIR = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "memro-rules-test-")));
process.env.HOME = path.join(TEST_DIR, "home");
const { main } = await import("../src/cli.js");
const { addRules, readRules, repoOf, rulePrefix } = await import("../src/hook.js");

/** Run `rules …` in a fresh folder holding two rules in .memro/rules.md,
 * against a fake Memro API whose account holds `older` rules. */
async function rules(args, { older = [], dir = fs.mkdtempSync(path.join(TEST_DIR, "repo-")), key = true } = {}) {
  if (!fs.existsSync(path.join(dir, ".memro"))) addRules(dir, ["always run the tests", "never use Intl in cart code"]);
  const calls = [];
  globalThis.fetch = async (url, init = {}) => {
    const u = new URL(url);
    calls.push({ method: init.method || "GET", route: u.pathname });
    const data = u.pathname === "/rules" ? older.map((t, i) => ({ id: `id-${i}`, content: rulePrefix(repoOf(dir)) + t })) : {};
    return new Response(JSON.stringify(data), { status: 200 });
  };
  const out = [];
  const log = console.log;
  console.log = (s) => out.push(s);
  const was = process.cwd();
  process.chdir(dir);
  try {
    await main(["rules", ...args, ...(key ? ["--key", "memro_sk_test"] : []), "--local"]);
  } finally {
    process.chdir(was);
    console.log = log;
  }
  return { out: out.join("\n"), calls, dir };
}

test("addRules creates the file with a header, appends new rules once, and readRules reads them back", () => {
  const dir = fs.mkdtempSync(path.join(TEST_DIR, "add-"));
  addRules(dir, ["never  use Intl", "never use Intl"]);
  addRules(dir, ["never use Intl", "always round down"]);
  const text = fs.readFileSync(path.join(dir, ".memro", "rules.md"), "utf8");
  assert.match(text, /^# Rules for AI agents/);
  assert.deepEqual(readRules(dir), ["never use Intl", "always round down"]);
});

test("rules lists the file's rules, numbered, and works without an account", async () => {
  const r = await rules([], { key: false });
  assert.match(r.out, /1\. always run the tests/);
  assert.match(r.out, /2\. never use Intl in cart code/);
  assert.equal(r.calls.length, 0);
});

test("rules also lists account rules not yet in the file, with how to import them", async () => {
  const r = await rules([], { older: ["always run the tests", "never deploy on Fridays"] });
  assert.match(r.out, /rules import/);
  assert.match(r.out, /never deploy on Fridays/);
  assert.equal(r.out.match(/always run the tests/g).length, 1, "a rule already in the file isn't listed twice");
});

test("rules edit and delete change only that line of the file", async () => {
  const { dir } = await rules(["edit", "2", "never", "use", "Intl", "anywhere"]);
  assert.deepEqual(readRules(dir), ["always run the tests", "never use Intl anywhere"]);
  await rules(["delete", "1"], { dir });
  assert.deepEqual(readRules(dir), ["never use Intl anywhere"]);
  assert.match(fs.readFileSync(path.join(dir, ".memro", "rules.md"), "utf8"), /^# Rules for AI agents/, "the header stays");
});

test("rules import moves account rules into the file and removes them from the account", async () => {
  const r = await rules(["import"], { older: ["never deploy on Fridays"] });
  assert.deepEqual(readRules(r.dir), ["always run the tests", "never use Intl in cart code", "never deploy on Fridays"]);
  assert.deepEqual(r.calls.at(-1), { method: "DELETE", route: "/memory/id-0" });
});

test("rules rejects a number that isn't in the file", async () => {
  await assert.rejects(rules(["delete", "3"]), /No rule 3/);
  await assert.rejects(rules(["edit", "x", "y"]), /No rule x/);
});
