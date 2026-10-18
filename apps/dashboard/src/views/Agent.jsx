import { useState } from "react";
import { Panel, Icon, Field } from "../components/ui";

const Row = ({ label, children }) => (
  <div className="grid grid-cols-1 sm:grid-cols-[180px_1fr] gap-1 sm:gap-6 px-5 py-4 border-b border-line last:border-0">
    <span className="text-[13px] text-muted">{label}</span>
    <div className="min-w-0">{children}</div>
  </div>
);

const Secret = ({ value, onCopy }) => {
  const [shown, setShown] = useState(false);
  if (!value) return <span className="text-[13px] text-subtle">Not stored in this browser</span>;
  return (
    <div className="flex items-center gap-2">
      <code className="font-mono text-[12px] bg-panel border border-line rounded-md px-2 py-1.5 truncate flex-1">
        {shown ? value : "•".repeat(32)}
      </code>
      <button className="btn-secondary h-8 px-2.5" onClick={() => setShown((s) => !s)} aria-label={shown ? "Hide" : "Reveal"}>
        <Icon name={shown ? "visibility_off" : "visibility"} className="!text-[16px]" />
      </button>
      <button className="btn-secondary h-8 px-2.5" onClick={onCopy} aria-label="Copy">
        <Icon name="content_copy" className="!text-[16px]" />
      </button>
    </div>
  );
};

export default function Agent({ agentId, privateKey, email, onCopy, onGenerate, onRestore, onExport, onSignOut, busy }) {
  const initialized = agentId && agentId !== "NOT_INITIALIZED";
  const [restore, setRestore] = useState({ id: "", key: "" });

  return (
    <div className="space-y-4 max-w-3xl">
      <div>
        <h1 className="text-[24px] font-semibold tracking-tight">Agent</h1>
        <p className="text-[13px] text-muted mt-1">The ed25519 identity your SDKs and MCP clients use to read and write memory.</p>
      </div>

      {initialized ? (
        <Panel title="Identity" bodyClass="p-0">
          {email && <Row label="Signed in as"><span className="text-[13px]">{email}</span></Row>}
          <Row label="Agent ID">
            <div className="flex items-center gap-2">
              <code className="font-mono text-[12px] bg-panel border border-line rounded-md px-2 py-1.5 truncate flex-1">{agentId}</code>
              <button className="btn-secondary h-8 px-2.5" onClick={() => onCopy(agentId, "Agent ID")} aria-label="Copy agent ID">
                <Icon name="content_copy" className="!text-[16px]" />
              </button>
            </div>
          </Row>
          <Row label="Private key">
            <Secret value={privateKey} onCopy={() => onCopy(privateKey, "Private key")} />
            <p className="text-[12px] text-subtle mt-2">Keep this secret. Anyone with it can read and write this agent's memory.</p>
          </Row>
        </Panel>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          <Panel title="New agent" bodyClass="p-5">
            <p className="text-[13px] text-muted mb-4">Generate a fresh identity. The private key stays in this browser.</p>
            <button className="btn-primary" onClick={onGenerate} disabled={busy}>Generate identity</button>
          </Panel>
          <Panel title="Existing agent" bodyClass="p-5">
            <form
              className="space-y-3"
              onSubmit={(e) => { e.preventDefault(); onRestore(restore.id.trim(), restore.key.trim()); }}
            >
              <Field label="Agent ID"><input className="input font-mono" value={restore.id} onChange={(e) => setRestore({ ...restore, id: e.target.value })} required /></Field>
              <Field label="Private key" hint="64 or 128 hex characters">
                <input type="password" className="input font-mono" value={restore.key} onChange={(e) => setRestore({ ...restore, key: e.target.value })} required />
              </Field>
              <button className="btn-secondary">Restore</button>
            </form>
          </Panel>
        </div>
      )}

      <Panel title="Data" bodyClass="p-0">
        <Row label="Export">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-[13px] text-muted">Download every memory for this agent as JSON, straight from the server.</p>
            <button className="btn-secondary" onClick={onExport} disabled={!initialized}><Icon name="download" className="!text-[17px]" /> Export</button>
          </div>
        </Row>
        <Row label="This browser">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-[13px] text-muted">Sign out and remove the session and keys stored in this browser. Your memories stay on the server.</p>
            <button className="btn-danger" onClick={onSignOut}><Icon name="logout" className="!text-[17px]" /> Sign out</button>
          </div>
        </Row>
      </Panel>
    </div>
  );
}
