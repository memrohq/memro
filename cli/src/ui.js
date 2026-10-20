// Terminal output for memro-cli: one visual language for the whole flow.
//
//   ┌  Memro connect  api.memro.co
//   │
//   ◇  Where should Memro be set up?
//   │  This project · ~/work/app
//   │
//   └  Done.
//
// Every line is cut to the terminal width, so a frame that is redrawn always
// takes exactly as many rows as it has lines (wrapped lines are what made
// redraws drift and repeat titles).

const ESC = "\x1b[";

/** Colors are used only on a terminal, and never with NO_COLOR set. */
export function useColor(stream = process.stdout) {
  return Boolean(stream.isTTY) && !("NO_COLOR" in process.env) && process.env.TERM !== "dumb";
}

const paint = (open, close) => (s) => (colorOn ? `${ESC}${open}m${s}${ESC}${close}m` : String(s));
let colorOn = useColor();
export function setColor(on) { colorOn = on; }

export const c = {
  dim: paint("2", "22"),
  bold: paint("1", "22"),
  accent: paint("38;5;141", "39"),
  green: paint("32", "39"),
  yellow: paint("33", "39"),
  red: paint("31", "39"),
  gray: paint("90", "39"),
};

export const S = {
  start: "┌",
  bar: "│",
  end: "└",
  active: "◆",
  done: "◇",
  cancel: "■",
  info: "●",
  warn: "▲",
  ok: "✔",
  radioOn: "●",
  radioOff: "○",
  checkOn: "◉",
  checkOff: "○",
  cursor: "›",
};

export const bar = () => c.gray(S.bar);

const ANSI = /\x1b\[[0-9;?]*[A-Za-z]/g;

/** Printable width of a string: ANSI codes take no columns. */
export const visibleLength = (s) => [...String(s).replace(ANSI, "")].length;

/**
 * Cut a line to `width` columns, keeping its ANSI codes and ending with "…"
 * when something was cut. Colors are reset at the cut so they don't bleed.
 */
export function fit(line, width) {
  if (!width || visibleLength(line) <= width) return line;
  const max = Math.max(1, width - 1);
  let out = "";
  let cols = 0;
  for (let i = 0; i < line.length;) {
    ANSI.lastIndex = i;
    const m = ANSI.exec(line);
    if (m && m.index === i) { out += m[0]; i += m[0].length; continue; }
    const ch = String.fromCodePoint(line.codePointAt(i));
    if (cols >= max) break;
    out += ch;
    cols++;
    i += ch.length;
  }
  return out + "…" + (colorOn ? `${ESC}0m` : "");
}

/** Columns available on `stream`, or 0 when unknown (no cutting). */
export const columns = (stream = process.stdout) => (stream.isTTY && stream.columns) || 0;

/**
 * A region of the terminal that can be redrawn in place. Lines are cut to
 * the width, so moving up `rows` always lands on the first line.
 */
export function liveFrame(output = process.stdout) {
  let rows = 0;
  // Up to the frame's first row, column 1 (ESC[G, not "\r": some terminals
  // translate an output CR into a newline), then clear to the end.
  const clear = () => {
    if (rows) output.write(`${ESC}${rows}A${ESC}G${ESC}0J`);
    rows = 0;
  };
  return {
    draw(lines) {
      const width = columns(output);
      clear();
      output.write(lines.map((l) => fit(l, width)).join("\n") + "\n");
      rows = lines.length;
    },
    clear,
  };
}

// --- Plain output, in the same style ------------------------------------------

const out = (s = "") => process.stdout.write(fit(s, columns()) + "\n");

export const intro = (title, detail = "") =>
  out(`${c.gray(S.start)}  ${c.bold(title)}${detail ? "  " + c.dim(detail) : ""}`);
export const gap = () => out(bar());
/** A line inside the current section. */
export const line = (s = "") => out(s ? `${bar()}  ${s}` : bar());
/** A finished step: its question and the answer under it. */
export function answered(title, answer) {
  out(`${c.green(S.done)}  ${title}`);
  if (answer) out(`${bar()}  ${c.dim(answer)}`);
  gap();
}
export const info = (s) => out(`${c.accent(S.info)}  ${s}`);
export const warn = (s) => out(`${c.yellow(S.warn)}  ${s}`);
export const ok = (label, detail = "") => out(`${bar()}  ${c.green(S.ok)} ${label}${detail ? " " + c.dim(detail) : ""}`);
export const fail = (label, detail = "") => out(`${bar()}  ${c.yellow("!")} ${label}${detail ? " " + c.dim(detail) : ""}`);
export const outro = (s, ...more) => {
  out(`${c.gray(S.end)}  ${s}`);
  for (const m of more) out(`   ${c.dim(m)}`);
  out();
};
export const cancelled = (s = "Cancelled. Nothing changed.") => outro(c.dim(s));
