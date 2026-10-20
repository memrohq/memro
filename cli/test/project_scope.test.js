import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

// Isolate HOME so tests never touch real configs.
const TEST_DIR = fs.mkdtempSync(path.join(os.tmpdir(), "memro-proj-test-"));
const HOME = path.join(TEST_DIR, "home");
const PROJECT_DIR = path.join(TEST_DIR, "my project space"); // Test paths with spaces!

process.env.HOME = HOME;
process.env.USERPROFILE = HOME;
process.env.APPDATA = path.join(HOME, "AppData");
process.env.XDG_CONFIG_HOME = path.join(HOME, ".config");
process.env.CODEX_HOME = path.join(HOME, ".codex");
process.env.MEMRO_NO_CLAUDE_CLI = "1";

const { parseArgs, isGitTracked, resolveProjectRoot, ensureGitIgnored, isHomeOrRoot } = await import("../src/cli.js");
const { CLIENTS, findClient } = await import("../src/clients.js");
const { PROJECT_BLOCK, BLOCK, START, END, upsertBlock, removeBlock, installInstructions, uninstallInstructions } = await import("../src/instructions.js");

const conn = { mcpUrl: "https://mcp.memro.co/mcp", apiKey: "memro_sk_proj_test" };

beforeEach(() => {
  fs.rmSync(HOME, { recursive: true, force: true });
  fs.rmSync(PROJECT_DIR, { recursive: true, force: true });
  fs.mkdirSync(HOME, { recursive: true });
  fs.mkdirSync(PROJECT_DIR, { recursive: true });
});

test("parseArgs handles user and project scopes and rejects conflicts", () => {
  // Default is user scope
  assert.equal(parseArgs(["connect"]).scope, "user");
  assert.equal(parseArgs(["connect", "--scope", "user"]).scope, "user");

  // Project scope via --project or --scope project
  assert.equal(parseArgs(["connect", "--project"]).scope, "project");
  assert.equal(parseArgs(["connect", "--scope", "project"]).scope, "project");
  assert.equal(parseArgs(["connect", "--project", "--scope", "project"]).scope, "project");

  // Other options work alongside --project
  const withOptions = parseArgs(["connect", "--project", "--client", "claude,cursor", "--no-instructions", "--yes"]);
  assert.equal(withOptions.scope, "project");
  assert.deepEqual(withOptions.clients, ["claude", "cursor"]);
  assert.equal(withOptions.instructions, false);
  assert.equal(withOptions.yes, true);

  // Conflicting scope arguments
  assert.throws(() => parseArgs(["connect", "--project", "--scope", "user"]), /Conflicting scope arguments/);
  assert.throws(() => parseArgs(["connect", "--scope", "user", "--scope", "project"]), /Conflicting scope arguments/);
  assert.throws(() => parseArgs(["connect", "--scope", "workspace"]), /Invalid scope "workspace"/);

  // Custom key argument
  assert.equal(parseArgs(["connect", "--key", "memro_sk_custom"]).key, "memro_sk_custom");
});

test("findClient resolves aliases such as 'claude' and 'code'", () => {
  assert.equal(findClient("claude")?.id, "claude-code");
  assert.equal(findClient("claudecode")?.id, "claude-code");
  assert.equal(findClient("claude-code")?.id, "claude-code");
  assert.equal(findClient("code")?.id, "vscode");
  assert.equal(findClient("agy")?.id, "antigravity");
  assert.equal(findClient("gemini")?.id, "gemini-cli");
  assert.equal(findClient("cursor")?.id, "cursor");
});

test("Claude Code in project mode writes ./.mcp.json and uses PROJECT_BLOCK", () => {
  const c = findClient("claude-code");
  assert.equal(c.supportsScope("project"), true);

  const projFile = c.file("project", PROJECT_DIR);
  const projRules = c.rules("project", PROJECT_DIR);
  // .mcp.json is Claude Code's project-scoped server file (`claude mcp add --scope project`).
  assert.equal(projFile, path.join(PROJECT_DIR, ".mcp.json"));
  assert.equal(projRules, path.join(PROJECT_DIR, "CLAUDE.md"));

  // Pre-populate with existing server
  fs.writeFileSync(projFile, JSON.stringify({ mcpServers: { existing: { type: "http", url: "https://example.com" } } }));

  c.add(conn, { scope: "project", cwd: PROJECT_DIR });

  // Verify project config
  const cfg = JSON.parse(fs.readFileSync(projFile, "utf8"));
  assert.ok(cfg.mcpServers.existing, "existing server preserved");
  assert.equal(cfg.mcpServers.memro.url, conn.mcpUrl);
  assert.equal(cfg.mcpServers.memro.headers.Authorization, `Bearer ${conn.apiKey}`);

  // Global user config must NOT have been created
  assert.equal(fs.existsSync(path.join(HOME, ".claude.json")), false);

  // Add project instructions
  installInstructions(projRules, { scope: "project" });
  const rulesContent = fs.readFileSync(projRules, "utf8");
  assert.ok(rulesContent.includes("## Memro — long-term memory"));
  assert.ok(rulesContent.includes("Follow this repository's privacy and memory-sharing boundaries."));

  // Idempotency: repeated add and install don't duplicate
  c.add(conn, { scope: "project", cwd: PROJECT_DIR });
  installInstructions(projRules, { scope: "project" });
  assert.equal(rulesContent.split(START).length - 1, 1);

  // Remove
  assert.equal(c.remove({ scope: "project", cwd: PROJECT_DIR }), true);
  const afterRemove = JSON.parse(fs.readFileSync(projFile, "utf8"));
  assert.equal(afterRemove.mcpServers.memro, undefined);
  assert.ok(afterRemove.mcpServers.existing, "existing server preserved after remove");

  assert.equal(uninstallInstructions(projRules), true);
  assert.equal(fs.existsSync(projRules), false);
});

test("Cursor in project mode writes to ./.cursor/mcp.json and leaves global config untouched", () => {
  const c = findClient("cursor");
  assert.equal(c.supportsScope("project"), true);

  const projFile = c.file("project", PROJECT_DIR);
  assert.equal(projFile, path.join(PROJECT_DIR, ".cursor", "mcp.json"));

  // Add in project mode
  c.add(conn, { scope: "project", cwd: PROJECT_DIR });
  assert.ok(fs.existsSync(projFile));
  assert.equal(fs.existsSync(path.join(HOME, ".cursor", "mcp.json")), false, "global config untouched");

  const cfg = JSON.parse(fs.readFileSync(projFile, "utf8"));
  assert.equal(cfg.mcpServers.memro.url, conn.mcpUrl);

  // Remove in project mode
  assert.equal(c.remove({ scope: "project", cwd: PROJECT_DIR }), true);
  const cfgAfter = JSON.parse(fs.readFileSync(projFile, "utf8"));
  assert.equal(cfgAfter.mcpServers?.memro, undefined);
});

test("VS Code in project mode writes to ./.vscode/mcp.json", () => {
  const c = findClient("vscode");
  assert.equal(c.supportsScope("project"), true);
  const projFile = c.file("project", PROJECT_DIR);
  assert.equal(projFile, path.join(PROJECT_DIR, ".vscode", "mcp.json"));

  c.add(conn, { scope: "project", cwd: PROJECT_DIR });
  const cfg = JSON.parse(fs.readFileSync(projFile, "utf8"));
  assert.equal(cfg.servers.memro.url, conn.mcpUrl);
  assert.equal(c.remove({ scope: "project", cwd: PROJECT_DIR }), true);
});

test("Codex in project mode writes to ./.codex/config.toml and ./AGENTS.md", () => {
  const c = findClient("codex");
  assert.equal(c.supportsScope("project"), true);
  const projFile = c.file("project", PROJECT_DIR);
  const projRules = c.rules("project", PROJECT_DIR);
  assert.equal(projFile, path.join(PROJECT_DIR, ".codex", "config.toml"));
  assert.equal(projRules, path.join(PROJECT_DIR, "AGENTS.md"));

  c.add(conn, { scope: "project", cwd: PROJECT_DIR });
  const text = fs.readFileSync(projFile, "utf8");
  assert.match(text, /\[mcp_servers\.memro\]/);

  installInstructions(projRules, { scope: "project" });
  assert.ok(fs.existsSync(projRules));

  assert.equal(c.remove({ scope: "project", cwd: PROJECT_DIR }), true);
  assert.equal(uninstallInstructions(projRules), true);
});

test("Claude Desktop does NOT support project scope", () => {
  const c = findClient("claude-desktop");
  assert.equal(c.supportsScope("user"), true);
  assert.equal(c.supportsScope("project"), false);
});

test("isGitTracked detects .git folder in directory", () => {
  assert.equal(isGitTracked(PROJECT_DIR), false);
  fs.mkdirSync(path.join(PROJECT_DIR, ".git"));
  assert.equal(isGitTracked(PROJECT_DIR), true);
});

test("Global mode continues to work identically as default without arguments", () => {
  const c = findClient("claude-code");
  // Calling without scope arguments defaults to user scope
  assert.equal(c.file(), path.join(HOME, ".claude.json"));
  assert.equal(c.rules(), path.join(HOME, ".claude", "CLAUDE.md"));

  c.add(conn);
  assert.ok(fs.existsSync(path.join(HOME, ".claude.json")));
  assert.equal(fs.existsSync(path.join(PROJECT_DIR, ".claude.json")), false);
  assert.equal(c.remove(), true);
});

test("resolveProjectRoot finds project boundary when called from nested subdirectories", () => {
  const nested = path.join(PROJECT_DIR, "src", "nested", "deep");
  fs.mkdirSync(nested, { recursive: true });

  // Before marker, resolves to start dir
  assert.equal(resolveProjectRoot(nested), nested);

  // With package.json at PROJECT_DIR
  fs.writeFileSync(path.join(PROJECT_DIR, "package.json"), "{}");
  assert.equal(resolveProjectRoot(nested), PROJECT_DIR);
});

test("resolveProjectRoot never treats user home directory as a project root", () => {
  // Even if HOME has .cursor or .claude global dirs, starting from a subfolder without a project marker stops before home
  fs.mkdirSync(path.join(HOME, ".claude"), { recursive: true });
  fs.mkdirSync(path.join(HOME, ".cursor"), { recursive: true });
  const sub = path.join(HOME, "Documents", "work", "unmarked");
  fs.mkdirSync(sub, { recursive: true });

  assert.equal(resolveProjectRoot(sub), sub);
});

test("ensureGitIgnored adds config path to .gitignore and prevents duplicates", () => {
  const cursorConfig = path.join(PROJECT_DIR, ".cursor", "mcp.json");
  const gitignore = path.join(PROJECT_DIR, ".gitignore");

  const res1 = ensureGitIgnored(cursorConfig, PROJECT_DIR);
  assert.equal(res1.added, true);
  const text1 = fs.readFileSync(gitignore, "utf8");
  assert.ok(text1.includes("/.cursor/mcp.json"));

  // Second run: already present, not re-added
  const res2 = ensureGitIgnored(cursorConfig, PROJECT_DIR);
  assert.equal(res2.added, false);
  assert.equal(res2.ignored, true);
  const text2 = fs.readFileSync(gitignore, "utf8");
  assert.equal(text2, text1, "no duplicate lines added");
});

test("instruction files handle existing content, missing markers, and duplicate markers", () => {
  const initial = "# Project Overview\n\nExisting guidelines here.\n";
  const withOneBlock = upsertBlock(initial, PROJECT_BLOCK);
  assert.ok(withOneBlock.startsWith(initial));
  assert.ok(withOneBlock.includes("## Memro — long-term memory"));

  // Duplicate blocks accidentally injected
  const corrupted = `${initial}\n${PROJECT_BLOCK}\n\n# More Rules\n\n${PROJECT_BLOCK}\n`;
  assert.equal(corrupted.split(START).length - 1, 2, "two start markers present");

  // upsertBlock cleans all duplicate blocks and leaves exactly one
  const cleaned = upsertBlock(corrupted, PROJECT_BLOCK);
  assert.equal(cleaned.split(START).length - 1, 1, "exactly one start marker after upsert");
  assert.ok(cleaned.includes("# Project Overview"));
  assert.ok(cleaned.includes("# More Rules"));

  // removeBlock cleans all markers
  const stripped = removeBlock(corrupted);
  assert.equal(stripped.includes(START), false);
  assert.ok(stripped.includes("# Project Overview"));
  assert.ok(stripped.includes("# More Rules"));
});

test("custom --key is used for project without overwriting global credentials", () => {
  const globalCredsFile = path.join(HOME, ".memro", "credentials.json");
  fs.mkdirSync(path.dirname(globalCredsFile), { recursive: true });
  fs.writeFileSync(globalCredsFile, JSON.stringify({ api_key: "memro_sk_global_user", agent_id: "agent_global" }));

  // Add a project using a distinct key
  const customConn = { mcpUrl: "https://mcp.memro.co/mcp", apiKey: "memro_sk_isolated_project" };
  const c = findClient("cursor");
  c.add(customConn, { scope: "project", cwd: PROJECT_DIR });

  // Project config has isolated key
  const projCfg = JSON.parse(fs.readFileSync(c.file("project", PROJECT_DIR), "utf8"));
  assert.equal(projCfg.mcpServers.memro.headers.Authorization, "Bearer memro_sk_isolated_project");

  // Global credentials file is completely unchanged
  const globalCreds = JSON.parse(fs.readFileSync(globalCredsFile, "utf8"));
  assert.equal(globalCreds.api_key, "memro_sk_global_user");
  assert.equal(globalCreds.agent_id, "agent_global");
});

// Regression (memro-cli 0.1.3): a home folder that is itself a git repo (or
// holds .claude/.cursor) became the "project", and project files landed in ~.
test("a git repository at the home folder is never the project root", () => {
  execFileSync("git", ["init", "-q", HOME]);
  const sub = path.join(HOME, "Documents", "work", "app");
  fs.mkdirSync(sub, { recursive: true });
  assert.equal(resolveProjectRoot(sub), sub);
  assert.equal(isHomeOrRoot(HOME), true);
  assert.equal(isHomeOrRoot(path.parse(HOME).root), true);
  assert.equal(isHomeOrRoot(sub), false);
  fs.rmSync(path.join(HOME, ".git"), { recursive: true, force: true });
});
