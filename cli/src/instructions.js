// The "use Memro automatically" note `memro connect` adds to each tool's
// global instructions file, between markers so it can be updated in place and
// removed by `memro disconnect`. The MCP server sends the same guidance to
// every client on connect (mcp/src/memro_mcp/server.py INSTRUCTIONS); this
// covers tools that ignore server instructions. Keep the two in step, and
// keep this short: Windsurf caps global rules at 6,000 characters.

import fs from "node:fs";
import path from "node:path";

export const START = "<!-- memro:start -->";
export const END = "<!-- memro:end -->";

export const BLOCK = `${START}
## Memro (long-term memory)

Memro is my long-term memory across tools and sessions (MCP server \`memro\`). Use it without being asked:
- At the start of a task, call \`recall\` with a short query about it and use what comes back.
- When I state a preference, make a decision, correct you, or you learn a lasting fact about me or a project, call \`remember\` with one short, self-contained sentence.
- Before answering about earlier work ("what did we decide about X?"), \`recall\` first.

Never store secrets (keys, passwords, tokens), one-off chatter, or anything I ask you not to keep. Don't announce routine saves.
${END}`;

const blockPattern = new RegExp(`\\n*${START}[\\s\\S]*?${END}\\n*`);

/** Text with the Memro block added, or replaced if it is already there. */
export function upsertBlock(text) {
  if (blockPattern.test(text)) return text.replace(blockPattern, `\n\n${BLOCK}\n`).replace(/^\n+/, "");
  const body = text.replace(/\s+$/, "");
  return (body ? body + "\n\n" : "") + BLOCK + "\n";
}

/** Text without the Memro block (unchanged if it has none). */
export function removeBlock(text) {
  if (!blockPattern.test(text)) return text;
  const out = text.replace(blockPattern, "\n\n").replace(/^\n+/, "").replace(/\n{3,}/g, "\n\n");
  return out.trim() ? out.replace(/\s+$/, "") + "\n" : "";
}

/** Atomically replace a text file, keeping a one-time backup of the original
 * and the file's existing permissions. */
function writeText(file, text) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  const existed = fs.existsSync(file);
  if (existed && !fs.existsSync(`${file}.memro-backup`)) fs.copyFileSync(file, `${file}.memro-backup`);
  const tmp = `${file}.memro-tmp-${process.pid}`;
  try {
    fs.writeFileSync(tmp, text, existed ? { mode: fs.statSync(file).mode & 0o777 } : {});
    fs.renameSync(tmp, file);
  } finally {
    fs.rmSync(tmp, { force: true });
  }
}

/** Add or refresh the block in a tool's instructions file. */
export function installInstructions(file) {
  const before = fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
  const after = upsertBlock(before);
  if (after !== before) writeText(file, after);
}

/** Remove the block; returns whether there was one. Deletes the file if
 * Memro's block was all it contained. */
export function uninstallInstructions(file) {
  if (!fs.existsSync(file)) return false;
  const before = fs.readFileSync(file, "utf8");
  const after = removeBlock(before);
  if (after === before) return false;
  if (after === "") fs.rmSync(file);
  else writeText(file, after);
  return true;
}
