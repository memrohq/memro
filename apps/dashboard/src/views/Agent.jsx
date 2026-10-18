import { Panel, Icon } from "../components/ui";

const Row = ({ label, children }) => (
  <div className="grid grid-cols-1 sm:grid-cols-[180px_1fr] gap-1 sm:gap-6 px-5 py-4 border-b border-line last:border-0">
    <span className="text-[13px] text-muted">{label}</span>
    <div className="min-w-0">{children}</div>
  </div>
);

export default function Agent({ agentId, email, onCopy, onExport, onSignOut, onOpenKeys }) {
  return (
    <div className="space-y-4 max-w-3xl">
      <div>
        <h1 className="text-[24px] font-semibold tracking-tight">Account</h1>
        <p className="text-[13px] text-muted mt-1">Your account and the agent that holds its memory.</p>
      </div>

      <Panel title="Account" bodyClass="p-0">
        <Row label="Signed in as"><span className="text-[13px]">{email || "—"}</span></Row>
        <Row label="Agent ID">
          <div className="flex items-center gap-2">
            <code className="font-mono text-[12px] bg-panel border border-line rounded-md px-2 py-1.5 truncate flex-1">{agentId || "—"}</code>
            <button className="btn-secondary h-8 px-2.5" onClick={() => onCopy(agentId, "Agent ID")} aria-label="Copy agent ID" disabled={!agentId}>
              <Icon name="content_copy" className="!text-[16px]" />
            </button>
          </div>
          <p className="text-[12px] text-subtle mt-2">Tools authenticate with API keys, not with this ID.</p>
        </Row>
        <Row label="Access">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-[13px] text-muted">Create and revoke the keys your tools use.</p>
            <button className="btn-secondary" onClick={onOpenKeys}><Icon name="vpn_key" className="!text-[17px]" /> API keys</button>
          </div>
        </Row>
      </Panel>

      <Panel title="Data" bodyClass="p-0">
        <Row label="Export">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-[13px] text-muted">Download every memory as JSON, straight from the server.</p>
            <button className="btn-secondary" onClick={onExport} disabled={!agentId}><Icon name="download" className="!text-[17px]" /> Export</button>
          </div>
        </Row>
        <Row label="This browser">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-[13px] text-muted">Sign out of this browser. Your memories stay on the server.</p>
            <button className="btn-danger" onClick={onSignOut}><Icon name="logout" className="!text-[17px]" /> Sign out</button>
          </div>
        </Row>
      </Panel>
    </div>
  );
}
