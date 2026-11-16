// Memro coordination for OpenCode: runs the same hook as Claude Code and Codex
// (~/.memro/hooks/memro-hook.mjs, from memro-cli's src/hook.js) on OpenCode's
// plugin events. `memro connect` copies it to OpenCode's plugins folder.
//
// OpenCode can block a tool (throw in tool.execute.before) but has no way to
// add a note for the model, so briefings, handoff notes and messages are not
// shown here; claims, refusals, the git guard, status and handoffs work.

import { spawn } from "node:child_process";
import os from "node:os";
import path from "node:path";

const HOOK = path.join(os.homedir(), ".memro", "hooks", "memro-hook.mjs");

/** Run the hook with a Claude Code-shaped event; its reply, or null on any failure. */
function hook(event) {
  return new Promise((done) => {
    try {
      const child = spawn("node", [HOOK, "opencode"], { stdio: ["pipe", "pipe", "ignore"] });
      let out = "";
      const timer = setTimeout(() => child.kill(), 90_000);
      child.stdout.on("data", (d) => { out += d; });
      child.on("error", () => done(null));
      child.on("close", () => {
        clearTimeout(timer);
        try { done(JSON.parse(out || "null")); } catch { done(null); }
      });
      child.stdin.end(JSON.stringify(event));
    } catch {
      done(null);
    }
  });
}

/** OpenCode's tool call as the hook sees it, or null for tools it ignores. */
function toolEvent(tool, args = {}) {
  if (tool === "bash") return { tool_name: "Bash", tool_input: { command: args.command } };
  if (tool === "patch" || tool === "apply_patch") return { tool_name: "apply_patch", tool_input: { command: args.patchText || args.patch || "" } };
  if (["edit", "write", "multiedit"].includes(tool)) return { tool_name: "Edit", tool_input: { file_path: args.filePath || args.file_path || args.path } };
  return null;
}

// OpenCode loads every export of this file as a plugin, so export only this.
export const MemroPlugin = async ({ directory }) => ({
  "tool.execute.before": async (input, output) => {
    const call = toolEvent(input.tool, output.args);
    if (!call) return;
    const r = await hook({ hook_event_name: "PreToolUse", session_id: input.sessionID, cwd: directory, ...call });
    // A thrown error refuses the tool call and is shown to the model.
    if (r?.hookSpecificOutput?.permissionDecision === "deny") throw new Error(r.hookSpecificOutput.permissionDecisionReason);
  },
  "chat.message": async (input, output) => {
    const prompt = (output.parts || []).filter((p) => p.type === "text").map((p) => p.text).join("\n");
    await hook({ hook_event_name: "UserPromptSubmit", session_id: input.sessionID, cwd: directory, prompt });
  },
  event: async ({ event }) => {
    const sid = event.properties?.sessionID || event.properties?.info?.id;
    if (!sid) return;
    // stop_hook_active: OpenCode can't be woken by a message, so don't read the inbox.
    if (event.type === "session.idle") await hook({ hook_event_name: "Stop", session_id: sid, cwd: directory, stop_hook_active: true });
    if (event.type === "session.deleted") await hook({ hook_event_name: "SessionEnd", session_id: sid, cwd: directory });
  },
});

// For the tests (a property, not an export).
MemroPlugin.toolEvent = toolEvent;
