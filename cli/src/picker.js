// A small multi-select for the terminal: arrow keys (or j/k) move, space
// toggles, "a" toggles all, Enter confirms, Esc/q/Ctrl-C cancels. No
// dependencies; the state logic is pure so it can be tested without a TTY.

import readline from "node:readline";

const ESC = "\x1b[";
const c = {
  dim: (s) => `${ESC}2m${s}${ESC}22m`,
  bold: (s) => `${ESC}1m${s}${ESC}22m`,
  accent: (s) => `${ESC}38;5;141m${s}${ESC}39m`,
  green: (s) => `${ESC}32m${s}${ESC}39m`,
};

/** Initial state: items are { label, hint?, checked }. */
export function initialState(items) {
  return { items: items.map((i) => ({ ...i })), cursor: 0, done: false, cancelled: false };
}

/** Apply one key ({ name, ctrl, sequence }) to the state. */
export function reduce(state, key) {
  const s = { ...state, items: state.items.map((i) => ({ ...i })) };
  const n = s.items.length;
  const name = key.name || key.sequence;
  if ((key.ctrl && name === "c") || name === "escape" || name === "q") {
    s.cancelled = true;
  } else if (name === "return" || name === "enter") {
    s.done = true;
  } else if (name === "up" || name === "k") {
    s.cursor = (s.cursor - 1 + n) % n;
  } else if (name === "down" || name === "j" || name === "tab") {
    s.cursor = (s.cursor + 1) % n;
  } else if (name === "space" || name === " ") {
    s.items[s.cursor].checked = !s.items[s.cursor].checked;
  } else if (name === "a") {
    const all = s.items.every((i) => i.checked);
    s.items.forEach((i) => { i.checked = !all; });
  }
  return s;
}

/** The lines to draw for a state. */
export function render(state, title) {
  const lines = [`  ${c.bold(title)}`, c.dim("  ↑/↓ move · space select · a all · enter confirm · esc cancel"), ""];
  state.items.forEach((item, i) => {
    const here = i === state.cursor;
    const box = item.checked ? c.green("◉") : c.dim("○");
    const label = here ? c.accent(item.label) : item.label;
    const hint = item.hint ? "  " + c.dim(item.hint) : "";
    lines.push(`  ${here ? c.accent("›") : " "} ${box} ${label}${hint}`);
  });
  const count = state.items.filter((i) => i.checked).length;
  lines.push("", c.dim(`  ${count} selected`));
  return lines;
}

/**
 * Show the picker and resolve with the selected items, or null if the user
 * cancelled. Requires an interactive terminal on `input`.
 */
export function pick(items, { title, input = process.stdin, output = process.stdout } = {}) {
  return new Promise((resolve) => {
    let state = initialState(items);
    let drawn = 0;
    const draw = () => {
      const lines = render(state, title);
      if (drawn) output.write(`${ESC}${drawn}A`); // back to the top of the last frame
      output.write(lines.map((l) => `${ESC}2K${l}`).join("\n") + "\n");
      drawn = lines.length;
    };
    const finish = (value) => {
      input.off("keypress", onKey);
      if (input.isTTY) input.setRawMode(false);
      input.pause();
      output.write(`${ESC}?25h`); // show the cursor again
      resolve(value);
    };
    const onKey = (_str, key = {}) => {
      state = reduce(state, key);
      if (state.cancelled) return finish(null);
      if (state.done) return finish(state.items.filter((i) => i.checked));
      draw();
    };
    readline.emitKeypressEvents(input);
    if (input.isTTY) input.setRawMode(true);
    input.resume();
    output.write(`${ESC}?25l`); // hide the cursor while choosing
    input.on("keypress", onKey);
    draw();
  });
}
