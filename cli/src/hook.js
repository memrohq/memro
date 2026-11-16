// Claude Code hook that claims every file before the agent edits it, so
// coordination no longer depends on the model remembering to call claim_task
// (memro ADR 0019). `memro connect` copies this file to ~/.memro/hooks/ and
// runs it with plain `node`, so it must import nothing but Node built-ins.
//
// PreToolUse (Edit, Write, MultiEdit, NotebookEdit): claim the file for this
// Claude Code session. Re-claiming extends the lease, so editing is the
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

const EDIT_TOOLS = new Set(["Edit", "Write", "MultiEdit", "NotebookEdit"]);
const TIMEOUT_MS = 3000;
// The MCP tools of every Claude Code window share this holder (the MCP URL's
// ?session_id=claude-code), so the hook can't tell whose claim it is.
const SHARED_MCP_SESSION = "claude-code";
const STATUS_TTL = 7200; // the server's maximum; SessionEnd clears it sooner
const STATES = { UserPromptSubmit: "working", Notification: "waiting for you", Stop: "idle" };

/** This Claude Code window as a lease holder: two windows are two holders. */
export const sessionOf = (input) => `claude-code:${String(input.session_id || "unknown").slice(0, 8)}`;

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
  const checkout = `${top || dir}@${os.hostname()}`;
  if (!top) return { resource: abs, checkout };
  const remote = git(top, "remote", "get-url", "origin");
  const repo = (remote ? remote.replace(/\.git$/, "").split(/[/:]/).pop() : "") || path.basename(top);
  // Compare real paths: git reports /private/var/… where the hook may say /var/….
  const real = (p) => { try { return fs.realpathSync(p); } catch { return p; } };
  const rel = path.relative(real(top), path.join(real(dir), path.relative(dir, abs)));
  return { resource: [repo, ...rel.split(path.sep)].join("/"), checkout };
}

// shortcut: where a holder works travels at the end of its free-text intent as
// "[…]" until leases get a metadata field.
const tagged = (text, where) => `${text} [${where}]`;
export const whereOf = (intent = "") => intent.match(/\[([^\]]+)\]$/)?.[1] || "";

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

/** Messages for this window that it hasn't seen yet, marked as seen. */
async function unread(cfg, session) {
  const { intents = [] } = await call(cfg, `/coordination/active?resource=${encodeURIComponent(`inbox/${session}`)}`);
  const seen = readSeen(session);
  const fresh = intents.filter((m) => !seen.includes(m.id));
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
async function releaseFiles(cfg, session, { all = false } = {}) {
  const { claims = [] } = await call(cfg, "/coordination/active");
  await Promise.all(claims.filter((c) => c.session_id === session && (all || !c.resource.startsWith("agents/"))).map((c) =>
    call(cfg, "/coordination/release", { resource: c.resource, session_id: session, status: "completed" }).catch(() => {})));
}

/** What to print for one hook event: a Claude Code hook response, or null to stay silent. */
export async function handle(input, cfg) {
  if (!cfg) return null;
  const session = sessionOf(input);

  const event = input.hook_event_name;

  if (event === "SessionEnd") {
    await releaseFiles(cfg, session, { all: true });
    fs.rmSync(seenFile(session), { force: true });
    return null;
  }

  if (STATES[event]) {
    const label = `${STATES[event]} · ${path.basename(input.cwd || "")}`;
    const status = call(cfg, "/coordination/claim", {
      resource: `agents/${session}`,
      session_id: session,
      intent: herdrPane() ? tagged(label, herdrPane()) : label,
      ttl_seconds: STATUS_TTL,
    });
    // A stop that a message already restarted doesn't check again, so two
    // agents can't keep waking each other forever.
    const quiet = event === "Notification" || (event === "Stop" && input.stop_hook_active);
    const [, inbox] = await Promise.all([status, quiet ? [] : unread(cfg, session)]);
    if (!inbox.length) {
      if (event === "Stop") await releaseFiles(cfg, session);
      return null;
    }
    if (event === "Stop") return { decision: "block", reason: formatMessages(inbox, session) };
    return { hookSpecificOutput: { hookEventName: "UserPromptSubmit", additionalContext: formatMessages(inbox, session) } };
  }

  if (event !== "PreToolUse" || !EDIT_TOOLS.has(input.tool_name)) return null;
  const file = input.tool_input?.file_path || input.tool_input?.notebook_path;
  if (!file) return null;
  const { resource, checkout } = locate(file, input.cwd);
  const r = await call(cfg, "/coordination/claim", {
    resource,
    session_id: session,
    intent: tagged(`Claude Code editing ${path.basename(file)}`, checkout),
  });
  // shortcut: a claim made through the MCP tools can't be told apart from this
  // window's own, so it never blocks; drop this once the MCP URL carries a
  // per-window session.
  if (r.granted || r.holder_session === SHARED_MCP_SESSION) return null;

  const what = r.holder_resource && r.holder_resource !== resource ? `${resource} (inside ${r.holder_resource})` : resource;
  const holder = r.holder_session || r.holder || "another agent";
  const theirs = whereOf(r.intent);
  const doing = r.intent ? ` (${r.intent.replace(/ \[[^\]]+\]$/, "")})` : "";
  if (theirs && theirs !== checkout) {
    return {
      hookSpecificOutput: {
        hookEventName: "PreToolUse",
        additionalContext: `Memro: ${holder} is also changing ${what}${doing} in another checkout (${theirs}). ` +
          "Your edit goes ahead, but expect a merge conflict; tell the user if your changes overlap.",
      },
    };
  }
  return deny(
    `Memro: ${what} is being changed by ${holder}${doing}. It is released when that agent finishes its turn. ` +
    `Meanwhile work on something else, message it (npx memro-cli send ${holder} "<message>" --from ${session}), ` +
    "or tell the user who holds it and ask how to proceed.",
  );
}

export async function main(stdin = process.stdin, stdout = process.stdout) {
  let text = "";
  for await (const chunk of stdin) text += chunk;
  try {
    const out = await handle(JSON.parse(text), loadConfig());
    if (out) stdout.write(JSON.stringify(out) + "\n");
  } catch {
    // fail open: see the top of this file
  }
}

if (import.meta.url === pathToFileURL(process.argv[1] || "").href) await main();
