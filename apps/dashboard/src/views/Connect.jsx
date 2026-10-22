import { useEffect, useState } from "react";
import { Panel, Icon, PageHeader, formatDate } from "../components/ui";
import { api, connectFlags, mcpUrl } from "../lib/api";

const KEY = "<your API key>";

/** Manual setup for each tool, for people who don't want to run the CLI. */
const TOOLS = (url) => [
  { id: "claude-code", name: "Claude Code", where: "Terminal",
    code: `claude mcp add --transport http --scope user memro ${url} \\\n  --header "Authorization: Bearer ${KEY}"` },
  { id: "codex", name: "Codex", where: "~/.codex/config.toml",
    code: `[mcp_servers.memro]\nurl = "${url}"\nhttp_headers = { Authorization = "Bearer ${KEY}" }` },
  { id: "cursor", name: "Cursor", where: "~/.cursor/mcp.json",
    code: JSON.stringify({ mcpServers: { memro: { url, headers: { Authorization: `Bearer ${KEY}` } } } }, null, 2) },
  { id: "antigravity", name: "Antigravity", where: "~/.gemini/antigravity/mcp_config.json",
    code: JSON.stringify({ mcpServers: { memro: { serverUrl: url, headers: { Authorization: `Bearer ${KEY}` } } } }, null, 2) },
  { id: "opencode", name: "OpenCode", where: "~/.config/opencode/opencode.json",
    code: JSON.stringify({ mcp: { memro: { type: "remote", url, enabled: true, headers: { Authorization: `Bearer ${KEY}` } } } }, null, 2) },
  { id: "gemini-cli", name: "Gemini CLI", where: "~/.gemini/settings.json",
    code: JSON.stringify({ mcpServers: { memro: { httpUrl: url, headers: { Authorization: `Bearer ${KEY}` } } } }, null, 2) },
  { id: "vscode", name: "VS Code", where: "User mcp.json (MCP: Open User Configuration)",
    code: JSON.stringify({ servers: { memro: { type: "http", url, headers: { Authorization: `Bearer ${KEY}` } } } }, null, 2) },
  { id: "windsurf", name: "Windsurf", where: "~/.codeium/windsurf/mcp_config.json",
    code: JSON.stringify({ mcpServers: { memro: { serverUrl: url, headers: { Authorization: `Bearer ${KEY}` } } } }, null, 2) },
  { id: "other", name: "Other MCP clients", where: "Streamable HTTP",
    code: `URL:     ${url}\nHeader:  Authorization: Bearer ${KEY}` },
];

function Approve({ apiBase, auth, code, onDone, notify }) {
  const [info, setInfo] = useState(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [approved, setApproved] = useState(false);
  const [confirmCode, setConfirmCode] = useState("");

  useEffect(() => {
    api(apiBase, `/auth/device?user_code=${encodeURIComponent(code)}`, { auth })
      .then(setInfo)
      .catch((err) => setError(err.status === 404 ? "This code wasn't found. Run the command again to get a new one." : err.message));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, code]);

  const approve = async () => {
    setBusy(true);
    try {
      await api(apiBase, "/auth/device/approve", { method: "POST", auth, json: { user_code: code } });
      setApproved(true);
      notify("Connected", info?.client_name || code);
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  };

  const status = approved ? "approved" : info?.status;
  const isConfirmed = confirmCode.trim().toUpperCase() === (code || "").trim().toUpperCase();

  return (
    <Panel title="Connect a device" bodyClass="p-5">
      {error ? (
        <p className="text-[13px] text-danger">{error}</p>
      ) : !info ? (
        <p className="text-[13px] text-muted">Checking code…</p>
      ) : status === "approved" ? (
        <div className="flex items-start gap-3">
          <Icon name="check_circle" className="text-ok mt-0.5" />
          <div>
            <p className="text-[14px] font-medium">You're connected</p>
            <p className="text-[13px] text-muted mt-1">Return to your terminal. It's adding Memro to your tools now.</p>
            <button className="btn-ghost mt-3 -ml-2" onClick={onDone}>Done</button>
          </div>
        </div>
      ) : status === "expired" ? (
        <p className="text-[13px] text-danger">This code has expired. Run the command again to get a new one.</p>
      ) : (
        <div className="space-y-4">
          <div className="p-3.5 rounded-xl border border-warn/30 bg-warn/10 text-[13px] flex items-start gap-2.5">
            <Icon name="warning" className="text-warn text-[18px] shrink-0 mt-0.5" />
            <div className="space-y-0.5">
              <p className="font-semibold text-ink">Security notice</p>
              <p className="text-muted leading-relaxed">
                Only approve a code you just started yourself. Approving connects that tool to YOUR account.
              </p>
            </div>
          </div>

          <div>
            <p className="text-[15px] font-medium text-ink">{info.client_name} wants to use your Memro memory</p>
            <p className="text-[13px] text-muted mt-1">A new API key will be created for this tool; you can revoke it anytime in API keys.</p>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 p-3 bg-panel border border-line rounded-xl text-[12.5px]">
            <div>
              <span className="text-subtle block text-[11px] uppercase tracking-wider font-medium">Created</span>
              <span className="text-ink font-mono mt-0.5 block">{info.created_at ? formatDate(info.created_at) : "Just now"}</span>
            </div>
            <div>
              <span className="text-subtle block text-[11px] uppercase tracking-wider font-medium">Expires</span>
              <span className="text-ink font-mono mt-0.5 block">{info.expires_at ? formatDate(info.expires_at) : "—"}</span>
            </div>
          </div>

          <div>
            <span className="text-subtle block text-[11px] uppercase tracking-wider font-medium mb-1.5">Code to approve</span>
            <code className="block w-fit font-mono text-[22px] font-semibold tracking-[0.2em] bg-panel border border-line rounded-lg px-4 py-2 text-ink select-all">
              {code}
            </code>
          </div>

          <div className="space-y-1.5 max-w-sm">
            <label htmlFor="confirm-user-code" className="text-subtle block text-[11px] uppercase tracking-wider font-medium">
              Confirm code
            </label>
            <input
              id="confirm-user-code"
              type="text"
              placeholder={`Type ${code} to confirm`}
              value={confirmCode}
              onChange={(e) => setConfirmCode(e.target.value)}
              className="input font-mono tracking-widest text-[14px] uppercase"
              autoComplete="off"
              spellCheck={false}
            />
            <p className="text-[11.5px] text-subtle">
              Type the code above to confirm you started this connection in your terminal.
            </p>
          </div>

          <div className="flex gap-2 pt-1">
            <button
              className="btn-primary"
              onClick={approve}
              disabled={busy || !isConfirmed}
            >
              {busy ? "Connecting…" : "Approve"}
            </button>
            <button className="btn-ghost" onClick={onDone}>Cancel</button>
          </div>
        </div>
      )}
    </Panel>
  );
}

export default function Connect({ apiBase, auth, code, onDone, onCopy, notify }) {
  const url = mcpUrl();
  const tools = TOOLS(url);
  const [tool, setTool] = useState(tools[0].id);
  const current = tools.find((t) => t.id === tool);
  const command = `npx memro-cli connect${connectFlags()}`;

  return (
    <div className="space-y-6 max-w-3xl">
      <PageHeader title="Connect your tools" description="Give Claude Code, Codex, Cursor, Antigravity, OpenCode and other agents a shared, persistent memory." />

      {code && <Approve apiBase={apiBase} auth={auth} code={code} onDone={onDone} notify={notify} />}

      <Panel title="One command" bodyClass="p-5">
        <p className="text-[13px] text-muted mb-3">Run this in a terminal. It signs you in through this page, then finds your tools and adds Memro to each one.</p>
        <div className="flex items-center gap-2">
          <code className="font-mono text-[13px] bg-panel border border-line rounded-md px-3 py-2 flex-1">{command}</code>
          <button className="btn-secondary h-9 px-2.5" onClick={() => onCopy(command, "Command")} aria-label="Copy command">
            <Icon name="content_copy" className="!text-[16px]" />
          </button>
        </div>
      </Panel>

      <Panel title="Manual setup" bodyClass="p-0">
        <div className="flex flex-wrap gap-1 p-3 border-b border-line">
          {tools.map((t) => (
            <button
              key={t.id}
              onClick={() => setTool(t.id)}
              className={`h-8 px-2.5 rounded-lg text-[12px] border ${tool === t.id ? "bg-panel border-line text-ink font-medium" : "border-transparent text-muted hover:text-ink"}`}
            >
              {t.name}
            </button>
          ))}
        </div>
        <div className="p-5 space-y-3">
          <div className="flex items-center justify-between gap-3">
            <p className="text-[12px] text-muted font-mono truncate">{current.where}</p>
            <button className="btn-secondary h-8 px-2.5" onClick={() => onCopy(current.code, current.name)} aria-label="Copy snippet">
              <Icon name="content_copy" className="!text-[16px]" />
            </button>
          </div>
          <pre className="font-mono text-[12px] leading-relaxed bg-panel border border-line rounded-lg p-3 overflow-x-auto scroll-thin">{current.code}</pre>
          <p className="text-[12px] text-subtle">Replace <code className="font-mono">{KEY}</code> with a key from API keys.</p>
        </div>
      </Panel>
    </div>
  );
}
