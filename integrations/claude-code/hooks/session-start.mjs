// SessionStart hook: recall this project's memories from Memro and hand them
// to Claude with the "use Memro" note, so a session starts with the project's
// decisions instead of relying on the model to call `recall` first. Any failure
// (no key, offline, slow server) ends quietly: memory must never block a session.

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { execFileSync } from "node:child_process";

const API = process.env.MEMRO_API_URL || "https://api.memro.co";
const LIMIT = 8;

// Keep in step with cli/src/instructions.js (PROJECT_BLOCK) and the MCP server's INSTRUCTIONS.
const NOTE = `Memro is the long-term memory for this project (MCP server \`memro\`). Use it without being asked:
- Before answering about earlier work, decisions, rules or preferences, call \`recall\` first, before searching files: Memro holds what was decided, and files can be incomplete.
- Call \`remember\` with one short, self-contained sentence that names the project when something will matter in a later session: a decision and why, a fix or workflow worth reusing, a preference or correction from the user, or an important constraint.
Never store secrets (keys, passwords, tokens). Don't announce routine saves.`;

function apiKey() {
  if (process.env.CLAUDE_PLUGIN_OPTION_API_KEY) return process.env.CLAUDE_PLUGIN_OPTION_API_KEY;
  if (process.env.MEMRO_API_KEY) return process.env.MEMRO_API_KEY;
  try {
    return JSON.parse(fs.readFileSync(path.join(os.homedir(), ".memro", "credentials.json"), "utf8")).api_key;
  } catch {
    return null;
  }
}

/** A name to recall by: the git repository's name, else the folder's. */
function projectName(dir) {
  try {
    const url = execFileSync("git", ["remote", "get-url", "origin"], { cwd: dir, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"], timeout: 2000 }).trim();
    const name = url.replace(/\.git$/, "").split(/[/:]/).pop();
    if (name) return name;
  } catch { /* not a git repo, or no origin */ }
  return path.basename(dir);
}

/** memro-cli may already have added a `memro` server to Claude Code; with the plugin too, every tool appears twice. */
function alsoConnectedByCli() {
  try {
    const cfg = JSON.parse(fs.readFileSync(path.join(os.homedir(), ".claude.json"), "utf8"));
    return Boolean(cfg.mcpServers?.memro);
  } catch {
    return false;
  }
}

async function recall(key, query) {
  const url = `${API}/search?${new URLSearchParams({ query, limit: String(LIMIT), mode: "hybrid" })}`;
  const res = await fetch(url, { headers: { Authorization: `Bearer ${key}` }, signal: AbortSignal.timeout(6000) });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()).results || [];
}

function emit(context, systemMessage) {
  const out = { hookSpecificOutput: { hookEventName: "SessionStart", additionalContext: context } };
  if (systemMessage) out.systemMessage = systemMessage;
  process.stdout.write(JSON.stringify(out));
}

const dir = process.env.CLAUDE_PROJECT_DIR || process.cwd();
const project = projectName(dir);
const warning = alsoConnectedByCli()
  ? "Memro is also set up in ~/.claude.json by memro-cli, so its tools show twice. Run `npx memro-cli disconnect --client claude-code` to keep just the plugin."
  : null;
const key = apiKey();

if (!key) {
  emit(NOTE, "Memro: no API key. Re-enable the plugin in /plugin to enter one, or set MEMRO_API_KEY.");
} else {
  try {
    const results = await recall(key, `${project} project: decisions, preferences, conventions, current work`);
    const lines = results
      .filter((r) => r.content)
      .map((r) => `- [${r.memory_type || "memory"}] ${r.content.replace(/\s+/g, " ").slice(0, 400)}`);
    const recalled = lines.length
      ? `\n\nMemro recalled for "${project}" at the start of this session (search it for more with \`recall\`):\n${lines.join("\n")}`
      : "";
    emit(NOTE + recalled, warning);
  } catch {
    emit(NOTE, warning); // offline or slow: the note alone still teaches Claude to use the tools
  }
}
