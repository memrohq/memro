import { test } from "node:test";
import assert from "node:assert/strict";
import { PassThrough } from "node:stream";

import { initialState, reduce, render, pick } from "../src/picker.js";

const items = [
  { label: "Claude Code", checked: true },
  { label: "Cursor", checked: false, hint: "not detected" },
  { label: "Codex", checked: true },
];
const key = (name, extra = {}) => ({ name, ...extra });
const strip = (s) => s.replace(/\x1b\[[0-9;?]*[A-Za-z]/g, "");

test("moving wraps around and space toggles the current item", () => {
  let s = initialState(items);
  s = reduce(s, key("up"));
  assert.equal(s.cursor, 2);
  s = reduce(s, key("down"));
  s = reduce(s, key("down"));
  assert.equal(s.cursor, 1);
  s = reduce(s, key("space"));
  assert.equal(s.items[1].checked, true);
  assert.equal(items[1].checked, false, "the caller's items are not mutated");
});

test("a toggles all, enter confirms, escape and ctrl-c cancel", () => {
  let s = reduce(initialState(items), key("a"));
  assert.ok(s.items.every((i) => i.checked));
  s = reduce(s, key("a"));
  assert.ok(s.items.every((i) => !i.checked));
  assert.equal(reduce(s, key("return")).done, true);
  assert.equal(reduce(s, key("escape")).cancelled, true);
  assert.equal(reduce(s, key("c", { ctrl: true })).cancelled, true);
});

test("render shows the selection, the cursor and hints", () => {
  const lines = render(initialState(items), "Pick").map(strip);
  assert.match(lines[0], /Pick/);
  assert.ok(lines.some((l) => l.includes("› ◉ Claude Code")));
  assert.ok(lines.some((l) => l.includes("○ Cursor  not detected")));
  assert.match(lines.at(-1), /2 selected/);
});

function fakeTTY() {
  const input = new PassThrough();
  input.isTTY = true;
  input.setRawMode = () => input;
  const output = new PassThrough();
  output.resume(); // discard what is drawn
  return { input, output };
}

test("pick resolves with the chosen items after real keypresses", async () => {
  const { input, output } = fakeTTY();
  const done = pick(items, { title: "Pick", input, output });
  input.write("j");  // down to Cursor
  input.write(" ");  // select it
  input.write("\r"); // confirm
  const chosen = await done;
  assert.deepEqual(chosen.map((i) => i.label), ["Claude Code", "Cursor", "Codex"]);
});

test("pick resolves null when cancelled", async () => {
  const { input, output } = fakeTTY();
  const done = pick(items, { title: "Pick", input, output });
  input.write("q");
  assert.equal(await done, null);
});

test("options sit below the tools and are not counted or toggled by a", () => {
  const withOption = [...items, { label: "Teach tools", checked: true, option: "instructions" }];
  let s = reduce(initialState(withOption), key("a"));
  assert.equal(s.items.at(-1).checked, true, "a leaves options alone");
  const lines = render(initialState(withOption), "Pick").map(strip);
  assert.match(lines.at(-1), /2 selected/);
  assert.equal(lines[lines.findIndex((l) => l.includes("Teach tools")) - 1], "");
});

test("pickOne handles arrow navigation, selection, and cancellation", async () => {
  const { pickOne, initialSingleState, reduceSingle, renderSingle } = await import("../src/picker.js");
  const singleItems = [
    { label: "Global", value: "user" },
    { label: "Project", value: "project" },
  ];

  let s = initialSingleState(singleItems, 0);
  assert.equal(s.cursor, 0);
  s = reduceSingle(s, key("down"));
  assert.equal(s.cursor, 1);
  s = reduceSingle(s, key("return"));
  assert.equal(s.done, true);

  const lines = renderSingle(initialSingleState(singleItems), "Choose").map(strip);
  assert.match(lines[0], /Choose/);
  assert.ok(lines.some((l) => l.includes("Global")));

  const { input, output } = fakeTTY();
  const done = pickOne(singleItems, { title: "Choose", input, output });
  input.write("j");  // down to Project
  input.write("\r"); // select it
  const chosen = await done;
  assert.equal(chosen?.value, "project");

  const { input: in2, output: out2 } = fakeTTY();
  const cancelled = pickOne(singleItems, { title: "Choose", input: in2, output: out2 });
  in2.write("q");
  assert.equal(await cancelled, null);
});

test("promptText asks a question and returns trimmed text", async () => {
  const { promptText } = await import("../src/picker.js");
  const { input, output } = fakeTTY();
  const prom = promptText("Enter key: ", { input, output });
  input.write("  memro_sk_prompt_test  \r");
  assert.equal(await prom, "memro_sk_prompt_test");
});

