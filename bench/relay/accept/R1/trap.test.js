import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";

test("B used no Intl or toLocaleString anywhere in src (POS devices have no ICU)", () => {
  const dir = new URL("../src/", import.meta.url);
  for (const f of fs.readdirSync(dir)) {
    // Comments may mention Intl ("no Intl here"); only code counts.
    const code = fs.readFileSync(new URL(f, dir), "utf8").replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/.*$/gm, "");
    assert.doesNotMatch(code, /\bIntl\s*\.|\.toLocaleString\s*\(/, f);
  }
});
