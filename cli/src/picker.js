// Small prompts for the terminal: multi-select, single-select and text. No
// dependencies; the state logic is pure so it can be tested without a TTY.
//
// Keys: ↑/↓ (or j/k) move, space toggles, "a" toggles all, Enter confirms,
// Esc/q/Ctrl-C cancels. While a prompt is open it is drawn as an active step
// (◆); once answered it collapses to its question and answer (◇), so the
// screen keeps a short record instead of every frame.

import readline from "node:readline";

import { c, S, bar, liveFrame } from "./ui.js";

const ESC = "\x1b[";

/** Initial state: items are { label, hint?, checked }. */
export function initialState(items) {
  return { items: items.map((i) => ({ ...i })), cursor: 0, done: false, cancelled: false };
}

const isCancel = (key, name) => (key.ctrl && name === "c") || name === "escape" || name === "q";

/** Apply one key ({ name, ctrl, sequence }) to the state. */
export function reduce(state, key) {
  const s = { ...state, items: state.items.map((i) => ({ ...i })) };
  const n = s.items.length;
  const name = key.name || key.sequence;
  if (isCancel(key, name)) {
    s.cancelled = true;
  } else if (name === "return" || name === "enter") {
    s.done = true;
  } else if (name === "up" || name === "k") {
    s.cursor = (s.cursor - 1 + n) % n;
  } else if (name === "down" || name === "j" || name === "tab") {
    s.cursor = (s.cursor + 1) % n;
  } else if (name === "space" || name === " ") {
    s.items[s.cursor].checked = !s.items[s.cursor].checked;
  } else if (name === "a") { // all tools; options below the list keep their state
    const tools = s.items.filter((i) => !i.option);
    const all = tools.every((i) => i.checked);
    tools.forEach((i) => { i.checked = !all; });
  }
  return s;
}

const header = (title, keys) => [`${c.accent(S.active)}  ${c.bold(title)}`, `${bar()}  ${c.dim(keys)}`, bar()];

/** The lines to draw for a state. */
export function render(state, title) {
  const lines = header(title, "↑/↓ move · space select · a all · enter confirm · esc cancel");
  state.items.forEach((item, i) => {
    if (item.option && !state.items[i - 1]?.option) lines.push(bar()); // options sit below the list
    const here = i === state.cursor;
    const box = item.checked ? c.green(S.checkOn) : c.dim(S.checkOff);
    const label = here ? c.accent(item.label) : item.checked ? item.label : c.dim(item.label);
    const hint = item.hint ? "  " + c.dim(item.hint) : "";
    lines.push(`${bar()}  ${here ? c.accent(S.cursor) : " "} ${box} ${label}${hint}`);
  });
  const count = state.items.filter((i) => i.checked && !i.option).length;
  lines.push(bar(), `${c.gray(S.end)}  ${c.dim(`${count} selected`)}`);
  return lines;
}

/** One-line answer for a finished multi-select. */
export function summary(state) {
  const tools = state.items.filter((i) => i.checked && !i.option).map((i) => i.label);
  const options = state.items.filter((i) => i.checked && i.option).map((i) => i.summary || i.label);
  return [tools.join(", ") || "none", ...options].join(" · ");
}

/**
 * Run a prompt: draw it, apply keys, and on Enter/cancel replace the frame
 * with the collapsed step. Resolves with onDone(state) or null.
 */
function run({ state, reduceFn, renderFn, title, answer, onDone, input, output }) {
  return new Promise((resolve) => {
    const frame = liveFrame(output);
    const draw = () => frame.draw(renderFn(state, title));
    const finish = (value, collapsed) => {
      input.off("keypress", onKey);
      output.off?.("resize", draw);
      if (input.isTTY) input.setRawMode(false);
      input.pause();
      frame.draw(collapsed);
      output.write(`${ESC}?25h`); // show the cursor again
      resolve(value);
    };
    const onKey = (_str, key = {}) => {
      state = reduceFn(state, key);
      if (state.cancelled) return finish(null, [`${c.red(S.cancel)}  ${title}`, `${bar()}  ${c.dim("Cancelled")}`]);
      if (state.done) return finish(onDone(state), [`${c.green(S.done)}  ${title}`, `${bar()}  ${c.dim(answer(state))}`, bar()]);
      draw();
    };
    readline.emitKeypressEvents(input);
    if (input.isTTY) input.setRawMode(true);
    input.resume();
    output.write(`${ESC}?25l`); // hide the cursor while choosing
    input.on("keypress", onKey);
    output.on?.("resize", draw);
    draw();
  });
}

/**
 * Show the picker and resolve with the selected items, or null if the user
 * cancelled. Requires an interactive terminal on `input`.
 */
export function pick(items, { title, input = process.stdin, output = process.stdout } = {}) {
  return run({
    state: initialState(items), reduceFn: reduce, renderFn: render, title, input, output,
    answer: summary,
    onDone: (s) => s.items.filter((i) => i.checked),
  });
}

/** Initial state for single-select: items are { label, hint?, value }. */
export function initialSingleState(items, defaultIndex = 0) {
  return { items: items.map((i) => ({ ...i })), cursor: defaultIndex, done: false, cancelled: false };
}

/** Apply key to single-select state. */
export function reduceSingle(state, key) {
  const s = { ...state, items: state.items.map((i) => ({ ...i })) };
  const n = s.items.length;
  const name = key.name || key.sequence;
  if (isCancel(key, name)) {
    s.cancelled = true;
  } else if (name === "return" || name === "enter") {
    s.done = true;
  } else if (name === "up" || name === "k") {
    s.cursor = (s.cursor - 1 + n) % n;
  } else if (name === "down" || name === "j" || name === "tab") {
    s.cursor = (s.cursor + 1) % n;
  }
  return s;
}

/** Render lines for single-select. The hint shows for the highlighted choice. */
export function renderSingle(state, title) {
  const lines = header(title, "↑/↓ move · enter select · esc cancel");
  state.items.forEach((item, i) => {
    const here = i === state.cursor;
    const dot = here ? c.green(S.radioOn) : c.dim(S.radioOff);
    const label = here ? c.accent(item.label) : c.dim(item.label);
    lines.push(`${bar()}  ${here ? c.accent(S.cursor) : " "} ${dot} ${label}`);
    if (here && item.hint) lines.push(`${bar()}      ${c.dim(item.hint)}`);
  });
  lines.push(c.gray(S.end));
  return lines;
}

/**
 * Show a single-select picker and resolve with the chosen item, or null if cancelled.
 */
export function pickOne(items, { title, defaultIndex = 0, input = process.stdin, output = process.stdout } = {}) {
  return run({
    state: initialSingleState(items, defaultIndex), reduceFn: reduceSingle, renderFn: renderSingle, title, input, output,
    answer: (s) => s.items[s.cursor].summary || s.items[s.cursor].label,
    onDone: (s) => s.items[s.cursor],
  });
}

/**
 * Ask a simple text prompt.
 */
export function promptText(query, { input = process.stdin, output = process.stdout } = {}) {
  return new Promise((resolve) => {
    const rl = readline.createInterface({ input, output });
    rl.question(query, (ans) => {
      rl.close();
      resolve(ans.trim());
    });
  });
}
