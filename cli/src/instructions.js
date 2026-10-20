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
- Before answering about earlier work ("what did we decide about X?"), call \`recall\` first.
- Call \`remember\` with one short, self-contained sentence when something will matter in a later session: a decision and why, a fix or workflow worth reusing, a preference or correction from me, or an important project constraint.

Types: \`semantic\` for facts ("Memro uses PostgreSQL"), \`profile\` for my preferences and working style ("User prefers reviewing migrations before deploys"), \`procedural\` for how-tos ("To run integration tests, start PostgreSQL first"), \`episodic\` for significant events ("The billing isolation tests passed"). If unsure, leave the type out and Memro chooses.

Don't save routine events (connecting a tool, starting a session), passing status, facts Memro already has (recall first), or summaries of a whole conversation. Never store secrets (keys, passwords, tokens) or anything I ask you not to keep. Don't announce routine saves.
${END}`;

// Same guidance as BLOCK, for a project's own instructions file. It names the
// tools and says "without being asked": a vaguer note ("recall relevant
// context") left agents saving only when told to.
export const PROJECT_BLOCK = `${START}
## Memro (long-term memory)

Memro is the long-term memory for this project (MCP server \`memro\`). Use it without being asked:
- At the start of a task, call \`recall\` with a short query about it and use what comes back.
- Before answering about earlier work ("what did we decide about X?"), call \`recall\` first.
- Call \`remember\` with one short, self-contained sentence when something will matter in a later session: a decision and why, a fix or workflow worth reusing, a preference or correction from the user, or an important constraint of this project. Name the project in the sentence.

Types: \`semantic\` for facts ("Memro uses PostgreSQL"), \`profile\` for the user's preferences and working style ("User prefers reviewing migrations before deploys"), \`procedural\` for how-tos ("To run integration tests, start PostgreSQL first"), \`episodic\` for significant events ("The billing isolation tests passed"). If unsure, leave the type out and Memro chooses.

Don't save routine events (connecting a tool, starting a session), passing status, facts Memro already has (recall first), or summaries of a whole conversation. Never store secrets (keys, passwords, tokens) or anything this project's rules keep private. Don't announce routine saves.
${END}`;

const globalBlockPattern = new RegExp(`\\n*${START}[\\s\\S]*?${END}\\n*`, "g");

/** Text without the Memro block (removes all instances and cleans up orphan markers). */
export function removeBlock(text) {
  if (!text.includes(START) && !text.includes(END)) return text;
  let out = text.replace(globalBlockPattern, "\n\n");
  // Clean up any orphan markers if one was deleted accidentally
  if (out.includes(START)) {
    out = out.replace(new RegExp(`\\n*${START}[^\\n]*\\n*`, "g"), "\n\n");
  }
  if (out.includes(END)) {
    out = out.replace(new RegExp(`\\n*${END}\\n*`, "g"), "\n\n");
  }
  out = out.replace(/^\n+/, "").replace(/\n{3,}/g, "\n\n");
  return out.trim() ? out.replace(/\s+$/, "") + "\n" : "";
}

/** Text with the Memro block added, or replaced if it is already there. */
export function upsertBlock(text, block = BLOCK) {
  const cleaned = removeBlock(text).replace(/\s+$/, "");
  return (cleaned ? cleaned + "\n\n" : "") + block + "\n";
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

/** Whether text already tells the agent to use Memro, outside our block
 * (e.g. guidance the user wrote themselves). */
export function hasOwnMemroGuidance(text) {
  const rest = removeBlock(text);
  return /memro/i.test(rest) && /\brecall\b/i.test(rest);
}

/**
 * Add or refresh the block in a tool's instructions file. If the file already
 * has its own Memro guidance, leave that in charge: add nothing, and drop an
 * older block of ours so the agent never gets the rules twice. Returns
 * "added", "updated", "unchanged" or "kept-own".
 */
export function installInstructions(file, { scope = "user", block, frontmatter } = {}) {
  const targetBlock = block || (scope === "project" ? PROJECT_BLOCK : BLOCK);
  const before = fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
  if (hasOwnMemroGuidance(before)) {
    const after = removeBlock(before);
    if (after !== before) writeText(file, after);
    return "kept-own";
  }
  let after = upsertBlock(before, targetBlock);
  // Some tools ignore a rules file without frontmatter (Antigravity drops
  // .agents/rules/*.md that lack a `trigger`); add it when the file has none.
  if (frontmatter && !after.startsWith("---")) after = frontmatter + "\n\n" + after;
  if (after === before) return "unchanged";
  writeText(file, after);
  return before.includes(START) ? "updated" : "added";
}

/** Remove the block; returns whether there was one. Deletes the file if
 * Memro's block was all it contained. */
export function uninstallInstructions(file, { frontmatter } = {}) {
  if (!fs.existsSync(file)) return false;
  const before = fs.readFileSync(file, "utf8");
  const after = removeBlock(before);
  if (after === before) return false;
  if (after === "" || (frontmatter && after.trim() === frontmatter.trim())) fs.rmSync(file);
  else writeText(file, after);
  return true;
}
