import { test } from "node:test";
import assert from "node:assert/strict";
import { PassThrough } from "node:stream";

import { fit, visibleLength, liveFrame, setColor } from "../src/ui.js";
import { pick, pickOne } from "../src/picker.js";

const strip = (s) => s.replace(/\x1b\[[0-9;?]*[A-Za-z]/g, "");

test("fit cuts to the width, ignoring color codes, and marks the cut", () => {
  setColor(true);
  try {
    const colored = "\x1b[2mConnect tools everywhere on this machine\x1b[22m";
    assert.equal(visibleLength(colored), 40);
    const cut = fit(colored, 20);
    assert.equal(visibleLength(cut), 20);
    assert.ok(strip(cut).endsWith("…"));
    assert.equal(fit("short", 20), "short");
    assert.equal(fit("anything", 0), "anything", "unknown width: unchanged");
  } finally {
    setColor(false);
  }
});

function narrowTTY(cols) {
  const input = new PassThrough();
  input.isTTY = true;
  input.setRawMode = () => input;
  const output = new PassThrough();
  output.isTTY = true;
  output.columns = cols;
  let written = "";
  output.on("data", (d) => { written += d; });
  return { input, output, written: () => written };
}

// Regression: hint lines wider than the terminal wrapped onto two rows, so
// each redraw moved up too few rows and repeated the title.
test("every drawn line fits a narrow terminal, and redraws move up exactly one frame", async () => {
  const { input, output, written } = narrowTTY(30);
  const done = pickOne(
    [
      { label: "All my projects", hint: "Adds Memro to your tools globally, everywhere on this machine", value: "user" },
      { label: "Only this project", hint: "~/Documents/work/some/very/long/project/path", value: "project" },
    ],
    { title: "Where should Memro be set up?", input, output },
  );
  input.write("j");
  input.write("k");
  input.write("\r");
  assert.equal((await done).value, "user");

  const text = written();
  const rows = strip(text.replace(/\x1b\[\d+A\x1b\[G/g, "\n")).split("\n");
  for (const row of rows) assert.ok(visibleLength(row) <= 30, `row wider than the terminal: ${JSON.stringify(row)}`);
  // Each redraw goes up by the rows of the frame it replaces.
  const ups = [...text.matchAll(/\x1b\[(\d+)A/g)].map((m) => Number(m[1]));
  assert.ok(ups.length >= 3, "redrawn on each key and collapsed at the end");
  assert.ok(ups.every((n) => n === 7 || n === 8), `moves up by whole frames: ${ups}`);
});

test("a finished prompt collapses to its question and answer", async () => {
  const { input, output, written } = narrowTTY(80);
  const done = pick(
    [{ label: "Claude Code", checked: true }, { label: "Cursor", checked: false }, { label: "Teach", checked: true, option: "i", summary: "+ instructions" }],
    { title: "Which tools should use Memro?", input, output },
  );
  input.write("\r");
  await done;
  assert.ok(!written().includes("\r"), "no bare carriage returns");
  const tail = strip(written().split(/\x1b\[\d+A\x1b\[G\x1b\[0J/).at(-1));
  assert.match(tail, /◇  Which tools should use Memro\?\n│  Claude Code · \+ instructions\n│\n/);
});
