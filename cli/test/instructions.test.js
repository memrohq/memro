import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { BLOCK, START, upsertBlock, removeBlock, installInstructions, uninstallInstructions } from "../src/instructions.js";

const mine = "# My rules\n\nAlways use tabs.\n";

test("the block is added once, refreshed in place and removed cleanly", () => {
  const once = upsertBlock(mine);
  assert.ok(once.startsWith(mine.trimEnd()));
  assert.equal(once.split(START).length - 1, 1);
  assert.equal(upsertBlock(once), once, "idempotent");
  const stale = once.replace("long-term memory across", "OLD TEXT");
  assert.equal(upsertBlock(stale), once, "an older block is replaced");
  assert.equal(removeBlock(once), mine);
  assert.equal(removeBlock(mine), mine, "no block, no change");
  assert.equal(removeBlock(upsertBlock("")), "");
});

test("the block stays well under Windsurf's 6,000-character limit", () => {
  assert.ok(BLOCK.length < 1500, `block is ${BLOCK.length} chars`);
});

test("install keeps the user's file and permissions; uninstall restores it", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "memro-rules-"));
  const file = path.join(dir, "CLAUDE.md");
  fs.writeFileSync(file, mine, { mode: 0o644 });
  installInstructions(file);
  installInstructions(file);
  const text = fs.readFileSync(file, "utf8");
  assert.ok(text.startsWith(mine.trimEnd()) && text.includes(START));
  assert.equal(text.split(START).length - 1, 1);
  if (process.platform !== "win32") assert.equal(fs.statSync(file).mode & 0o777, 0o644);
  assert.equal(fs.readFileSync(`${file}.memro-backup`, "utf8"), mine);
  assert.equal(uninstallInstructions(file), true);
  assert.equal(fs.readFileSync(file, "utf8"), mine);
  assert.equal(uninstallInstructions(file), false);
});

test("a file Memro created holds only the block and is deleted on uninstall", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "memro-rules-"));
  const file = path.join(dir, "sub", "AGENTS.md");
  installInstructions(file);
  assert.equal(fs.readFileSync(file, "utf8"), BLOCK + "\n");
  assert.equal(uninstallInstructions(file), true);
  assert.equal(fs.existsSync(file), false);
});
