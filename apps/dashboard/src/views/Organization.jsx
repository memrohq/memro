import { useCallback, useEffect, useMemo, useState } from "react";
import { Panel, Icon, Modal, Field, Empty, timeAgo, PageHeader, Skeleton, Segmented, TypePill } from "../components/ui";
import { api } from "../lib/api";
import Graph from "./Graph";

// Organisations (ADR 0026): the people who manage a customer's account, its
// tenant keys and usage per end user. Members never see end users' memories.
// A work email may create one; a verified domain (DNS TXT) proves the company
// and lets colleagues ask to join (ADR 0027).

const ROLES = ["owner", "admin", "billing", "viewer"];
const ROLE_HINT = {
  owner: "Everything, including managing owners",
  admin: "Members, invitations, tenant keys and usage",
  billing: "Usage and billing",
  viewer: "Read-only usage",
};
const thisMonth = () => new Date().toISOString().slice(0, 7);
const PAGE_SIZE = 50;
const compact = (n) => Intl.NumberFormat(undefined, { notation: "compact" }).format(n || 0);
const pct = (used, max) => (max ? Math.floor((used * 100) / max) : 0);

// The plan line and, when something needs attention, a banner (ADR 0028).
function planSummary({ limits: l, stored_tokens, queries_this_month, seats, data_deleted_at }) {
  if (!l) return null;
  const parts = [`${l.plan[0].toUpperCase()}${l.plan.slice(1)} plan`];
  if (l.max_stored_tokens_total) parts.push(`${compact(stored_tokens)} of ${compact(l.max_stored_tokens_total)} stored tokens`);
  if (l.max_queries_monthly) parts.push(`${compact(queries_this_month)} of ${compact(l.max_queries_monthly)} queries this month`);
  if (l.max_members) parts.push(`${seats} of ${l.max_members} seats`);
  let alert = null;
  const ended = l.trial_ends_at && new Date(l.trial_ends_at) < new Date();
  if (ended && l.trial_notice === "deleted") {
    alert = { danger: true, title: "Your trial's data was deleted", text: "Choose a plan to start saving again." };
  } else if (ended) {
    alert = { danger: true, title: "Your trial has ended", text: `Memories are read-only and will be deleted on ${new Date(data_deleted_at).toLocaleDateString()} unless you choose a plan.` };
  } else {
    const worst = Math.max(pct(stored_tokens, l.max_stored_tokens_total), pct(queries_this_month, l.max_queries_monthly));
    if (worst >= 80) alert = { danger: worst >= 100, title: `You've used ${worst}% of your plan`, text: "When a limit is reached, Memro stops saving (or searching). Contact Memro to upgrade." };
    else if (l.trial_ends_at) parts.push(`trial ends ${new Date(l.trial_ends_at).toLocaleDateString()}`);
  }
  return { text: parts.join(" · "), alert };
}
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
      <PageHeader title="Organization" description="People who manage your company's Memro account, its server keys, and what each of your users costs. Owners and admins can open a user's memories, and every view is logged; other members see usage only.">
        {orgs?.length > 1 && (
          <select className="input h-9 w-auto" value={orgId || ""} onChange={(e) => setOrgId(e.target.value)} aria-label="Organisation">
            {orgs.map((o) => <option key={o.tenant_id} value={o.tenant_id}>{o.name}</option>)}
          </select>
        )}
      </PageHeader>

      {orgs === null ? (
        <div className="card p-5 space-y-3"><Skeleton className="h-4 w-1/2" /><Skeleton className="h-4 w-1/3" /></div>
      ) : !org ? (
        <NoOrg apiBase={apiBase} auth={auth} notify={notify} onCreated={(m) => { setOrgId(m.tenant_id); loadOrgs(); }} />
      ) : (
        <>
          <p className="text-[13px] text-muted">{org.name} · you're <span className="text-ink font-medium">{org.role}</span></p>
          <Usage key={`u-${org.tenant_id}`} apiBase={apiBase} auth={auth} org={org} notify={notify} canRead={manage} />
          <Members key={`m-${org.tenant_id}`} apiBase={apiBase} auth={auth} org={org} manage={manage} notify={notify} onCopy={onCopy} onLeft={loadOrgs} />
          {manage && <JoinRequests key={`j-${org.tenant_id}`} apiBase={apiBase} auth={auth} org={org} notify={notify} />}
          {manage && <Domains key={`d-${org.tenant_id}`} apiBase={apiBase} auth={auth} org={org} notify={notify} onCopy={onCopy} />}
          {manage && <TenantKeys key={`k-${org.tenant_id}`} apiBase={apiBase} auth={auth} org={org} notify={notify} onCopy={onCopy} />}
        </>
      )}
    </div>
  );
}

function Usage({ apiBase, auth, org, notify, canRead }) {
  const [viewing, setViewing] = useState(null);
  const [month, setMonth] = useState(thisMonth);
  const [data, setData] = useState(null);
  const [limit, setLimit] = useState(null);
  const [query, setQuery] = useState("");
  const [hideDeleted, setHideDeleted] = useState(false);
  const [page, setPage] = useState(0);
  useEffect(() => { setPage(0); }, [query, hideDeleted, month]);
  // shortcut: filters and pages in the browser over the whole month's list;
  // page on the server once organisations have tens of thousands of users.
  const rows = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (data?.members || []).filter((m) => (!hideDeleted || m.user != null)
      && (!q || (m.user || "").toLowerCase().includes(q) || m.agent_id.toLowerCase().startsWith(q)));
  }, [data, query, hideDeleted]);
  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
  const shown = rows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  useEffect(() => {
    api(apiBase, `/tenant/${org.tenant_id}/limits`, { auth }).then((u) => setLimit(planSummary(u)))
      .catch(() => setLimit(null));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken, org.tenant_id]);

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
    <>
    {limit?.alert && (
      <div className={`card p-4 text-[13px] ${limit.alert.danger ? "border-danger" : ""}`} role="status">
        <p className="font-medium">{limit.alert.title}</p>
        <p className="text-muted mt-0.5">{limit.alert.text}</p>
      </div>
    )}
    <Panel
      title="Usage per user"
      description={data ? `${num(data.active_members)} active users · ${num(data.totals?.queries)} searches · estimated model cost ${usd(data.cost_usd)}${limit?.text ? ` · ${limit.text}` : ""}` : "Loading…"}
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
        <>
        <div className="flex flex-wrap items-center gap-3 px-5 py-3 border-b border-line">
          <input className="input h-8 w-64" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search by user ID" aria-label="Search users" />
          <label className="flex items-center gap-2 text-[12.5px] text-muted">
            <input type="checkbox" checked={hideDeleted} onChange={(e) => setHideDeleted(e.target.checked)} /> Hide deleted users
          </label>
        </div>
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
              {shown.length === 0 && (
                <tr><td colSpan={5} className="px-5 py-6 text-center text-subtle">No users match.</td></tr>
              )}
              {shown.map((m) => {
                const t = m.metrics;
                const tokens = (t.stored_tokens || 0) + (t.query_tokens || 0) + (t.vision_input_tokens || 0) + (t.vision_output_tokens || 0);
                return (
                  <tr key={m.agent_id} className="border-b border-line last:border-0">
                    <td className="px-5 py-2.5 font-mono text-[12px] truncate max-w-[220px]" title={m.agent_id}>
                      {m.user == null ? <span className="text-subtle">deleted user</span>
                        : canRead ? <button className="hover:underline text-left" onClick={() => setViewing(m)}>{m.user}</button> : m.user}
                    </td>
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
        <div className="flex items-center justify-between gap-3 px-5 py-3 border-t border-line text-[12.5px] text-muted">
          <span className="tabular-nums">{rows.length ? `${num(page * PAGE_SIZE + 1)}–${num(page * PAGE_SIZE + shown.length)} of ${num(rows.length)}` : "0 users"}</span>
          <div className="flex gap-2">
            <button className="btn-secondary h-8" disabled={page === 0} onClick={() => setPage(page - 1)}>Previous</button>
            <button className="btn-secondary h-8" disabled={page >= pages - 1} onClick={() => setPage(page + 1)}>Next</button>
          </div>
        </div>
        </>
      )}
    </Panel>
    {viewing && <EndUserView apiBase={apiBase} auth={auth} org={org} member={viewing} onClose={() => setViewing(null)} />}
    </>
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

// Not in an organisation yet: ask to join one at your verified domain, or
// create one with a work email.
function NoOrg({ apiBase, auth, notify, onCreated }) {
  const [info, setInfo] = useState(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try { setInfo(await api(apiBase, "/orgs/discover", { auth })); }
    catch (err) { notify("Couldn't load organisations", err.message, true); setInfo({ orgs: [] }); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken]);
  useEffect(() => { load(); }, [load]);

  const ask = async (o) => {
    setBusy(true);
    try { await api(apiBase, `/orgs/${o.tenant_id}/join-requests`, { method: "POST", auth }); notify("Request sent", `An owner of ${o.name} will review it`); load(); }
    catch (err) { notify("Couldn't send request", err.message, true); }
    finally { setBusy(false); }
  };
  const create = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      const m = await api(apiBase, "/orgs", { method: "POST", auth, json: { name } });
      if (m.trial_started) notify("Organisation created", `${m.name} is on a 30-day free trial`);
      else notify("Organisation created", `${info.domain} already used its free trial, so ${m.name} can't save memories until it has a plan. Contact Memro.`, true);
      onCreated(m);
    }
    catch (err) { notify("Couldn't create organisation", err.message, true); }
    finally { setBusy(false); }
  };

  if (info === null) return <div className="card p-5"><Skeleton className="h-4 w-1/2" /></div>;
  return (
    <div className="space-y-6">
      {info.orgs.length > 0 && (
        <Panel title="Your company is on Memro" description={`Organisations that verified ${info.domain}. An owner approves each request.`} bodyClass="p-0">
          <ul>
            {info.orgs.map((o) => (
              <li key={o.tenant_id} className="flex items-center gap-3 px-5 py-3 border-b border-line last:border-0">
                <p className="text-[13px] font-medium flex-1 truncate">{o.name}</p>
                {o.requested ? <span className="text-[12px] text-muted">Request pending</span>
                  : <button className="btn-primary h-8" disabled={busy} onClick={() => ask(o)}>Request to join</button>}
              </li>
            ))}
          </ul>
        </Panel>
      )}
      {info.work_email ? (
        <Panel title="Create an organisation" description="You'll be its owner. Verify your company's domain afterwards to create server keys and let colleagues ask to join.">
          <form onSubmit={create} className="flex items-end gap-2">
            <div className="flex-1"><Field label="Company name"><input required maxLength={100} className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Acme Inc." /></Field></div>
            <button className="btn-primary" disabled={busy || !name.trim()}>{busy ? "Creating…" : "Create"}</button>
          </form>
        </Panel>
      ) : (
        <Empty icon="corporate_fare" title="You're not in an organisation">Sign in with your work email to create one, or ask an owner of your company's account to invite you.</Empty>
      )}
    </div>
  );
}

// Requests from people at a verified domain; nothing shows when there are none.
function JoinRequests({ apiBase, auth, org, notify }) {
  const [reqs, setReqs] = useState([]);
  const [roles, setRoles] = useState({});
  const [busy, setBusy] = useState(false);
  const base = `/tenant/${org.tenant_id}/join-requests`;

  const load = useCallback(async () => {
    try { setReqs(await api(apiBase, base, { auth })); } catch (err) { notify("Couldn't load join requests", err.message, true); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken, base]);
  useEffect(() => { load(); }, [load]);

  const decide = async (r, approve) => {
    setBusy(true);
    try {
      await api(apiBase, `${base}/${r.id}`, approve ? { method: "POST", auth, json: { role: roles[r.id] || "viewer" } } : { method: "DELETE", auth });
      notify(approve ? "Request approved" : "Request declined", r.email);
      load();
    } catch (err) { notify("Couldn't update request", err.message, true); }
    finally { setBusy(false); }
  };

  if (!reqs.length) return null;
  return (
    <Panel title={`Join requests · ${reqs.length}`} description="People at your verified domain asking to join. They get only the role you pick." bodyClass="p-0">
      <ul>
        {reqs.map((r) => (
          <li key={r.id} className="flex items-center gap-3 px-5 py-3 border-b border-line last:border-0">
            <div className="min-w-0 flex-1">
              <p className="text-[13px] font-medium truncate">{r.email}</p>
              <p className="text-[12px] text-subtle mt-0.5">asked {timeAgo(r.created_at)}</p>
            </div>
            <select className="input h-8 w-auto" value={roles[r.id] || "viewer"} onChange={(e) => setRoles({ ...roles, [r.id]: e.target.value })} aria-label={`Role for ${r.email}`}>
              {ROLES.filter((x) => org.role === "owner" || x !== "owner").map((x) => <option key={x} value={x}>{x}</option>)}
            </select>
            <button className="btn-primary h-8" disabled={busy} onClick={() => decide(r, true)}>Approve</button>
            <button className="btn-ghost" disabled={busy} onClick={() => decide(r, false)}>Decline</button>
          </li>
        ))}
      </ul>
    </Panel>
  );
}

// Company domains, proven with a DNS TXT record and re-checked weekly.
function Domains({ apiBase, auth, org, notify, onCopy }) {
  const [data, setData] = useState(null);
  const [domain, setDomain] = useState("");
  const [busy, setBusy] = useState(false);
  const base = `/tenant/${org.tenant_id}`;

  const load = useCallback(async () => {
    try { setData(await api(apiBase, `${base}/domains`, { auth })); }
    catch (err) { notify("Couldn't load domains", err.message, true); setData({ domains: [], join_policy: "invite" }); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken, base]);
  useEffect(() => { load(); }, [load]);

  const run = async (what, fn) => {
    setBusy(true);
    try { await fn(); load(); } catch (err) { notify(what, err.message, true); } finally { setBusy(false); }
  };
  const claim = (e) => {
    e.preventDefault();
    run("Couldn't add domain", async () => { await api(apiBase, `${base}/domains`, { method: "POST", auth, json: { domain } }); setDomain(""); });
  };
  const verify = (d) => run("Not verified yet", async () => { await api(apiBase, `${base}/domains/${d.domain}/verify`, { method: "POST", auth }); notify("Domain verified", d.domain); });
  const remove = (d) => run("Couldn't remove domain", () => api(apiBase, `${base}/domains/${d.domain}`, { method: "DELETE", auth }));
  const setPolicy = (join_policy) => run("Couldn't change who can join", () => api(apiBase, base, { method: "PATCH", auth, json: { join_policy } }));
  const anyVerified = data?.domains.some((d) => d.verified);

  return (
    <Panel
      title="Domains"
      description="Prove your company owns its domain with a DNS TXT record. A verified domain unlocks server keys and lets colleagues ask to join."
      bodyClass="p-0"
    >
      {data === null ? (
        <div className="p-5"><Skeleton className="h-4 w-1/2" /></div>
      ) : (
        <>
          <ul>
            {data.domains.map((d) => (
              <li key={d.domain} className="px-5 py-3 border-b border-line">
                <div className="flex items-center gap-3">
                  <p className="text-[13px] font-medium flex-1 truncate">{d.domain}</p>
                  <span className={`text-[12px] ${d.verified ? "text-ink" : "text-muted"}`}>{d.verified ? `Verified${d.txt_name ? "" : " by Memro"}` : "Not verified"}</span>
                  {!d.verified && <button className="btn-secondary h-8" disabled={busy} onClick={() => verify(d)}>Verify</button>}
                  <button className="btn-ghost text-danger" disabled={busy} onClick={() => remove(d)}>Remove</button>
                </div>
                {!d.verified && d.txt_name && (
                  <div className="mt-2 space-y-1.5">
                    <p className="text-[12px] text-muted">Add this TXT record at your DNS provider, then press Verify. Changes can take up to an hour.</p>
                    {[["Name", d.txt_name], ["Value", d.txt_value]].map(([label, v]) => (
                      <div key={label} className="flex items-center gap-2">
                        <span className="text-[12px] text-subtle w-12">{label}</span>
                        <code className="font-mono text-[12px] bg-panel border border-line rounded-md px-2 py-1.5 truncate flex-1">{v}</code>
                        <button className="btn-secondary h-8 px-2.5" onClick={() => onCopy(v, `TXT ${label.toLowerCase()}`)} aria-label={`Copy TXT ${label.toLowerCase()}`}><Icon name="content_copy" className="!text-[16px]" /></button>
                      </div>
                    ))}
                  </div>
                )}
              </li>
            ))}
          </ul>
          <form onSubmit={claim} className="flex items-end gap-2 p-5 border-b border-line">
            <div className="flex-1"><Field label="Add a domain"><input required className="input" value={domain} onChange={(e) => setDomain(e.target.value)} placeholder="example.com" /></Field></div>
            <button className="btn-secondary" disabled={busy || !domain.trim()}>Add</button>
          </form>
          <div className="flex items-center gap-3 p-5">
            <div className="flex-1">
              <p className="text-[13px] font-medium">Who can join</p>
              <p className="text-[12px] text-subtle mt-0.5">{anyVerified ? "People at a verified domain may ask; an owner or admin approves each one." : "Verify a domain to let colleagues ask to join."}</p>
            </div>
            <select className="input h-8 w-auto" value={data.join_policy} disabled={busy || !anyVerified} onChange={(e) => setPolicy(e.target.value)} aria-label="Who can join">
              <option value="invite">Invitation only</option>
              <option value="request">Verified domain can request</option>
            </select>
          </div>
        </>
      )}
    </Panel>
  );
}

// One end user's memories and graph, read-only (ADR 0030). Opening it is
// written to the organisation's audit log.
function EndUserView({ apiBase, auth, org, member, onClose }) {
  const [tab, setTab] = useState("memories");
  const [mems, setMems] = useState(null);
  const base = `/tenant/${org.tenant_id}/end-users/${member.agent_id}`;
  useEffect(() => {
    api(apiBase, `${base}/memories?limit=100`, { auth }).then(setMems).catch((err) => setMems({ error: err.message }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [base]);
  return (
    <Modal open title={`Memories of ${member.user}`} description="Read-only. This view is recorded in your organisation's audit log." onClose={onClose} width="max-w-5xl">
      <Segmented options={[{ id: "memories", label: "Memories" }, { id: "graph", label: "Graph" }]} value={tab} onChange={setTab} size="sm" />
      <div className="mt-4">
        {tab === "graph" ? (
          <Graph apiBase={apiBase} auth={auth} path={`${base}/graph`} title="" description="" height="60vh" />
        ) : mems === null ? (
          <Skeleton className="h-4 w-1/2" />
        ) : mems.error ? (
          <Empty icon="error" title="Couldn't load memories">{mems.error}</Empty>
        ) : mems.length === 0 ? (
          <Empty icon="stacks" title="No memories">This user hasn't saved anything, or deleted it.</Empty>
        ) : (
          <ul className="max-h-[60vh] overflow-y-auto scroll-thin divide-y divide-line border border-line rounded-lg">
            {mems.map((m) => (
              <li key={m.id} className="px-4 py-3">
                <p className="text-[13.5px] whitespace-pre-wrap break-words">{m.content}</p>
                <div className="flex items-center gap-2 mt-1.5"><TypePill type={m.memory_type} /><span className="text-[12px] text-subtle">{timeAgo(m.created_at)}</span></div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}
