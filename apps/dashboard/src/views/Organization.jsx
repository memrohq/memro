import { useCallback, useEffect, useState } from "react";
import { Panel, Icon, Modal, Field, Empty, timeAgo, PageHeader, Skeleton } from "../components/ui";
import { api } from "../lib/api";

// Organisations (ADR 0026): the people who manage a customer's account, its
// tenant keys and usage per end user. Members never see end users' memories.

const ROLES = ["owner", "admin", "billing", "viewer"];
const ROLE_HINT = {
  owner: "Everything, including managing owners",
  admin: "Members, invitations, tenant keys and usage",
  billing: "Usage and billing",
  viewer: "Read-only usage",
};
const thisMonth = () => new Date().toISOString().slice(0, 7);
const num = (n) => (n || 0).toLocaleString();
const usd = (n) => `$${(n || 0).toFixed(n && n < 0.01 ? 6 : 2)}`;

export default function Organization({ apiBase, auth, onCopy, notify, inviteToken, onInviteDone }) {
  const [orgs, setOrgs] = useState(null);
  const [orgId, setOrgId] = useState(null);

  const loadOrgs = useCallback(async () => {
    try {
      const list = await api(apiBase, "/orgs", { auth });
      setOrgs(list);
      setOrgId((cur) => (list.some((o) => o.tenant_id === cur) ? cur : list[0]?.tenant_id || null));
    } catch (err) {
      notify("Couldn't load organisations", err.message, true);
      setOrgs([]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken]);

  // An invitation link (/invite?token=…) is accepted once the person is signed in.
  useEffect(() => {
    if (!inviteToken) { loadOrgs(); return; }
    (async () => {
      try {
        const m = await api(apiBase, "/invites/accept", { method: "POST", auth, json: { token: inviteToken } });
        notify("Invitation accepted", `You joined ${m.name} as ${m.role}`);
        setOrgId(m.tenant_id);
      } catch (err) {
        notify("Couldn't accept invitation", err.message, true);
      }
      onInviteDone();
      loadOrgs();
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [inviteToken]);

  const org = orgs?.find((o) => o.tenant_id === orgId);
  const manage = org && (org.role === "owner" || org.role === "admin");

  return (
    <div className="space-y-6 max-w-4xl">
      <PageHeader title="Organization" description="People who manage your company's Memro account, its server keys, and what each of your users costs. Members see usage, never your users' memories.">
        {orgs?.length > 1 && (
          <select className="input h-9 w-auto" value={orgId || ""} onChange={(e) => setOrgId(e.target.value)} aria-label="Organisation">
            {orgs.map((o) => <option key={o.tenant_id} value={o.tenant_id}>{o.name}</option>)}
          </select>
        )}
      </PageHeader>

      {orgs === null ? (
        <div className="card p-5 space-y-3"><Skeleton className="h-4 w-1/2" /><Skeleton className="h-4 w-1/3" /></div>
      ) : !org ? (
        <Empty icon="corporate_fare" title="You're not in an organisation">Ask an owner of your company's account to invite you. The invitation email has a link that brings you here.</Empty>
      ) : (
        <>
          <p className="text-[13px] text-muted">{org.name} · you're <span className="text-ink font-medium">{org.role}</span></p>
          <Usage key={`u-${org.tenant_id}`} apiBase={apiBase} auth={auth} org={org} notify={notify} />
          <Members key={`m-${org.tenant_id}`} apiBase={apiBase} auth={auth} org={org} manage={manage} notify={notify} onCopy={onCopy} onLeft={loadOrgs} />
          {manage && <TenantKeys key={`k-${org.tenant_id}`} apiBase={apiBase} auth={auth} org={org} notify={notify} onCopy={onCopy} />}
        </>
      )}
    </div>
  );
}

function Usage({ apiBase, auth, org, notify }) {
  const [month, setMonth] = useState(thisMonth);
  const [data, setData] = useState(null);

  useEffect(() => {
    let alive = true;
    setData(null);
    api(apiBase, `/tenant/${org.tenant_id}/usage/members?month=${month}`, { auth })
      .then((d) => alive && setData(d))
      .catch((err) => { notify("Couldn't load usage", err.message, true); alive && setData({ members: [], totals: {} }); });
    return () => { alive = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken, org.tenant_id, month]);

  const exportCsv = () => {
    const head = ["user", "agent_id", "queries", "memories_stored", "stored_tokens", "query_tokens", "vision_input_tokens", "vision_output_tokens", "cost_usd"];
    const rows = data.members.map((m) => [m.user ?? "", m.agent_id, ...head.slice(2, -1).map((k) => m.metrics[k] || 0), m.cost_usd]);
    const csv = [head, ...rows].map((r) => r.map((v) => `"${String(v).replaceAll('"', '""')}"`).join(",")).join("\n");
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([csv], { type: "text/csv" }));
    a.download = `${org.name}-usage-${month}.csv`;
    a.click();
    URL.revokeObjectURL(a.href);
  };

  return (
    <Panel
      title="Usage per user"
      description={data ? `${num(data.active_members)} active users · ${num(data.totals?.queries)} searches · estimated model cost ${usd(data.cost_usd)}` : "Loading…"}
      action={
        <div className="flex items-center gap-2">
          <input type="month" className="input h-8 w-auto" value={month} max={thisMonth()} onChange={(e) => e.target.value && setMonth(e.target.value)} aria-label="Month" />
          <button className="btn-secondary h-8 px-2.5" onClick={exportCsv} disabled={!data?.members?.length}><Icon name="download" className="!text-[16px]" /> CSV</button>
        </div>
      }
      bodyClass="p-0"
    >
      {data === null ? (
        <div className="p-5"><Skeleton className="h-4 w-1/2" /></div>
      ) : data.members.length === 0 ? (
        <Empty icon="monitoring" title="No usage this month">Usage appears here once your backend calls Memro with a tenant key and X-Memro-User.</Empty>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-[13px]">
            <thead className="text-subtle text-left">
              <tr className="border-b border-line">
                <th className="font-medium px-5 py-2.5">User</th>
                <th className="font-medium px-3 py-2.5 text-right">Searches</th>
                <th className="font-medium px-3 py-2.5 text-right">Memories saved</th>
                <th className="font-medium px-3 py-2.5 text-right">Tokens</th>
                <th className="font-medium px-5 py-2.5 text-right">Est. cost</th>
              </tr>
            </thead>
            <tbody>
              {data.members.map((m) => {
                const t = m.metrics;
                const tokens = (t.stored_tokens || 0) + (t.query_tokens || 0) + (t.vision_input_tokens || 0) + (t.vision_output_tokens || 0);
                return (
                  <tr key={m.agent_id} className="border-b border-line last:border-0">
                    <td className="px-5 py-2.5 font-mono text-[12px] truncate max-w-[220px]" title={m.agent_id}>{m.user ?? <span className="text-subtle">deleted user</span>}</td>
                    <td className="px-3 py-2.5 text-right tabular-nums">{num(t.queries)}</td>
                    <td className="px-3 py-2.5 text-right tabular-nums">{num(t.memories_stored)}</td>
                    <td className="px-3 py-2.5 text-right tabular-nums">{num(tokens)}</td>
                    <td className="px-5 py-2.5 text-right tabular-nums">{usd(m.cost_usd)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Panel>
  );
}

function Members({ apiBase, auth, org, manage, notify, onCopy, onLeft }) {
  const [members, setMembers] = useState(null);
  const [invites, setInvites] = useState([]);
  const [me, setMe] = useState(null);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [draft, setDraft] = useState({ email: "", role: "viewer" });
  const [link, setLink] = useState(null);
  const [busy, setBusy] = useState(false);
  const base = `/tenant/${org.tenant_id}`;

  const load = useCallback(async () => {
    try {
      const [m, who] = await Promise.all([api(apiBase, `${base}/members`, { auth }), api(apiBase, "/auth/whoami", { auth })]);
      setMembers(m); setMe(who.user_id);
      if (manage) setInvites(await api(apiBase, `${base}/invites`, { auth }));
    } catch (err) {
      notify("Couldn't load members", err.message, true);
      setMembers([]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken, base, manage]);

  useEffect(() => { load(); }, [load]);

  const run = async (what, fn) => {
    setBusy(true);
    try { await fn(); load(); } catch (err) { notify(what, err.message, true); } finally { setBusy(false); }
  };
  const invite = (e) => {
    e.preventDefault();
    run("Couldn't invite", async () => {
      const res = await api(apiBase, `${base}/invites`, { method: "POST", auth, json: draft });
      notify(res.emailed ? "Invitation sent" : "Invitation created", draft.email);
      setLink(res.emailed ? null : res.link);
      setDraft({ email: "", role: "viewer" }); setInviteOpen(false);
    });
  };
  const setRole = (m, role) => run("Couldn't change role", () => api(apiBase, `${base}/members/${m.user_id}`, { method: "PATCH", auth, json: { role } }));
  const remove = (m) => run(m.user_id === me ? "Couldn't leave" : "Couldn't remove member", async () => {
    await api(apiBase, `${base}/members/${m.user_id}`, { method: "DELETE", auth });
    if (m.user_id === me) onLeft();
  });
  const mayEdit = (m) => manage && (org.role === "owner" || m.role !== "owner");

  return (
    <Panel
      title={`Members${members ? ` · ${members.length}` : ""}`}
      action={manage && <button className="btn-primary h-8" onClick={() => setInviteOpen(true)}><Icon name="person_add" className="!text-[18px]" /> Invite</button>}
      bodyClass="p-0"
    >
      {link && (
        <div className="p-5 border-b border-line">
          <p className="text-[13px] font-medium">Email isn't set up here, so share this link with them</p>
          <p className="text-[12px] text-muted mt-1">Only the invited address can use it, once, within 7 days.</p>
          <div className="flex items-center gap-2 mt-2">
            <code className="font-mono text-[12px] bg-panel border border-line rounded-md px-2 py-1.5 truncate flex-1">{link}</code>
            <button className="btn-secondary h-8 px-2.5" onClick={() => onCopy(link, "Invitation link")} aria-label="Copy link"><Icon name="content_copy" className="!text-[16px]" /></button>
          </div>
        </div>
      )}
      {members === null ? (
        <div className="p-5"><Skeleton className="h-4 w-1/2" /></div>
      ) : (
        <ul>
          {members.map((m) => (
            <li key={m.user_id} className="flex items-center gap-3 px-5 py-3 border-b border-line last:border-0">
              <div className="min-w-0 flex-1">
                <p className="text-[13px] font-medium truncate">{m.email}{m.user_id === me && <span className="text-subtle font-normal"> (you)</span>}</p>
                <p className="text-[12px] text-subtle mt-0.5">joined {timeAgo(m.created_at)}</p>
              </div>
              {mayEdit(m) ? (
                <select className="input h-8 w-auto" value={m.role} disabled={busy} onChange={(e) => setRole(m, e.target.value)} aria-label={`Role of ${m.email}`}>
                  {ROLES.filter((r) => org.role === "owner" || r !== "owner").map((r) => <option key={r} value={r}>{r}</option>)}
                </select>
              ) : <span className="text-[12px] text-muted">{m.role}</span>}
              {(mayEdit(m) || m.user_id === me) && (
                <button className="btn-ghost text-danger" disabled={busy} onClick={() => remove(m)}>{m.user_id === me ? "Leave" : "Remove"}</button>
              )}
            </li>
          ))}
          {invites.map((i) => (
            <li key={i.id} className="flex items-center gap-3 px-5 py-3 border-b border-line last:border-0">
              <div className="min-w-0 flex-1">
                <p className="text-[13px] truncate">{i.email}</p>
                <p className="text-[12px] text-subtle mt-0.5">invited as {i.role} · expires {timeAgo(i.expires_at)}</p>
              </div>
              <button className="btn-ghost" disabled={busy} onClick={() => run("Couldn't cancel", () => api(apiBase, `${base}/invites/${i.id}`, { method: "DELETE", auth }))}>Cancel</button>
            </li>
          ))}
        </ul>
      )}

      <Modal open={inviteOpen} title="Invite someone" onClose={() => setInviteOpen(false)}>
        <form onSubmit={invite} className="space-y-4">
          <Field label="Email" hint="They sign in with this address to accept.">
            <input autoFocus type="email" required className="input" value={draft.email} onChange={(e) => setDraft({ ...draft, email: e.target.value })} placeholder="name@company.com" />
          </Field>
          <Field label="Role" hint={ROLE_HINT[draft.role]}>
            <select className="input" value={draft.role} onChange={(e) => setDraft({ ...draft, role: e.target.value })}>
              {ROLES.filter((r) => org.role === "owner" || r !== "owner").map((r) => <option key={r} value={r}>{r}</option>)}
            </select>
          </Field>
          <div className="flex justify-end gap-2">
            <button type="button" className="btn-ghost" onClick={() => setInviteOpen(false)}>Cancel</button>
            <button className="btn-primary" disabled={busy}>{busy ? "Inviting…" : "Send invitation"}</button>
          </div>
        </form>
      </Modal>
    </Panel>
  );
}

function TenantKeys({ apiBase, auth, org, notify, onCopy }) {
  const [keys, setKeys] = useState(null);
  const [created, setCreated] = useState(null);
  const [revoking, setRevoking] = useState(null);
  const [busy, setBusy] = useState(false);
  const base = `/tenant/${org.tenant_id}/api-keys`;

  const load = useCallback(async () => {
    try { setKeys(await api(apiBase, base, { auth })); } catch (err) { notify("Couldn't load keys", err.message, true); setKeys([]); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken, base]);
  useEffect(() => { load(); }, [load]);

  const create = async () => {
    setBusy(true);
    try { setCreated(await api(apiBase, base, { method: "POST", auth, json: { name: `${org.name} server` } })); load(); }
    catch (err) { notify("Couldn't create key", err.message, true); }
    finally { setBusy(false); }
  };
  const revoke = async () => {
    setBusy(true);
    try { await api(apiBase, `${base}/${revoking.id}`, { method: "DELETE", auth }); notify("Key revoked", revoking.name); setRevoking(null); load(); }
    catch (err) { notify("Couldn't revoke key", err.message, true); }
    finally { setBusy(false); }
  };

  return (
    <Panel
      title="Server keys"
      description="Your backend sends one of these with X-Memro-User: <your user's ID>. Each user gets their own memory. Keep keys on your servers only."
      action={<button className="btn-secondary h-8" onClick={create} disabled={busy}><Icon name="add" className="!text-[18px]" /> New key</button>}
      bodyClass="p-0"
    >
      {created && (
        <div className="p-5 border-b border-line">
          <p className="text-[13px] font-medium">Copy your new key now</p>
          <p className="text-[12px] text-muted mt-1">You won't be able to see it again. It can act for any of your users.</p>
          <div className="flex items-center gap-2 mt-2">
            <code className="font-mono text-[12px] bg-panel border border-line rounded-md px-2 py-1.5 truncate flex-1">{created.key}</code>
            <button className="btn-secondary h-8 px-2.5" onClick={() => onCopy(created.key, "Server key")} aria-label="Copy key"><Icon name="content_copy" className="!text-[16px]" /></button>
          </div>
          <button className="btn-ghost mt-2 -ml-2" onClick={() => setCreated(null)}>I've saved it</button>
        </div>
      )}
      {keys === null ? (
        <div className="p-5"><Skeleton className="h-4 w-1/2" /></div>
      ) : keys.length === 0 ? (
        <Empty icon="key" title="No server keys yet">Create one for your backend.</Empty>
      ) : (
        <ul>
          {keys.map((k) => (
            <li key={k.id} className="flex items-center gap-4 px-5 py-3 border-b border-line last:border-0">
              <div className="min-w-0 flex-1">
                <p className="text-[13px] font-medium truncate">{k.name}</p>
                <p className="text-[12px] text-subtle font-mono mt-0.5">{k.prefix}… · created {timeAgo(k.created_at)} · {k.last_used_at ? `used ${timeAgo(k.last_used_at)}` : "never used"}</p>
              </div>
              <button className="btn-ghost text-danger" onClick={() => setRevoking(k)}>Revoke</button>
            </li>
          ))}
        </ul>
      )}
      <Modal open={!!revoking} title="Revoke key?" onClose={() => setRevoking(null)}>
        <p className="text-[13px] text-muted">Your backend stops reaching Memro with <span className="text-ink font-medium">{revoking?.name}</span> immediately. Create and deploy a new key first.</p>
        <div className="flex justify-end gap-2 mt-5">
          <button className="btn-ghost" onClick={() => setRevoking(null)}>Cancel</button>
          <button className="btn-danger" onClick={revoke} disabled={busy}>{busy ? "Revoking…" : "Revoke"}</button>
        </div>
      </Modal>
    </Panel>
  );
}
