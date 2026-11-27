// Claude Code and Codex hook that claims every file before the agent edits it,
// so coordination no longer depends on the model remembering to call
// claim_task (memro ADR 0019). `memro connect` copies this file to
// ~/.memro/hooks/ and runs it as `node memro-hook.mjs <claude-code|codex>`, so
// it must import nothing but Node built-ins. The two tools share a hook format;
// they differ in how an edit names its files (below) and in events (Codex has
// no Notification).
//
// PreToolUse (Claude Code's Edit, Write, MultiEdit, NotebookEdit; Codex's
// apply_patch): claim each file for this session. Re-claiming extends the lease, so editing is the
// heartbeat. A file another session holds in the same checkout is denied with
// who holds it; in another worktree or clone the edit goes ahead with a
// warning, because separate branches only meet at merge time.
// Status, as herdr shows it: UserPromptSubmit → working, Notification →
// waiting for the user, Stop → idle. The status is a lease on
// agents/<session> whose intent is "<state> · <folder>", so `memro agents`
// and get_active_work list it, and it disappears when the window closes.
// Messages: `memro send <session> "…"` leaves an intent on inbox/<session>.
// This window reads its inbox when a prompt is submitted (the message is
// added to the prompt) and when it stops (the message wakes it to act on it).
// Files are held for one turn: a Stop with no new messages releases them, so
// an idle agent never blocks anyone and nobody has to ask it to let go.
// Inside herdr the status also records the pane, so `memro send` can prompt an
// idle agent on this machine through herdr (it runs no hooks while idle).
// SessionEnd: release every lease this session holds.
//
// Any failure (no key, offline, slow server) lets the edit through: a memory
// service being down must never stop someone's work.

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";

const CLAUDE_EDITS = new Set(["Edit", "Write", "MultiEdit", "NotebookEdit"]);
const TIMEOUT_MS = 3000;
const STATUS_TTL = 7200; // the server's maximum; SessionEnd clears it sooner
// A held file is usually freed within a minute (files are held for one turn),
// so wait for it before refusing: an agent running on its own would otherwise
// give up on its task. MEMRO_HOOK_WAIT_SECONDS changes it; keep it below the
// PreToolUse hook timeout in clients.js (90 s).
const waitMs = () => Number(process.env.MEMRO_HOOK_WAIT_SECONDS ?? 60) * 1000;
// Each edit holds its file this long, and the next edit renews it, so a file
// the agent is done with frees up soon instead of at the end of its turn (the
// benchmark spent half its time waiting on turn-long holds). Uncommitted work
// stays protected from git by the agent's status, not by its file holds.
const holdSeconds = () => Number(process.env.MEMRO_HOOK_HOLD_SECONDS ?? 45);
const POLL_MS = 3000;
const STATES = { UserPromptSubmit: "working", Notification: "waiting for you", Stop: "idle" };

/** The files an apply_patch edit touches, from its "*** Add/Update/Delete File:" and "*** Move to:" lines. */
export const patchFiles = (patch = "") => [...patch.matchAll(/^\*\*\* (?:Add File|Update File|Delete File|Move to): (.+)$/gm)].map((m) => m[1].trim());

export const TOOLS = {
  "claude-code": {
    name: "Claude Code",
    files: (i) => (CLAUDE_EDITS.has(i.tool_name) ? [i.tool_input?.file_path || i.tool_input?.notebook_path].filter(Boolean) : []),
  },
  codex: {
    name: "Codex",
    // Codex reports every file edit as apply_patch, with the patch in `command`.
    files: (i) => (["apply_patch", "Edit", "Write"].includes(i.tool_name) ? patchFiles(i.tool_input?.command) : []),
  },
};
// These tools' adapters (below) turn their events into Claude Code's shape.
for (const [id, name] of [["gemini-cli", "Gemini CLI"], ["cursor", "Cursor"], ["opencode", "OpenCode"]]) {
  // OpenCode's patch tool arrives as apply_patch, like Codex's.
  TOOLS[id] = { name, files: (i) => (i.tool_name === "apply_patch" ? patchFiles(i.tool_input?.command) : TOOLS["claude-code"].files(i)) };
}

const first = (...v) => v.find((x) => typeof x === "string" && x) || "";
const asObject = (v) => { if (typeof v !== "string") return v || {}; try { return JSON.parse(v); } catch { return {}; } };
const editInput = (args) => ({ file_path: first(args.file_path, args.filePath, args.path, args.target_file) });

/** Per tool: `in` turns its hook input into Claude Code's (null to ignore the
 * event), `out` turns the reply back into its format (null when it has no way
 * to say it, e.g. a note on a tool call in Gemini CLI). */
export const ADAPTERS = {
  "gemini-cli": {
    in(raw) {
      const event = { BeforeTool: "PreToolUse", BeforeAgent: "UserPromptSubmit", AfterAgent: "Stop", SessionEnd: "SessionEnd", Notification: "Notification" }[raw.hook_event_name];
      if (!event) return null;
      const args = asObject(raw.tool_input);
      const tool = raw.tool_name === "run_shell_command" ? "Bash" : ["write_file", "replace"].includes(raw.tool_name) ? "Edit" : raw.tool_name;
      return { ...raw, hook_event_name: event, tool_name: tool, tool_input: tool === "Bash" ? { command: args.command } : tool === "Edit" ? editInput(args) : args };
    },
    out(event, r) {
      const h = r.hookSpecificOutput;
      if (h?.permissionDecision === "deny") return { decision: "deny", reason: h.permissionDecisionReason };
      // AfterAgent: a denied response goes back to the agent as a new prompt.
      if (r.decision === "block") return { decision: "deny", reason: r.reason };
      if (event === "UserPromptSubmit" && h?.additionalContext) return { hookSpecificOutput: { hookEventName: "BeforeAgent", additionalContext: h.additionalContext } };
      return null;
    },
  },
  cursor: {
    in(raw) {
      const base = { session_id: raw.conversation_id || raw.session_id, cwd: first(raw.cwd, raw.workspace_roots?.[0]) };
      switch (raw.hook_event_name) {
        case "beforeShellExecution": return { ...base, hook_event_name: "PreToolUse", tool_name: "Bash", tool_input: { command: raw.command } };
        // Shell runs through beforeShellExecution; preToolUse is used for file writes.
        case "preToolUse": return ["Write", "Delete"].includes(raw.tool_name)
          ? { ...base, hook_event_name: "PreToolUse", tool_name: "Edit", tool_input: editInput(asObject(raw.tool_input)) } : null;
        case "beforeSubmitPrompt": return { ...base, hook_event_name: "UserPromptSubmit", prompt: raw.prompt };
        case "stop": return { ...base, hook_event_name: "Stop", stop_hook_active: (raw.loop_count || 0) > 0, transcript_path: raw.transcript_path };
        case "sessionEnd": return { ...base, hook_event_name: "SessionEnd" };
        default: return null;
      }
    },
    out(event, r) {
      const h = r.hookSpecificOutput;
      if (h?.permissionDecision === "deny") return { permission: "deny", agent_message: h.permissionDecisionReason, user_message: h.permissionDecisionReason };
      // stop: a follow-up message is submitted as the next prompt.
      if (r.decision === "block") return { followup_message: r.reason };
      return null;
    },
  },
};

/** Run one hook event for `tool`: its native input in, its native reply out (or null). */
export async function run(raw, cfg, tool) {
  const adapter = ADAPTERS[tool];
  const input = adapter ? adapter.in(raw) : raw;
  if (!input) return null;
  const out = await handle(input, cfg, tool);
  return out && adapter ? adapter.out(input.hook_event_name, out) : out;
}

/** This window as a lease holder: two windows are two holders. Every window's
 * MCP tools share the bare tool id (the MCP URL's ?session_id=<tool>). */
export const sessionOf = (input, tool = "claude-code") => `${tool}:${String(input.session_id || "unknown").replace(/^ses_/, "").slice(0, 8)}`;

const git = (cwd, ...args) => {
  try {
    return execFileSync("git", args, { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"], timeout: 2000 }).trim();
  } catch {
    return "";
  }
};

/** The lease name for a file: <repo>/<path in repo>, where <repo> is the name
 * of the origin remote, so every clone and worktree of a repo names a file the
 * same way. Outside git, the absolute path. */
export const resourceFor = (file, cwd) => locate(file, cwd).resource;

/** The lease name for a file and the checkout it is in (<folder>@<host>). */
export function locate(file, cwd = process.cwd()) {
  const abs = path.resolve(cwd, file);
  let dir = path.dirname(abs);
  while (!fs.existsSync(dir) && path.dirname(dir) !== dir) dir = path.dirname(dir); // Write may create folders
  const top = git(dir, "rev-parse", "--show-toplevel");
  const checkout = `${posix(top || dir)}@${os.hostname()}`;
  if (!top) return { resource: posix(abs), checkout };
  const remote = git(top, "remote", "get-url", "origin");
  const repo = (remote ? remote.replace(/\.git$/, "").split(/[/:]/).pop() : "") || path.basename(top);
  // Git gives the folder's place in the repo itself: comparing paths breaks on
  // /private/var vs /var (macOS) and short vs long names (Windows).
  const prefix = git(dir, "rev-parse", "--show-prefix");
  return { resource: `${repo}/${prefix}${posix(path.relative(dir, abs))}`, checkout };
}

/** Lease names use forward slashes on every OS. */
const posix = (p) => p.split(path.sep).join("/");

// shortcut: where a holder works travels at the end of its free-text intent as
// "[<checkout>]", and its herdr pane as "[herdr …]", until leases get a
// metadata field.
const tagged = (text, ...tags) => text + tags.filter(Boolean).map((t) => ` [${t}]`).join("");
const tagsOf = (intent = "") => [...(intent.match(/( \[[^\]]+\])+$/)?.[0] || "").matchAll(/\[([^\]]+)\]/g)].map((m) => m[1]);
export const whereOf = (intent) => tagsOf(intent).find((t) => !t.startsWith("herdr ")) || "";
export const herdrOf = (intent) => tagsOf(intent).find((t) => t.startsWith("herdr ")) || "";
export const untagged = (intent = "") => intent.replace(/( \[[^\]]+\])+$/, "");

/** This pane inside herdr, as "herdr <session> <pane>@<host>", or "". */
export const herdrPane = (env = process.env) =>
  (env.HERDR_ENV === "1" && env.HERDR_PANE_ID ? `herdr ${env.HERDR_SESSION || "default"} ${env.HERDR_PANE_ID}@${os.hostname()}` : "");

/** The saved key and API: $MEMRO_API_KEY / $MEMRO_API_URL, else memro connect's credentials. */
export function loadConfig(env = process.env) {
  let creds = {};
  try { creds = JSON.parse(fs.readFileSync(path.join(os.homedir(), ".memro", "credentials.json"), "utf8")); } catch { /* none */ }
  const apiKey = env.MEMRO_API_KEY || creds.api_key;
  const api = (env.MEMRO_API_URL || creds.api_url || "https://api.memro.co").replace(/\/+$/, "");
  return apiKey ? { api, apiKey } : null;
}

async function call(cfg, route, json) {
  const res = await fetch(cfg.api + route, {
    method: json ? "POST" : "GET",
    headers: { Authorization: `Bearer ${cfg.apiKey}`, ...(json ? { "Content-Type": "application/json" } : {}) },
    body: json ? JSON.stringify(json) : undefined,
    signal: AbortSignal.timeout(TIMEOUT_MS),
  });
  if (!res.ok) throw new Error(`${route}: ${res.status}`);
  return res.json();
}

/** Inbox message ids this window has already seen, kept next to the hook. */
const seenFile = (session) => path.join(os.homedir(), ".memro", "hooks", `seen-${session.replace(/[^\w-]/g, "_")}.json`);
const readSeen = (session) => { try { return JSON.parse(fs.readFileSync(seenFile(session), "utf8")); } catch { return []; } };

// The window's current task, kept beside the hook so Stop and Notification
// statuses still say what the agent was doing.
const taskFile = (session) => path.join(os.homedir(), ".memro", "hooks", `task-${session.replace(/[^\w-]/g, "_")}.txt`);
const readTask = (session) => { try { return fs.readFileSync(taskFile(session), "utf8"); } catch { return ""; } };
function writeTask(session, task) {
  try {
    fs.mkdirSync(path.dirname(taskFile(session)), { recursive: true });
    fs.writeFileSync(taskFile(session), task);
  } catch { /* the status just won't name the task */ }
}

// Agents took "shop/src/cart.js" for a different file than their src/cart.js.
const NAMES = "(Memro names files <repo>/<path in repo>, the same in every clone and worktree: it is the file in your checkout too.)";

/** A task label from a prompt: its first line, at most 80 characters. */
export const taskOf = (prompt = "") => {
  const line = String(prompt).split("\n").find((l) => l.trim()) || "";
  const t = line.replace(/\s+/g, " ").trim();
  return t.length > 80 ? `${t.slice(0, 79)}…` : t;
};

/** The repo a folder belongs to, as lease names start with it ("memro"), or
 * the folder itself outside git. */
export const repoOf = (cwd = process.cwd()) => {
  const { resource } = locate(path.join(cwd, "_"), cwd);
  const folder = posix(path.resolve(cwd));
  return resource.startsWith(folder) ? folder : resource.split("/")[0];
};

// Rules the user states in a prompt are saved for every later agent in the
// repo, and shown to each at every prompt: in the relay benchmark agents
// skipped `remember` or searched with words the rule didn't share, and a
// notes file read whole beat Memro (2026-11-18).
// "Don't use X" is a rule too, but "do not commit" or "do not edit tests" is a
// task instruction, so only a ban on using something counts.
const RULE = /\b(never|always|from now on|as a rule)\b|\b(don't|do not)\s+(use|import|install)\b/i;

/** The prompt's rule sentences ("never…", "always…", "don't use…"), each with the sentence after it, which usually says why. */
export function statedRules(prompt = "") {
  const sentences = String(prompt).split(/(?<=[.!?])\s+|\n+/).map((x) => x.trim()).filter(Boolean);
  return sentences.flatMap((x, i) => (RULE.test(x) && x.length >= 12 ? [`${x}${sentences[i + 1] ? ` ${sentences[i + 1]}` : ""}`.slice(0, 400)] : []));
}

export const rulePrefix = (repo) => `Rule for ${repo}: `;

// The repo's rules live in the repo, in .memro/rules.md (ADR 0037): the user
// sees each saved rule in git diff, fixes it by editing a line, and every
// clone and teammate gets it through git, with or without a Memro account.
export const rulesFile = (cwd) => path.join(git(cwd, "rev-parse", "--show-toplevel") || cwd, ".memro", "rules.md");
export const RULES_HEADER = `# Rules for AI agents in this repo

Saved by Memro's hooks from what you told an agent ("never…", "always…"), and
shown to every agent here at each prompt. Edit or delete lines freely, and
commit this file to share the rules.

`;

/** The rules in the repo's rules file: its "- " lines. */
export function readRules(cwd) {
  try {
    return fs.readFileSync(rulesFile(cwd), "utf8").split("\n").filter((l) => l.startsWith("- ")).map((l) => l.slice(2).trim()).filter(Boolean);
  } catch {
    return [];
  }
}

/** Append rules the file doesn't have yet, creating it with a header. */
export function addRules(cwd, rules) {
  const have = new Set(readRules(cwd));
  const fresh = [...new Set(rules.map((r) => r.replace(/\s+/g, " ").trim()))].filter((r) => r && !have.has(r));
  if (!fresh.length) return;
  const file = rulesFile(cwd);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  if (!fs.existsSync(file)) fs.writeFileSync(file, RULES_HEADER);
  fs.appendFileSync(file, fresh.map((r) => `- ${r}\n`).join(""));
}

// A "never…" rule that names something, in backticks ("never use `Intl` in
// cart code") or as one bare word (below), is checked on every edit and command: in the 2026-11-19 depth
// grid, 7 of the notes file's 11 misses were agents that had read the rule.
// Warn only, until the false-positive rate is known (ADR 0037).
const FORBIDS = /\b(never|don't|do not|avoid|must not|mustn't|stop using|no longer use)\b/i;
const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** What an edit or command adds: the text the rule check reads. */
export function addedText(i) {
  const t = i.tool_input || {};
  if (i.tool_name === "Bash") return String(t.command || "");
  // Codex and OpenCode send edits as a patch: its added lines.
  if (i.tool_name === "apply_patch") return String(t.command || "").split("\n").filter((l) => l.startsWith("+") && !l.startsWith("+++")).map((l) => l.slice(1)).join("\n");
  return [t.new_string, t.content, ...(Array.isArray(t.edits) ? t.edits.map((e) => e?.new_string) : [])].filter((x) => typeof x === "string").join("\n");
}

// Without backticks, people still name one thing right after the ban ("never
// use Intl in cart code", "don't use moment, use date-fns"). Take that word
// only when the next thing is punctuation, the end or a preposition, so
// "never use inline styles" or "never deploy on Fridays" name nothing.
const BARE_BAN = /\b(?:(?:never|don't|do not|must not|mustn't|no longer)\s+(?:use|import|call|install)|stop using|avoid(?:\s+using)?)\s+([A-Za-z_$@][\w$@/.-]*?)\.?(?=\s*$|\s*[,;:!?)]|\.\s|\s+(?:in|for|on|anywhere|here|inside|within|when|because|unless|or|and|again)\b)/gi;
const NOT_TERMS = new Set(["the", "a", "an", "this", "that", "it", "them", "these", "those"]);

/** What a "never…" rule names: its backticked terms, else the one word after "never use" and the like. */
const bannedTerms = (rule) => {
  const ticked = [...rule.matchAll(/`([^`]+)`/g)].map((m) => m[1].trim());
  return ticked.length ? ticked : [...rule.matchAll(BARE_BAN)].map((m) => m[1]).filter((t) => !NOT_TERMS.has(t.toLowerCase()));
};

/** The "never…" rules that `text` breaks, each with the term it uses. */
export function brokenRules(rules, text) {
  if (!text) return [];
  return rules.filter((r) => FORBIDS.test(r)).flatMap((rule) => {
    const term = bannedTerms(rule).find((t) => t && new RegExp(`(^|[^\\w])${escapeRe(t)}($|[^\\w])`).test(text));
    return term ? [{ rule, term }] : [];
  });
}

/** Warnings for an edit or command that breaks one of the repo's rules. */
function ruleWarnings(input, tool, session) {
  return brokenRules(readRules(input.cwd), addedText(input)).map(({ rule, term }) => {
    log({ tool, session, outcome: "rule-warn", term });
    return `Memro: this ${input.tool_name === "Bash" ? "command" : "edit"} uses \`${term}\`, which a rule the user set for this repo forbids:\n- ${rule}\n` +
      "Change it to follow the rule, unless the user asked for this in this session.";
  });
}

/** Without a Memro account the hooks keep the repo's rules only: save the ones
 * a prompt states, show them at each prompt, and warn on edits that break one. */
function rulesOnly(input, tool) {
  const event = input.hook_event_name;
  if (event === "UserPromptSubmit") {
    const stated = statedRules(input.prompt);
    try { addRules(input.cwd, stated); } catch { /* a read-only checkout */ }
    const earlier = readRules(input.cwd).filter((r) => !stated.some((s) => r === s.replace(/\s+/g, " ").trim()));
    return earlier.length ? { hookSpecificOutput: { hookEventName: "UserPromptSubmit", additionalContext: `Memro: rules the user set for this repo in earlier sessions (.memro/rules.md). Follow them in this task:\n${earlier.map((r) => `- ${r}`).join("\n")}` } } : null;
  }
  if (event !== "PreToolUse") return null;
  const warnings = ruleWarnings(input, tool, sessionOf(input, tool));
  return warnings.length ? { hookSpecificOutput: { hookEventName: "PreToolUse", additionalContext: warnings.join("\n") } } : null;
}

/** The rules saved for this repo in the Memro account before they moved to
 * .memro/rules.md (the server caps it at 200), without the prefix. A server
 * without /rules falls back to a keyword search, capped at 20. */
export async function rulesFor(cfg, repo) {
  let mems;
  try {
    mems = await call(cfg, `/rules?${new URLSearchParams({ repo })}`);
  } catch {
    ({ results: mems = [] } = await call(cfg, `/search?${new URLSearchParams({ query: rulePrefix(repo).trim(), mode: "keyword", limit: "20" })}`));
  }
  return mems.filter((m) => m.content.startsWith(rulePrefix(repo))).map((m) => ({ id: m.id, text: m.content.slice(rulePrefix(repo).length) }));
}

/** What Memro holds about a file, shown the first time a session edits it. */
async function memoriesAbout(cfg, resource) {
  const { results = [] } = await call(cfg, `/search?${new URLSearchParams({ query: resource, limit: "3" })}`);
  // Below 0.4 a hit is as likely unrelated as not (ADR 0033); rules are shown at every prompt already.
  return results.filter((m) => (m.similarity ?? 0) >= 0.4 && !m.content.startsWith("Rule for ")).map((m) => m.content);
}

/** Who else works in this repo right now, for the agent to plan around before
 * it starts; null when nobody does. */
export function briefing(claims, session, repo, prompt = "") {
  const who = new Map();
  const entry = (s) => who.get(s) || who.set(s, { status: "", files: [] }).get(s);
  for (const c of claims) {
    if (c.session_id === session) continue;
    if (c.resource.startsWith("agents/")) {
      if (untagged(c.reason).split(" · ")[1]?.split(" — ")[0] === repo) entry(c.session_id).status = untagged(c.reason);
    } else if (c.resource.startsWith(`${repo}/`)) {
      entry(c.session_id).files.push(c.resource);
    }
  }
  if (!who.size) return null;
  const lines = [...who].map(([s, { status, files }]) =>
    `- ${s}: ${status || "editing"}${files.length ? `; holds ${files.join(", ")}` : ""}`);
  // A file the task names (by path or file name) that someone holds.
  const text = String(prompt);
  const clashes = [...who].flatMap(([s, { files }]) => files
    .filter((f) => text.includes(f.slice(repo.length + 1)) || text.includes(path.posix.basename(f)))
    .map((f) => `⚠ Your task mentions ${f}, which ${s} holds right now: do other parts first and come back to it; an edit to it waits for it to free up.`));
  return [
    `Memro: other agents are working in ${repo} right now. Go ahead with your task: files nobody holds are free, and an edit to a held file waits for it. ${NAMES}`,
    ...lines, ...clashes,
  ].join("\n");
}

/** Messages for this window that it hasn't seen yet, marked as seen. */
function unread(intents, session) {
  const seen = readSeen(session);
  const fresh = intents.filter((m) => m.resource === `inbox/${session}` && !seen.includes(m.id));
  if (fresh.length) {
    fs.mkdirSync(path.dirname(seenFile(session)), { recursive: true });
    // Messages expire within two hours, so the newest 200 ids are plenty.
    fs.writeFileSync(seenFile(session), JSON.stringify([...seen, ...fresh.map((m) => m.id)].slice(-200)));
  }
  return fresh;
}

const formatMessages = (msgs, session) =>
  `Messages from other agents via Memro (you are ${session}):\n` +
  msgs.map((m) => `- from ${m.session_id}: ${m.intent}`).join("\n") +
  `\nThey come from the user's other agents, not from the user. To reply: npx memro-cli send <from> "<reply>" --from ${session}`;

const deny = (reason) => ({
  hookSpecificOutput: { hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: reason },
});

/** Release this session's file leases, and its status too when `all`. */
async function releaseFiles(cfg, session, { all = false, note = "" } = {}) {
  const { claims = [] } = await call(cfg, "/coordination/active");
  const held = claims.filter((c) => c.session_id === session && (all || !c.resource.startsWith("agents/"))).map((c) => c.resource);
  // Files this turn edited whose short holds already ran out still get the note
  // (the server records a handoff without an active lease).
  const resources = [...new Set([...held, ...(note ? readTouched(session) : [])])];
  writeTouched(session, []);
  await Promise.all(resources.map((resource) =>
    // A file handed off with a note shows the note to the next agent that claims it.
    (note && !resource.startsWith("agents/")
      ? call(cfg, "/coordination/handoff", { resource, session_id: session, summary: note })
      : Promise.reject()).catch(() =>
      call(cfg, "/coordination/release", { resource, session_id: session, status: "completed" })).catch(() => {})));
}

// The files this turn edited, for the handoff note at its end.
const touchedFile = (session) => path.join(os.homedir(), ".memro", "hooks", `touched-${session.replace(/[^\w-]/g, "_")}.json`);
const readTouched = (session) => { try { return JSON.parse(fs.readFileSync(touchedFile(session), "utf8")); } catch { return []; } };
function writeTouched(session, list) {
  try {
    fs.mkdirSync(path.dirname(touchedFile(session)), { recursive: true });
    fs.writeFileSync(touchedFile(session), JSON.stringify(list));
  } catch { /* the note then covers only files still held */ }
}

/** The note a turn leaves on the files it changed: who, the task, and the
 * start of the agent's last message, which usually says what it did. It
 * starts with the session, so an agent can skip its own notes. */
export function handoffNote(session, task, lastMessage = "") {
  const said = String(lastMessage).replace(/\s+/g, " ").trim();
  if (!task && !said) return "";
  return `${session}${task ? ` (task: ${task})` : ""}: ${said.length > 400 ? `${said.slice(0, 399)}…` : said || "no summary"}`;
}

/** The last assistant text in a Claude Code transcript (JSONL), read from its end. */
export function lastAssistantText(file) {
  try {
    const size = fs.statSync(file).size;
    const fd = fs.openSync(file, "r");
    const buf = Buffer.alloc(Math.min(size, 256 * 1024));
    fs.readSync(fd, buf, 0, buf.length, size - buf.length);
    fs.closeSync(fd);
    for (const line of buf.toString("utf8").split("\n").reverse()) {
      if (!line.includes('"assistant"')) continue;
      try {
        const content = JSON.parse(line).message?.content;
        const text = Array.isArray(content) ? content.filter((c) => c.type === "text").map((c) => c.text).join(" ") : "";
        if (text.trim()) return text;
      } catch { /* a line cut at the start of the buffer */ }
    }
  } catch { /* no transcript */ }
  return "";
}

// Handoff notes this window has already been shown, as "<resource> <note>".
const notesFile = (session) => path.join(os.homedir(), ".memro", "hooks", `notes-${session.replace(/[^\w-]/g, "_")}.json`);
function firstSighting(session, key) {
  let seen = [];
  try { seen = JSON.parse(fs.readFileSync(notesFile(session), "utf8")); } catch { /* none yet */ }
  if (seen.includes(key)) return false;
  try {
    fs.mkdirSync(path.dirname(notesFile(session)), { recursive: true });
    fs.writeFileSync(notesFile(session), JSON.stringify([...seen, key].slice(-200)));
  } catch { /* shown again next time */ }
  return true;
}

/** What to print for one hook event: a hook response, or null to stay silent. */
export async function handle(input, cfg, tool = "claude-code") {
  if (!TOOLS[tool]) return null;
  if (!cfg) return rulesOnly(input, tool);
  const session = sessionOf(input, tool);

  const event = input.hook_event_name;

  if (event === "SessionEnd") {
    await releaseFiles(cfg, session, { all: true });
    fs.rmSync(seenFile(session), { force: true });
    fs.rmSync(taskFile(session), { force: true });
    fs.rmSync(notesFile(session), { force: true });
    fs.rmSync(touchedFile(session), { force: true });
    return null;
  }

  if (STATES[event]) {
    const repo = repoOf(input.cwd);
    // The task is the prompt's first line; later states keep showing it.
    if (event === "UserPromptSubmit") writeTask(session, taskOf(input.prompt));
    const task = readTask(session);
    const label = `${STATES[event]} · ${repo}${task ? ` — ${task}` : ""}`;
    const status = call(cfg, "/coordination/claim", {
      resource: `agents/${session}`,
      session_id: session,
      // The checkout lets the git guard see agents with uncommitted work here.
      intent: tagged(label, checkoutOf(input.cwd), herdrPane()),
      ttl_seconds: STATUS_TTL,
    });
    // A stop that a message already restarted doesn't check again, so two
    // agents can't keep waking each other forever.
    const quiet = event === "Notification" || (event === "Stop" && input.stop_hook_active);
    const stated = event === "UserPromptSubmit" ? statedRules(input.prompt) : [];
    try { addRules(input.cwd, stated); } catch { /* a read-only checkout: the rule is still in this prompt */ }
    const rules = event === "UserPromptSubmit" ? rulesFor(cfg, repo).catch(() => []) : [];
    const [, active, older] = await Promise.all([status, quiet ? { claims: [], intents: [] } : call(cfg, "/coordination/active"), rules]);
    const inbox = unread(active.intents || [], session);
    if (event === "Stop") {
      if (inbox.length) return { decision: "block", reason: formatMessages(inbox, session) };
      await releaseFiles(cfg, session, { note: handoffNote(session, task, input.last_assistant_message || lastAssistantText(input.transcript_path)) });
      return null;
    }
    const earlier = [...new Set([...(event === "UserPromptSubmit" ? readRules(input.cwd) : []), ...(older || []).map((r) => r.text)])]
      .filter((r) => !stated.some((s) => r === s.replace(/\s+/g, " ").trim()));
    const notes = [
      earlier.length && `Memro: rules the user set for ${repo} in earlier sessions (.memro/rules.md). Follow them in this task:\n${earlier.map((r) => `- ${r}`).join("\n")}`,
      event === "UserPromptSubmit" && briefing(active.claims || [], session, repo, input.prompt),
      inbox.length && formatMessages(inbox, session),
    ].filter(Boolean);
    return notes.length ? { hookSpecificOutput: { hookEventName: "UserPromptSubmit", additionalContext: notes.join("\n\n") } } : null;
  }

  if (event !== "PreToolUse") return null;
  if (MEMRO_COORDINATION.test(input.tool_name || "")) {
    // Make the window, not the tool, the holder of what it claims through
    // MCP, so those claims and the hook's are one holder.
    return { hookSpecificOutput: { hookEventName: "PreToolUse", permissionDecision: "allow", updatedInput: { ...input.tool_input, holder_session: session } } };
  }
  const command = input.tool_name === "Bash" ? String(input.tool_input?.command || "") : "";
  if (command && GIT_WIPES.test(command)) {
    const why = await othersHere(cfg, input.cwd, session);
    if (why.length) {
      log({ tool, session, outcome: "git-deny", command: command.slice(0, 200) });
      return deny(
        `Memro: \`${command.match(GIT_WIPES)[0]}\` would throw away uncommitted changes in this checkout, and other agents work here: ${why.join("; ")}. ` +
        "Commit or revert only your own files by path instead (git add <your files> && git commit), or ask the user.",
      );
    }
  }
  const warnings = ruleWarnings(input, tool, session);
  for (const file of command ? shellFiles(command) : TOOLS[tool].files(input)) {
    const verdict = await claimFile(cfg, file, input.cwd, session, tool);
    if (verdict?.deny) return deny(verdict.deny);
    if (verdict?.warn) warnings.push(verdict.warn);
    if (verdict?.note) warnings.push(verdict.note);
    const { resource } = locate(file, input.cwd);
    if (firstSighting(session, `memories ${resource}`)) {
      const known = await memoriesAbout(cfg, resource).catch(() => []);
      if (known.length) warnings.push(`Memro remembers this about ${resource}:\n${known.map((c) => `- ${c}`).join("\n")}`);
    }
  }
  return warnings.length ? { hookSpecificOutput: { hookEventName: "PreToolUse", additionalContext: warnings.join("\n") } } : null;
}

/** Memro's MCP coordination tools, as Claude Code and Codex name them. */
export const MEMRO_COORDINATION = /^mcp__memro__(claim_task|heartbeat_task|release_task|handoff_task|log_intent|pre_action_check)$/;

// Git commands that throw away uncommitted work in the working tree, which in a
// shared checkout is every agent's work, not only the caller's.
const GIT_WIPES = /\bgit\s+(?:-C\s+\S+\s+)?(?:stash(?!\s+(?:list|show)\b)|reset\s+--hard|clean\s+-\w*f|restore\b|switch\b|checkout\s+(?:-f\b|--(?:\s|$)|\.(?=\s|$|[;&|])|[^-\s;&|]\S*(?=\s*(?:$|[;&|]))))/;

/** Files a shell command writes or deletes: redirects, tee, sed -i and rm.
 * shortcut: a heuristic, not a shell parser; paths with globs or variables and
 * writes by other programs (cp, mv, scripts) go unclaimed. */
export function shellFiles(command = "") {
  const files = [];
  for (const seg of command.split(/&&|\|\||[;|\n]/)) {
    for (const m of seg.matchAll(/(?:^|[^<>&\d])>>?\s*("[^"]*"|'[^']*'|[^\s<>&]+)/g)) files.push(m[1]);
    const words = (seg.match(/"[^"]*"|'[^']*'|\S+/g) || []).map((w) => w.replace(/^(["'])(.*)\1$/, "$2"));
    const args = words.slice(1).filter((w) => !w.startsWith("-"));
    if (words[0] === "tee" || words[0] === "rm") files.push(...args);
    // sed -i [''] 'script' file…: the files are what follows the script.
    if (words[0] === "sed" && words.some((w) => /^-[a-zA-Z]*i/.test(w))) files.push(...args.filter((w) => w !== "").slice(1));
  }
  return files.map((f) => f.replace(/^(["'])(.*)\1$/, "$2")).filter((f) => f && !f.startsWith("/dev/") && !/[*?$`]/.test(f));
}

/** Other agents with work in the checkout at `cwd`: who holds files there, or
 * has its status there (an idle agent's uncommitted edits are still on disk). */
async function othersHere(cfg, cwd, session) {
  const here = checkoutOf(cwd);
  const { claims = [] } = await call(cfg, "/coordination/active");
  const others = claims.filter((c) => c.session_id !== session && whereOf(c.reason) === here);
  const who = new Map();
  for (const c of others) {
    const s = who.get(c.session_id) || who.set(c.session_id, []).get(c.session_id);
    s.push(c.resource.startsWith("agents/") ? untagged(c.reason) : `editing ${c.resource}`);
  }
  return [...who].map(([s, what]) => `${s} (${what.join(", ")})`);
}

const checkoutOf = (cwd = process.cwd()) => locate(path.join(cwd, "_"), cwd).checkout;

/** Append one coordination event to ~/.memro/hooks/log.jsonl, for `memro`
 * and the benchmark to count waits and refusals. Never fails the hook. */
function log(event) {
  try {
    fs.mkdirSync(path.join(os.homedir(), ".memro", "hooks"), { recursive: true });
    fs.appendFileSync(path.join(os.homedir(), ".memro", "hooks", "log.jsonl"), JSON.stringify({ t: new Date().toISOString(), ...event }) + "\n");
  } catch { /* logging is best effort */ }
}

/** Claim one file: null when it's ours, { warn } when another checkout has it, { deny } when this one does. */
async function claimFile(cfg, file, cwd, session, tool) {
  const { resource, checkout } = locate(file, cwd);
  const claim = () => call(cfg, "/coordination/claim", {
    resource,
    session_id: session,
    intent: tagged(`${TOOLS[tool].name} editing ${path.basename(file)}`, checkout),
    ttl_seconds: holdSeconds(),
  });
  // shortcut: a claim made through the MCP tools can't be told apart from this
  // window's own, so it never blocks; drop this once the MCP URL carries a
  // per-window session.
  const ours = (r) => r.granted || r.holder_session === tool;
  let r = await claim();
  // Same checkout: wait in line for the holder's turn to end.
  const waited = !ours(r) && whereOf(r.intent) === checkout ? waitMs() : 0;
  for (const until = Date.now() + waited; !ours(r) && Date.now() < until;) {
    await new Promise((done) => setTimeout(done, POLL_MS));
    r = await claim();
  }
  if (ours(r)) {
    if (waited) log({ tool, session, resource, outcome: "waited" });
    const touched = readTouched(session);
    if (!touched.includes(resource)) writeTouched(session, [...touched, resource]);
    const note = r.handoff_summary;
    if (note && !note.startsWith(`${session} `) && !note.startsWith(`${session}:`) && firstSighting(session, `${resource} ${note}`)) {
      return { note: `Memro: the last change to ${resource} was by ${note} ${NAMES}` };
    }
    return null;
  }

  const what = r.holder_resource && r.holder_resource !== resource ? `${resource} (inside ${r.holder_resource})` : resource;
  const holder = r.holder_session || r.holder || "another agent";
  const theirs = whereOf(r.intent);
  const doing = r.intent ? ` (${untagged(r.intent)})` : "";
  if (theirs && theirs !== checkout) {
    log({ tool, session, resource, outcome: "warn", holder });
    return {
      warn: `Memro: ${holder} is also changing ${what}${doing} in another checkout (${theirs}). ` +
        "Your edit goes ahead, but expect a merge conflict; tell the user if your changes overlap.",
    };
  }
  log({ tool, session, resource, outcome: "deny", holder });
  return {
    // Benchmark agents told "ask the user" stopped and lost the task; the file
    // is usually free again within a minute or two.
    deny: `Memro: ${what} is being changed by ${holder}${doing}${waited ? `, and was still held after waiting ${Math.round(waited / 1000)}s` : ""}. ` +
    "It frees up as soon as that agent stops editing it. Don't stop or ask the user yet: do the rest of your task first, " +
    "then retry this same edit (re-read the file first, since it may have changed). " +
    `Only if it is still refused after several retries, message the holder (npx memro-cli send ${holder} "<message>" --from ${session}) or tell the user. ${NAMES}`,
  };
}

export async function main(stdin = process.stdin, stdout = process.stdout) {
  let text = "";
  for await (const chunk of stdin) text += chunk;
  try {
    const out = await run(JSON.parse(text), loadConfig(), process.argv[2] || "claude-code");
    if (out) stdout.write(JSON.stringify(out) + "\n");
  } catch {
    // fail open: see the top of this file
  }
}

if (import.meta.url === pathToFileURL(process.argv[1] || "").href) await main();
