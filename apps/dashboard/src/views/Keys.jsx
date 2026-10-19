import { useCallback, useEffect, useState } from "react";
import { Panel, Icon, Modal, Field, Empty, timeAgo, PageHeader, Skeleton } from "../components/ui";
import { api } from "../lib/api";

export default function Keys({ apiBase, auth, onCopy, notify }) {
  const [keys, setKeys] = useState(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [created, setCreated] = useState(null);
  const [revoking, setRevoking] = useState(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setKeys(await api(apiBase, "/api-keys", { auth }));
    } catch (err) {
      notify("Couldn't load API keys", err.message, true);
      setKeys([]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken]);

  useEffect(() => { load(); }, [load]);

  const create = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await api(apiBase, "/api-keys", { method: "POST", auth, json: { name: name.trim() } });
      setCreated(res);
      setName("");
      setCreateOpen(false);
      load();
    } catch (err) {
      notify("Couldn't create key", err.message, true);
    } finally {
      setBusy(false);
    }
  };

  const revoke = async () => {
    setBusy(true);
    try {
      await api(apiBase, `/api-keys/${revoking.id}`, { method: "DELETE", auth });
      notify("Key revoked", revoking.name);
      setRevoking(null);
      load();
    } catch (err) {
      notify("Couldn't revoke key", err.message, true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-6 max-w-3xl">
      <PageHeader title="API keys" description="Keys let your tools, SDKs and MCP clients use this account's memory. npx memro connect creates one for you.">
        <button className="btn-primary" onClick={() => setCreateOpen(true)}><Icon name="add" className="!text-[18px]" /> New key</button>
      </PageHeader>

      {created && (
        <div className="card p-5 border-ok/40">
          <p className="text-[14px] font-medium">Copy your new key now</p>
          <p className="text-[13px] text-muted mt-1">You won't be able to see it again. Store it like a password.</p>
          <div className="flex items-center gap-2 mt-3">
            <code className="font-mono text-[12px] bg-panel border border-line rounded-md px-2 py-1.5 truncate flex-1">{created.key}</code>
            <button className="btn-secondary h-8 px-2.5" onClick={() => onCopy(created.key, "API key")} aria-label="Copy key">
              <Icon name="content_copy" className="!text-[16px]" />
            </button>
          </div>
          <button className="btn-ghost mt-3 -ml-2" onClick={() => setCreated(null)}>I've saved it</button>
        </div>
      )}

      <Panel title={`Active keys${keys ? ` · ${keys.length}` : ""}`} bodyClass="p-0">
        {keys === null ? (
          <div className="p-5 space-y-3"><Skeleton className="h-4 w-1/2" /><Skeleton className="h-4 w-1/3" /></div>
        ) : keys.length === 0 ? (
          <Empty icon="key" title="No API keys yet">Run npx memro connect, or create a key to connect a tool.</Empty>
        ) : (
          <ul>
            {keys.map((k) => (
              <li key={k.id} className="flex items-center gap-4 px-5 py-3.5 border-b border-line last:border-0">
                <div className="min-w-0 flex-1">
                  <p className="text-[13px] font-medium truncate">{k.name}</p>
                  <p className="text-[12px] text-subtle font-mono mt-0.5">{k.prefix}… · created {timeAgo(k.created_at)} · {k.last_used_at ? `used ${timeAgo(k.last_used_at)}` : "never used"}</p>
                </div>
                <button className="btn-ghost text-danger" onClick={() => setRevoking(k)}>Revoke</button>
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <Modal open={createOpen} title="New API key" onClose={() => setCreateOpen(false)}>
        <form onSubmit={create} className="space-y-4">
          <Field label="Name" hint="So you can recognise it later, e.g. “Work laptop” or “CI”.">
            <input autoFocus className="input" maxLength={60} value={name} onChange={(e) => setName(e.target.value)} placeholder="Work laptop" />
          </Field>
          <div className="flex justify-end gap-2">
            <button type="button" className="btn-ghost" onClick={() => setCreateOpen(false)}>Cancel</button>
            <button className="btn-primary" disabled={busy}>{busy ? "Creating…" : "Create key"}</button>
          </div>
        </form>
      </Modal>

      <Modal open={!!revoking} title="Revoke key?" onClose={() => setRevoking(null)}>
        <p className="text-[13px] text-muted">Tools using <span className="text-ink font-medium">{revoking?.name}</span> will stop working immediately. This can't be undone.</p>
        <div className="flex justify-end gap-2 mt-5">
          <button className="btn-ghost" onClick={() => setRevoking(null)}>Cancel</button>
          <button className="btn-danger" onClick={revoke} disabled={busy}>{busy ? "Revoking…" : "Revoke"}</button>
        </div>
      </Modal>
    </div>
  );
}
