import { useCallback, useEffect, useMemo, useState } from "react";
import { Panel, Icon, Modal, Field, Empty, timeAgo, PageHeader, Skeleton, Logo, Segmented, Drawer } from "../components/ui";
import { api } from "../lib/api";

// The staff console at /admin (ADR 0028, 0029): all of Memro for Memro staff,
// apart from any account they belong to. Overview, every organisation (plans,
// limits, trials) and every person (staff rights). The server allows all of
// this to platform admins only.

const PLANS = ["trial", "enterprise", "custom"];
const num = (n) => (n || 0).toLocaleString();
const compact = (n) => Intl.NumberFormat(undefined, { notation: "compact" }).format(n || 0);
const day = 24 * 60 * 60 * 1000;
const dateInput = (iso) => (iso ? iso.slice(0, 10) : "");

function planLabel(t) {
  if (!t.limits) return "Self-serve tier"; // Free, or Builder/Pro/Scale bought through checkout
  return t.limits.plan[0].toUpperCase() + t.limits.plan.slice(1);
}

function trialState(l) {
  if (!l?.trial_ends_at) return null;
  const left = new Date(l.trial_ends_at) - Date.now();
  return left < 0 ? { text: `ended ${timeAgo(l.trial_ends_at)}`, ended: true } : { text: `ends in ${Math.ceil(left / day)} days`, ended: false };
}

const TABS = [{ id: "overview", label: "Overview" }, { id: "orgs", label: "Organisations" }, { id: "users", label: "Users" }, { id: "activity", label: "Activity" }];

export default function AdminConsole({ apiBase, auth, notify, onCopy, email, allowed, me, onSignOut }) {
  const [tab, setTab] = useState(() => {
    const t = new URLSearchParams(window.location.search).get("tab");
    return TABS.some((x) => x.id === t) ? t : "overview";
  });
  const pick = (t) => { setTab(t); window.history.replaceState(null, "", t === "overview" ? "/admin" : `/admin?tab=${t}`); };

  return (
    <div className="min-h-full bg-canvas">
      <header className="h-16 px-4 md:px-8 flex items-center gap-3 border-b border-line bg-surface">
        <Logo className="size-7" />
        <span className="text-[16px] font-semibold tracking-[-0.02em]">memro</span>
        <span className="h-5 px-1.5 rounded-md border border-line text-[10.5px] font-medium text-muted flex items-center">Staff console</span>
        <span className="flex-1" />
        <span className="hidden sm:block text-[12.5px] text-muted truncate">{email}</span>
        <a className="btn-ghost" href="/">Dashboard</a>
        <button className="btn-ghost" onClick={onSignOut}>Sign out</button>
      </header>
      <main className="max-w-[1240px] mx-auto px-4 md:px-8 py-6 md:py-8">
        {allowed === null ? (
          <div className="card p-5 space-y-3"><Skeleton className="h-4 w-1/2" /><Skeleton className="h-4 w-1/3" /></div>
        ) : !allowed ? (
          <Empty icon="lock" title="This console is for Memro staff">
            You're signed in as {email}, which isn't a staff account. Sign out and sign in with your Memro staff address.
          </Empty>
        ) : (
          <div className="space-y-6">
            <Segmented options={TABS} value={tab} onChange={pick} />
            {tab === "overview" && <Overview apiBase={apiBase} auth={auth} notify={notify} onOpen={pick} />}
            {tab === "orgs" && <Organisations apiBase={apiBase} auth={auth} notify={notify} onCopy={onCopy} />}
            {tab === "users" && <Users apiBase={apiBase} auth={auth} notify={notify} me={me} />}
            {tab === "activity" && <Activity apiBase={apiBase} auth={auth} notify={notify} />}
          </div>
        )}
      </main>
    </div>
  );
}

function Stat({ label, icon, value, foot }) {
  return (
    <div className="card p-5 flex flex-col">
      <div className="flex items-center justify-between">
        <p className="text-[13px] text-muted">{label}</p>
        <span className="size-8 rounded-[10px] bg-accent-soft border border-accent-line flex items-center justify-center">
          <Icon name={icon} className="!text-[17px] text-accent" />
        </span>
      </div>
      <p className="text-[30px] leading-none font-semibold tracking-[-0.03em] tabular mt-3">{value}</p>
      {foot && <p className="text-[12.5px] text-muted mt-3">{foot}</p>}
    </div>
  );
}

function Overview({ apiBase, auth, notify, onOpen }) {
  const [o, setO] = useState(null);
  useEffect(() => {
    api(apiBase, "/admin/overview", { auth }).then(setO).catch((err) => { notify("Couldn't load the overview", err.message, true); setO({}); });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken]);

  if (o === null) return <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-4">{[0, 1, 2, 3].map((i) => <div key={i} className="card p-5"><Skeleton className="h-8 w-20" /></div>)}</div>;
  return (
    <div className="space-y-6">
      <PageHeader title="Overview" description="All of Memro: every account, organisation and agent." />
      <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Stat label="People" icon="group" value={num(o.users)} foot={`${num(o.active_users_30d)} signed in in the last 30 days · ${num(o.staff)} staff`} />
        <Stat label="Organisations" icon="corporate_fare" value={num(o.organisations)} foot={`${num(o.orgs_on_contract)} on a contract · ${num(o.orgs_on_trial)} on a trial`} />
        <Stat label="Agents" icon="smart_toy" value={compact(o.agents)} foot={`${compact(o.end_users)} are organisations' users`} />
        <Stat label="Stored" icon="database" value={compact(o.stored_tokens)} foot={`tokens in ${compact(o.memories)} memories · ${compact(o.queries_this_month)} queries this month`} />
      </div>
      <Revenue apiBase={apiBase} auth={auth} notify={notify} />
      {(o.orgs_trial_ending_7d > 0 || o.orgs_trial_ended > 0) && (
        <Panel title="Trials needing attention" description="Ending trials stop saving; ended ones are deleted 60 days after their end unless they choose a plan.">
          <p className="text-[13px]">{num(o.orgs_trial_ending_7d)} ending within 7 days · {num(o.orgs_trial_ended)} ended</p>
          <button className="btn-secondary h-8 mt-3" onClick={() => onOpen("orgs")}>Review organisations</button>
        </Panel>
      )}
    </div>
  );
}

function Users({ apiBase, auth, notify, me }) {
  const [users, setUsers] = useState(null);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(null);
  const [plans, setPlans] = useState({});

  const load = useCallback(async (q = "") => {
    try {
      const list = await api(apiBase, `/admin/users?q=${encodeURIComponent(q)}`, { auth });
      setUsers(list);
      const subjects = list.slice(0, 500).map((u) => `user:${u.id}`).join(",");
      if (subjects) billing(apiBase, auth, `/billing/admin/accounts?subjects=${subjects}`)
        .then((v) => setPlans(Object.fromEntries((v || []).map((a) => [a.subject.slice(5), a])))).catch(() => {});
    }
    catch (err) { notify("Couldn't load users", err.message, true); setUsers([]); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken]);
  useEffect(() => { load(); }, [load]);

  const setStaff = async (u, grant) => {
    setBusy(true);
    try {
      await api(apiBase, `/admin/staff/${u.id}`, { method: grant ? "PUT" : "DELETE", auth });
      notify(grant ? "Now staff" : "No longer staff", u.email);
      load(query.trim());
    } catch (err) { notify("Couldn't change staff rights", err.message, true); }
    finally { setBusy(false); }
  };

  return (
    <div className="space-y-6">
      <PageHeader title="Users" description="Everyone with a Memro account. Agents and storage are their personal ones; organisations' usage is under Organisations." />
      <Panel
        title={`People${users ? ` · ${users.length}${users.length === 500 ? "+" : ""}` : ""}`}
        action={
          <form onSubmit={(e) => { e.preventDefault(); load(query.trim()); }} className="flex gap-2">
            <input className="input h-8 w-56" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search by email" aria-label="Search people" />
            <button className="btn-secondary h-8">Search</button>
          </form>
        }
        bodyClass="p-0"
      >
        {users === null ? (
          <div className="p-5"><Skeleton className="h-4 w-1/2" /></div>
        ) : users.length === 0 ? (
          <Empty icon="group" title="No one found">Try another part of their email address.</Empty>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-[13px]">
              <thead className="text-subtle text-left">
                <tr className="border-b border-line">
                  <th className="font-medium px-5 py-2.5">Person</th>
                  <th className="font-medium px-3 py-2.5">Plan</th>
                  <th className="font-medium px-3 py-2.5">Organisations</th>
                  <th className="font-medium px-3 py-2.5 text-right">Agents</th>
                  <th className="font-medium px-3 py-2.5 text-right">Stored tokens</th>
                  <th className="font-medium px-5 py-2.5"><span className="sr-only">Actions</span></th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr key={u.id} className="border-b border-line last:border-0 align-top">
                    <td className="px-5 py-2.5 max-w-[280px]">
                      <button className="font-medium truncate hover:underline text-left" onClick={() => setOpen(u.id)}>{u.email}</button>
                      {u.staff && <span className="ml-2 text-[11px] px-1.5 py-0.5 rounded-md border border-line text-muted">staff</span>}
                      <p className="text-[12px] text-subtle">joined {timeAgo(u.created_at)} · {u.last_sign_in ? `signed in ${timeAgo(u.last_sign_in)}` : "never signed in"}</p>
                    </td>
                    <td className="px-3 py-2.5 text-[12.5px] capitalize">{plans[u.id] ? `${plans[u.id].plan}${plans[u.id].comp ? " (free)" : ""}` : <span className="text-subtle normal-case">—</span>}</td>
                    <td className="px-3 py-2.5 text-[12.5px]">
                      {u.orgs.length ? u.orgs.map((o) => <p key={o.tenant_id} className="truncate">{o.name} <span className="text-subtle">· {o.role}</span></p>) : <span className="text-subtle">none</span>}
                    </td>
                    <td className="px-3 py-2.5 text-right tabular-nums">{num(u.agents)}</td>
                    <td className="px-3 py-2.5 text-right tabular-nums">{compact(u.stored_tokens)}</td>
                    <td className="px-5 py-2.5 text-right whitespace-nowrap">
                      {u.id === me ? <span className="text-[12px] text-subtle">you</span>
                        : <button className="btn-ghost" disabled={busy} onClick={() => setStaff(u, !u.staff)}>{u.staff ? "Remove staff" : "Make staff"}</button>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>
      {open && <UserDrawer apiBase={apiBase} auth={auth} notify={notify} id={open} me={me} onClose={() => setOpen(null)} onChanged={() => load(query.trim())} />}
    </div>
  );
}

function Organisations({ apiBase, auth, notify, onCopy }) {
  const [tenants, setTenants] = useState(null);
  const [query, setQuery] = useState("");
  const [editing, setEditing] = useState(null);
  const [creating, setCreating] = useState(false);
  const [open, setOpen] = useState(null);

  const load = useCallback(async () => {
    try { setTenants(await api(apiBase, "/admin/tenants", { auth })); }
    catch (err) { notify("Couldn't load organisations", err.message, true); setTenants([]); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken]);
  useEffect(() => { load(); }, [load]);

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (tenants || []).filter((t) => !q || t.name.toLowerCase().includes(q) || t.domains.some((d) => d.includes(q)) || t.id.startsWith(q));
  }, [tenants, query]);

  return (
    <div className="space-y-6">
      <PageHeader title="Organisations" description="Every organisation: its plan, limits, trial and size. Builder, Pro and Scale are bought by the organisation on its billing page.">
        <button className="btn-primary h-9" onClick={() => setCreating(true)}><Icon name="add" className="!text-[18px]" /> New organisation</button>
      </PageHeader>

      <Panel
        title={`Organisations${tenants ? ` · ${tenants.length}` : ""}`}
        action={<input className="input h-8 w-56" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search name, domain or ID" aria-label="Search organisations" />}
        bodyClass="p-0"
      >
        {tenants === null ? (
          <div className="p-5"><Skeleton className="h-4 w-1/2" /></div>
        ) : shown.length === 0 ? (
          <Empty icon="corporate_fare" title="No organisations">{query ? "Nothing matches that search." : "Create one, or wait for customers to sign up."}</Empty>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-[13px]">
              <thead className="text-subtle text-left">
                <tr className="border-b border-line">
                  <th className="font-medium px-5 py-2.5">Organisation</th>
                  <th className="font-medium px-3 py-2.5">Plan</th>
                  <th className="font-medium px-3 py-2.5 text-right">Stored tokens</th>
                  <th className="font-medium px-3 py-2.5 text-right">Queries this month</th>
                  <th className="font-medium px-3 py-2.5 text-right">Users</th>
                  <th className="font-medium px-3 py-2.5 text-right">Members</th>
                  <th className="font-medium px-5 py-2.5"><span className="sr-only">Actions</span></th>
                </tr>
              </thead>
              <tbody>
                {shown.map((t) => {
                  const trial = trialState(t.limits);
                  const pool = t.limits?.max_stored_tokens_total;
                  const qmax = t.limits?.max_queries_monthly;
                  return (
                    <tr key={t.id} className="border-b border-line last:border-0">
                      <td className="px-5 py-2.5 max-w-[260px]">
                        <button className="font-medium truncate hover:underline text-left max-w-full" onClick={() => setOpen(t.id)}>{t.name}</button>
                        <p className="text-[12px] text-subtle truncate">{t.domains.join(", ") || "no verified domain"} · {t.self_serve ? "self-service" : "staff"} · {timeAgo(t.created_at)}</p>
                      </td>
                      <td className="px-3 py-2.5">
                        <p>{planLabel(t)}</p>
                        {trial && <p className={`text-[12px] ${trial.ended ? "text-danger" : "text-subtle"}`}>{trial.text}</p>}
                      </td>
                      <td className="px-3 py-2.5 text-right tabular-nums">{compact(t.stored_tokens)}{pool ? <span className="text-subtle"> / {compact(pool)}</span> : null}</td>
                      <td className="px-3 py-2.5 text-right tabular-nums">{compact(t.queries_this_month)}{qmax ? <span className="text-subtle"> / {compact(qmax)}</span> : null}</td>
                      <td className="px-3 py-2.5 text-right tabular-nums">{num(t.end_users)}</td>
                      <td className="px-3 py-2.5 text-right tabular-nums">{num(t.members)}</td>
                      <td className="px-5 py-2.5 text-right"><button className="btn-secondary h-8" onClick={() => setEditing(t)}>Manage</button></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      {open && <OrgDrawer apiBase={apiBase} auth={auth} notify={notify} id={open} onClose={() => setOpen(null)} onChanged={load} onManage={(t) => setEditing(t)} />}
      {editing && <ManageLimits apiBase={apiBase} auth={auth} notify={notify} tenant={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load(); }} />}
      {creating && <CreateOrg apiBase={apiBase} auth={auth} notify={notify} onCopy={onCopy} onClose={() => setCreating(false)} onCreated={load} />}
    </div>
  );
}

// Blank inputs mean "no limit" / "no end". Plans and prices: core ADR 0028,
// memro-cloud ADR 0012.
const PRESETS = {
  trial: () => ({ perUser: "", total: 100000, queries: 5000, seats: 3, images: false, trialEnds: new Date(Date.now() + 30 * day).toISOString().slice(0, 10) }),
  enterprise: () => ({ perUser: "", total: "", queries: "", seats: "", images: true, trialEnds: "" }),
};
const PLAN_HINT = {
  trial: "Free: 100k stored tokens and 5k queries a month shared, 3 seats, no images, 30 days. Read-only after, deleted 60 days later.",
  enterprise: "From $1,500/month, annual; 100M stored tokens and 5M queries included; overage billed.",
  custom: "Your own limits; billed like Enterprise.",
};

function ManageLimits({ apiBase, auth, notify, tenant, onClose, onSaved }) {
  const l = tenant.limits;
  const [draft, setDraft] = useState({
    plan: l?.plan || "enterprise",
    perUser: l?.max_stored_tokens_per_user ?? "",
    total: l?.max_stored_tokens_total ?? "",
    queries: l?.max_queries_monthly ?? "",
    seats: l?.max_members ?? "",
    images: l ? l.allow_images : true,
    trialEnds: dateInput(l?.trial_ends_at),
  });
  const [busy, setBusy] = useState(false);
  const set = (k) => (e) => setDraft({ ...draft, [k]: e.target.value });

  const extend = (days) => {
    const from = draft.trialEnds && new Date(draft.trialEnds) > Date.now() ? new Date(draft.trialEnds) : new Date();
    setDraft({ ...draft, trialEnds: new Date(from.getTime() + days * day).toISOString().slice(0, 10) });
  };
  const toNum = (v) => (v === "" ? null : Number(v));
  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api(apiBase, `/tenant/${tenant.id}/limits`, { method: "PUT", auth, json: {
        plan: draft.plan,
        max_stored_tokens_per_user: toNum(draft.perUser),
        max_stored_tokens_total: toNum(draft.total),
        max_queries_monthly: toNum(draft.queries),
        max_members: toNum(draft.seats),
        allow_images: draft.images,
        trial_ends_at: draft.trialEnds ? new Date(`${draft.trialEnds}T23:59:59Z`).toISOString() : null,
      } });
      notify("Plan saved", tenant.name);
      onSaved();
    } catch (err) { notify("Couldn't save the plan", err.message, true); }
    finally { setBusy(false); }
  };
  const reset = async () => {
    setBusy(true);
    try { await api(apiBase, `/tenant/${tenant.id}/limits`, { method: "DELETE", auth }); notify("Back to default limits", tenant.name); onSaved(); }
    catch (err) { notify("Couldn't reset limits", err.message, true); }
    finally { setBusy(false); }
  };
  // Picking a plan fills in its preset; staff can change any value after.
  const pick = (plan) => setDraft({ ...draft, plan, ...(PRESETS[plan]?.() || {}) });

  return (
    <Modal open title={`Manage ${tenant.name}`} onClose={onClose}>
      <form onSubmit={save} className="space-y-4">
        <p className="text-[12px] text-subtle">{num(tenant.stored_tokens)} stored tokens · {num(tenant.queries_this_month)} queries this month · {num(tenant.end_users)} users · {num(tenant.members)} members</p>
        <Field label="Plan" hint={PLAN_HINT[draft.plan]}>
          <select className="input" value={draft.plan} onChange={(e) => pick(e.target.value)}>
            {PLANS.map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </Field>
        <p className="text-[12px] text-subtle -mt-2">Hard limits below; blank means none. Builder, Pro and Scale aren't set here: the organisation buys them on its billing page, and "Use default limits" hands it back to that tier.</p>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Stored tokens, shared"><input type="number" min="1" className="input" value={draft.total} onChange={set("total")} placeholder="No limit" /></Field>
          <Field label="Stored tokens per user"><input type="number" min="1" className="input" value={draft.perUser} onChange={set("perUser")} placeholder="No limit" /></Field>
          <Field label="Queries a month, shared"><input type="number" min="1" className="input" value={draft.queries} onChange={set("queries")} placeholder="No limit" /></Field>
          <Field label="Seats"><input type="number" min="1" className="input" value={draft.seats} onChange={set("seats")} placeholder="No limit" /></Field>
        </div>
        <label className="flex items-center gap-2 text-[13px]">
          <input type="checkbox" checked={draft.images} onChange={(e) => setDraft({ ...draft, images: e.target.checked })} /> Image analysis
        </label>
        <Field label="Trial ends" hint="After this date saving stops; reading and deleting still work for 60 days, then the data is deleted.">
          <div className="flex items-center gap-2">
            <input type="date" className="input" value={draft.trialEnds} onChange={set("trialEnds")} />
            <button type="button" className="btn-secondary h-9" onClick={() => extend(14)}>+14 days</button>
            <button type="button" className="btn-secondary h-9" onClick={() => extend(30)}>+30 days</button>
            {draft.trialEnds && <button type="button" className="btn-ghost" onClick={() => setDraft({ ...draft, trialEnds: "" })}>No end</button>}
          </div>
        </Field>
        <div className="flex items-center justify-between gap-2 pt-1">
          <button type="button" className="btn-ghost" disabled={busy || !tenant.limits} onClick={reset}>Use default limits</button>
          <div className="flex gap-2">
            <button type="button" className="btn-ghost" onClick={onClose}>Cancel</button>
            <button className="btn-primary" disabled={busy}>{busy ? "Saving…" : "Save"}</button>
          </div>
        </div>
      </form>
    </Modal>
  );
}

function CreateOrg({ apiBase, auth, notify, onCopy, onClose, onCreated }) {
  const [draft, setDraft] = useState({ name: "", domain: "", owner_email: "" });
  const [link, setLink] = useState(null);
  const [busy, setBusy] = useState(false);

  const create = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      const body = { name: draft.name.trim() };
      if (draft.domain.trim()) body.domain = draft.domain.trim();
      if (draft.owner_email.trim()) body.owner_email = draft.owner_email.trim();
      const res = await api(apiBase, "/tenant", { method: "POST", auth, json: body });
      notify("Organisation created", body.name);
      onCreated();
      if (res.owner_invite && !res.owner_invite.emailed) setLink(res.owner_invite.link);
      else onClose();
    } catch (err) { notify("Couldn't create organisation", err.message, true); }
    finally { setBusy(false); }
  };

  return (
    <Modal open title="New organisation" onClose={onClose}>
      {link ? (
        <div className="space-y-3">
          <p className="text-[13px]">Email isn't set up here, so send this link to the owner. Only their address can use it, once, within 7 days.</p>
          <div className="flex items-center gap-2">
            <code className="font-mono text-[12px] bg-panel border border-line rounded-md px-2 py-1.5 truncate flex-1">{link}</code>
            <button className="btn-secondary h-8 px-2.5" onClick={() => onCopy(link, "Invitation link")} aria-label="Copy link"><Icon name="content_copy" className="!text-[16px]" /></button>
          </div>
          <div className="flex justify-end"><button className="btn-primary" onClick={onClose}>Done</button></div>
        </div>
      ) : (
        <form onSubmit={create} className="space-y-4">
          <Field label="Name"><input autoFocus required className="input" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} placeholder="Acme Inc." /></Field>
          <Field label="Domain" hint="Optional. You vouch for it, so it counts as verified."><input className="input" value={draft.domain} onChange={(e) => setDraft({ ...draft, domain: e.target.value })} placeholder="acme.com" /></Field>
          <Field label="Owner email" hint="Optional. They get an invitation to own the organisation."><input type="email" className="input" value={draft.owner_email} onChange={(e) => setDraft({ ...draft, owner_email: e.target.value })} placeholder="founder@acme.com" /></Field>
          <p className="text-[12px] text-subtle">Staff-created organisations use the default limits until you set a plan.</p>
          <div className="flex justify-end gap-2">
            <button type="button" className="btn-ghost" onClick={onClose}>Cancel</button>
            <button className="btn-primary" disabled={busy || !draft.name.trim()}>{busy ? "Creating…" : "Create"}</button>
          </div>
        </form>
      )}
    </Modal>
  );
}

// --- Detail drawers, account actions, plans, activity (ADR 0029) --------------------

const TIERS = ["builder", "pro", "scale"];
const MONTH = (iso) => new Date(iso).toLocaleDateString(undefined, { month: "short", year: "numeric", timeZone: "UTC" });

// billing calls Memro Cloud's billing API; null when this server has none.
async function billing(apiBase, auth, path, opts = {}) {
  try { return await api(apiBase, path, { auth, ...opts }); }
  catch (err) { if (err.status === 404 || err.status === 405) return null; throw err; }
}

function Section({ title, children, empty }) {
  return (
    <section className="space-y-2">
      <h4 className="text-[12px] font-medium uppercase tracking-wide text-subtle">{title}</h4>
      {children || <p className="text-[13px] text-subtle">{empty || "None"}</p>}
    </section>
  );
}

function Rows({ items, render }) {
  if (!items?.length) return null;
  return <ul className="text-[13px] divide-y divide-line border border-line rounded-lg">{items.map((x, i) => <li key={i} className="px-3 py-2">{render(x)}</li>)}</ul>;
}

// Usage per month as a table: one row per month, one column per metric.
function UsageTable({ usage }) {
  if (!usage?.length) return null;
  const metrics = ["queries", "memories_stored", "stored_tokens_held", "stored_tokens"].filter((m) => usage.some((u) => u.metric === m));
  const months = [...new Set(usage.map((u) => u.month))];
  const at = (month, metric) => usage.find((u) => u.month === month && u.metric === metric)?.value || 0;
  const label = { queries: "Queries", memories_stored: "Memories saved", stored_tokens_held: "Tokens held", stored_tokens: "Tokens written" };
  return (
    <div className="overflow-x-auto border border-line rounded-lg">
      <table className="w-full text-[12.5px]">
        <thead className="text-subtle text-left"><tr className="border-b border-line"><th className="font-medium px-3 py-2">Month</th>{metrics.map((m) => <th key={m} className="font-medium px-3 py-2 text-right">{label[m]}</th>)}</tr></thead>
        <tbody>{months.map((mo) => <tr key={mo} className="border-b border-line last:border-0"><td className="px-3 py-2">{MONTH(mo)}</td>{metrics.map((m) => <td key={m} className="px-3 py-2 text-right tabular-nums">{compact(at(mo, m))}</td>)}</tr>)}</tbody>
      </table>
    </div>
  );
}

function AuditList({ items }) {
  return (
    <Rows items={items} render={(e) => (
      <div className="min-w-0">
        <p className="truncate"><span className="font-mono text-[12px]">{e.action}</span>{e.resource && <span className="text-muted"> · {e.resource}</span>}</p>
        <p className="text-[12px] text-subtle">{timeAgo(e.created_at)}{e.actor ? ` · by ${e.actor}` : ""}{e.org ? ` · ${e.org}` : ""}</p>
      </div>
    )} />
  );
}

function KeyList({ keys }) {
  return (
    <Rows items={keys} render={(k) => (
      <div className="flex items-center gap-2">
        <span className="font-medium truncate flex-1">{k.name}</span>
        <span className="font-mono text-[12px] text-subtle">{k.prefix}…</span>
        <span className={`text-[12px] ${k.revoked_at ? "text-danger" : "text-subtle"}`}>{k.revoked_at ? "revoked" : k.last_used_at ? `used ${timeAgo(k.last_used_at)}` : "never used"}</span>
      </div>
    )} />
  );
}

// Plan from Memro Cloud: the tier, and giving or ending a complimentary one.
function PlanSection({ apiBase, auth, notify, subject }) {
  const [view, setView] = useState(undefined); // undefined loading, null no billing
  const [draft, setDraft] = useState({ plan: "pro", months: 1, note: "" });
  const [busy, setBusy] = useState(false);
  const load = useCallback(() => {
    billing(apiBase, auth, `/billing/admin/accounts?subjects=${encodeURIComponent(subject)}`)
      .then((v) => setView(v ? v[0] : null)).catch((err) => { notify("Couldn't load the plan", err.message, true); setView(null); });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken, subject]);
  useEffect(() => { load(); }, [load]);
  const comp = async (months) => {
    setBusy(true);
    try {
      await api(apiBase, "/billing/admin/comp", { method: "POST", auth, json: { subject, plan: draft.plan, months, note: draft.note } });
      notify(months ? `Gave ${draft.plan} for ${months} month${months > 1 ? "s" : ""}` : "Complimentary plan ended", draft.note);
      load();
    } catch (err) { notify("Couldn't change the plan", err.message, true); }
    finally { setBusy(false); }
  };

  if (view === undefined) return <Section title="Plan"><Skeleton className="h-4 w-40" /></Section>;
  if (view === null) return <Section title="Plan" empty="Billing isn't running on this server (it comes with Memro Cloud)." />;
  return (
    <Section title="Plan">
      <div className="border border-line rounded-lg p-3 space-y-3 text-[13px]">
        <p>
          <span className="font-medium capitalize">{view.plan}</span>
          <span className="text-muted"> · {view.has_subscription ? "paid through Dodo" : view.comp ? `complimentary (${view.comp})` : view.status === "prepaid" ? "prepaid with x402" : "never paid"}</span>
          {view.paid_through && <span className="text-muted"> · until {new Date(view.paid_through).toLocaleDateString()}</span>}
        </p>
        {view.has_subscription ? (
          <p className="text-[12px] text-subtle">Change or cancel a Dodo subscription in Dodo.</p>
        ) : (
          <div className="flex flex-wrap items-end gap-2">
            <select className="input h-8 w-auto" value={draft.plan} onChange={(e) => setDraft({ ...draft, plan: e.target.value })} aria-label="Tier">
              {TIERS.map((t) => <option key={t} value={t}>{t}</option>)}
            </select>
            <select className="input h-8 w-auto" value={draft.months} onChange={(e) => setDraft({ ...draft, months: Number(e.target.value) })} aria-label="Months">
              {[1, 2, 3, 6, 12].map((m) => <option key={m} value={m}>{m} month{m > 1 ? "s" : ""}</option>)}
            </select>
            <input className="input h-8 flex-1 min-w-[140px]" value={draft.note} onChange={(e) => setDraft({ ...draft, note: e.target.value })} placeholder="Why (e.g. design partner)" />
            <button className="btn-secondary h-8" disabled={busy} onClick={() => comp(draft.months)}>{view.comp && view.plan === draft.plan ? "Extend" : "Give for free"}</button>
            {view.comp && <button className="btn-ghost h-8" disabled={busy} onClick={() => comp(0)}>End it</button>}
          </div>
        )}
      </div>
    </Section>
  );
}

// Account actions with a confirmation; delete needs the name typed.
function Actions({ apiBase, auth, notify, base, suspended, confirmText, kind, signOut, onChanged, onDeleted }) {
  const [dialog, setDialog] = useState(null); // "suspend" | "delete"
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const run = async (action, json, done) => {
    setBusy(true);
    try {
      const r = await api(apiBase, action === "delete" ? base : `${base}/${action}`, { method: action === "delete" ? "DELETE" : "POST", auth, json });
      notify(done(r || {}));
      setDialog(null); setText("");
      action === "delete" ? onDeleted() : onChanged();
    } catch (err) { notify("Couldn't do that", err.message, true); }
    finally { setBusy(false); }
  };
  return (
    <>
      <div className="flex flex-wrap gap-2">
        {suspended
          ? <button className="btn-secondary h-8" disabled={busy} onClick={() => run("unsuspend", undefined, () => "Suspension lifted")}>Unsuspend</button>
          : <button className="btn-secondary h-8" disabled={busy} onClick={() => setDialog("suspend")}>Suspend</button>}
        {signOut && <button className="btn-secondary h-8" disabled={busy} onClick={() => run("sign-out", undefined, (r) => `Signed out of ${r.sessions} session${r.sessions === 1 ? "" : "s"}`)}>Sign out everywhere</button>}
        <button className="btn-secondary h-8" disabled={busy} onClick={() => run("revoke-keys", undefined, (r) => `Revoked ${r.keys} key${r.keys === 1 ? "" : "s"}`)}>Revoke all keys</button>
        <button className="btn-danger h-8" disabled={busy} onClick={() => setDialog("delete")}>Delete…</button>
      </div>
      <Modal open={dialog === "suspend"} title={`Suspend this ${kind}?`} onClose={() => setDialog(null)}>
        <p className="text-[13px] text-muted">Every request it makes is refused until you lift it: sign-in, dashboard, API keys{kind === "organisation" ? " and server keys" : ""}. Nothing is deleted.</p>
        <Field label="Reason" hint="Shown to them in the error message."><input autoFocus className="input" value={text} onChange={(e) => setText(e.target.value)} placeholder="e.g. unpaid invoice" /></Field>
        <div className="flex justify-end gap-2 mt-4">
          <button className="btn-ghost" onClick={() => setDialog(null)}>Cancel</button>
          <button className="btn-danger" disabled={busy} onClick={() => run("suspend", { reason: text.trim() }, () => "Suspended")}>Suspend</button>
        </div>
      </Modal>
      <Modal open={dialog === "delete"} title={`Delete this ${kind}?`} onClose={() => setDialog(null)}>
        <p className="text-[13px] text-muted">This permanently deletes {kind === "organisation" ? "the organisation, its members' access, keys, and every one of its users' memories and files" : "the person, their sessions, and their own agents' memories and files"}. It can't be undone.</p>
        <Field label={`Type ${confirmText} to confirm`}><input autoFocus className="input" value={text} onChange={(e) => setText(e.target.value)} /></Field>
        <div className="flex justify-end gap-2 mt-4">
          <button className="btn-ghost" onClick={() => setDialog(null)}>Cancel</button>
          <button className="btn-danger" disabled={busy || text.trim() !== confirmText} onClick={() => run("delete", { confirm: text.trim() }, () => "Deleted")}>Delete permanently</button>
        </div>
      </Modal>
    </>
  );
}

function UserDrawer({ apiBase, auth, notify, id, me, onClose, onChanged }) {
  const [d, setD] = useState(null);
  const load = useCallback(() => {
    api(apiBase, `/admin/users/${id}`, { auth }).then(setD).catch((err) => { notify("Couldn't load the person", err.message, true); onClose(); });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken, id]);
  useEffect(() => { load(); }, [load]);
  return (
    <Drawer open title={d ? d.email : "Loading…"} onClose={onClose}>
      {!d ? <Skeleton className="h-4 w-1/2" /> : (
        <div className="space-y-6">
          <div className="text-[13px] space-y-1">
            <p>{d.staff && <span className="mr-2 text-[11px] px-1.5 py-0.5 rounded-md border border-line text-muted">staff</span>}
              {d.suspended_at && <span className="mr-2 text-[11px] px-1.5 py-0.5 rounded-md border border-danger text-danger">suspended{d.suspended_reason ? `: ${d.suspended_reason}` : ""}</span>}
              joined {timeAgo(d.created_at)} · {d.last_sign_in ? `signed in ${timeAgo(d.last_sign_in)}` : "never signed in"}</p>
            <p className="text-muted">{num(d.agents)} agents · {num(d.memories)} memories · {compact(d.stored_tokens)} stored tokens</p>
          </div>
          {id !== me && !d.staff && (
            <Actions apiBase={apiBase} auth={auth} notify={notify} base={`/admin/users/${id}`} kind="person" suspended={!!d.suspended_at}
              confirmText={d.email} signOut onChanged={() => { load(); onChanged(); }} onDeleted={() => { onChanged(); onClose(); }} />
          )}
          {d.staff && id !== me && <p className="text-[12px] text-subtle">Remove their staff rights to suspend or delete them.</p>}
          <PlanSection apiBase={apiBase} auth={auth} notify={notify} subject={`user:${id}`} />
          <Section title="Organisations">{d.orgs.length ? <Rows items={d.orgs} render={(o) => <p>{o.name} <span className="text-subtle">· {o.role}</span></p>} /> : null}</Section>
          <Section title="Agents">{d.agent_list.length ? <Rows items={d.agent_list} render={(a) => (
            <div className="flex gap-2"><span className="font-mono text-[12px] truncate flex-1" title={a.id}>{a.id.slice(0, 16)}…</span><span className="text-subtle">{num(a.memories)} memories · {compact(a.stored_tokens)} tokens</span></div>
          )} /> : null}</Section>
          <Section title="API keys">{d.keys.length ? <KeyList keys={d.keys} /> : null}</Section>
          <Section title="Recent sign-ins">{d.sign_ins.length ? <Rows items={d.sign_ins} render={(t) => <p>{new Date(t).toLocaleString()}</p>} /> : null}</Section>
          <Section title="Usage by month">{d.usage.length ? <UsageTable usage={d.usage} /> : null}</Section>
          <Section title="Audit log">{d.audit.length ? <AuditList items={d.audit} /> : null}</Section>
        </div>
      )}
    </Drawer>
  );
}

function OrgDrawer({ apiBase, auth, notify, id, onClose, onChanged, onManage }) {
  const [d, setD] = useState(null);
  const load = useCallback(() => {
    api(apiBase, `/admin/tenants/${id}`, { auth }).then(setD).catch((err) => { notify("Couldn't load the organisation", err.message, true); onClose(); });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken, id]);
  useEffect(() => { load(); }, [load]);
  return (
    <Drawer open title={d ? d.name : "Loading…"} onClose={onClose}>
      {!d ? <Skeleton className="h-4 w-1/2" /> : (
        <div className="space-y-6">
          <div className="text-[13px] space-y-1">
            <p>{d.suspended_at && <span className="mr-2 text-[11px] px-1.5 py-0.5 rounded-md border border-danger text-danger">suspended{d.suspended_reason ? `: ${d.suspended_reason}` : ""}</span>}
              {d.self_serve ? "self-service" : "created by staff"} · {timeAgo(d.created_at)}</p>
            <p className="text-muted">{num(d.end_users)} users · {num(d.members)} members · {compact(d.stored_tokens)} stored tokens · {compact(d.queries_this_month)} queries this month</p>
          </div>
          <Actions apiBase={apiBase} auth={auth} notify={notify} base={`/admin/tenants/${id}`} kind="organisation" suspended={!!d.suspended_at}
            confirmText={d.name} onChanged={() => { load(); onChanged(); }} onDeleted={() => { onChanged(); onClose(); }} />
          <Section title="Contract or trial">
            <div className="border border-line rounded-lg p-3 text-[13px] flex items-center gap-3">
              <p className="flex-1">{d.limits ? <><span className="font-medium capitalize">{d.limits.plan}</span>{d.limits.trial_ends_at && <span className="text-muted"> · trial ends {new Date(d.limits.trial_ends_at).toLocaleDateString()}</span>}</> : <span className="text-muted">None: it uses the tier below</span>}</p>
              <button className="btn-secondary h-8" onClick={() => onManage(d)}>Manage</button>
            </div>
          </Section>
          <PlanSection apiBase={apiBase} auth={auth} notify={notify} subject={id} />
          <Section title="Members">{d.member_list.length ? <Rows items={d.member_list} render={(m) => <p>{m.email} <span className="text-subtle">· {m.role}</span></p>} /> : null}</Section>
          <Section title="Pending invitations">{d.invites.length ? <Rows items={d.invites} render={(i) => <p>{i.email} <span className="text-subtle">· {i.role} · expires {timeAgo(i.expires_at)}</span></p>} /> : null}</Section>
          <Section title="Domains">{d.domain_claims.length ? <Rows items={d.domain_claims} render={(x) => <p>{x.domain} <span className="text-subtle">· {x.verified_at ? "verified" : "not verified"}</span></p>} /> : null}</Section>
          <Section title="Keys">{d.keys.length ? <KeyList keys={d.keys} /> : null}</Section>
          <Section title="Usage by month">{d.usage.length ? <UsageTable usage={d.usage} /> : null}</Section>
          <Section title="Audit log">{d.audit.length ? <AuditList items={d.audit} /> : null}</Section>
        </div>
      )}
    </Drawer>
  );
}

// One series of daily counts as bars, with a hover readout and its numbers
// as a table for screen readers.
function DayBars({ title, days }) {
  const [hover, setHover] = useState(null);
  const max = Math.max(1, ...days.map((d) => d.count));
  const w = 600, h = 120, gap = 2, bw = w / days.length - gap;
  const label = (d) => new Date(d.day).toLocaleDateString(undefined, { day: "numeric", month: "short", timeZone: "UTC" });
  const shown = hover ?? days[days.length - 1];
  return (
    <div className="card p-5">
      <div className="flex items-baseline justify-between">
        <p className="text-[13px] text-muted">{title}</p>
        <p className="text-[12.5px] text-muted tabular-nums">{shown ? `${label(shown)}: ${num(shown.count)}` : ""}</p>
      </div>
      <p className="text-[24px] font-semibold tracking-[-0.03em] tabular mt-1">{num(days.reduce((a, d) => a + d.count, 0))} <span className="text-[12.5px] font-normal text-muted tracking-normal">in 30 days</span></p>
      <svg viewBox={`0 0 ${w} ${h}`} className="w-full h-[120px] mt-3" role="img" aria-label={`${title}, last 30 days`} onMouseLeave={() => setHover(null)}>
        <line x1="0" x2={w} y1={h - 0.5} y2={h - 0.5} stroke="var(--line)" />
        {days.map((d, i) => {
          const bh = d.count ? Math.max(4, (d.count / max) * (h - 8)) : 0;
          const x = i * (bw + gap);
          return (
            <g key={d.day} onMouseEnter={() => setHover(d)}>
              <rect x={x} y="0" width={bw + gap} height={h} fill="transparent" />
              {bh > 0 && <path d={`M${x},${h} v${-(bh - 4)} q0,-4 4,-4 h${bw - 8} q4,0 4,4 v${bh - 4} z`} fill="var(--accent)" opacity={hover && hover !== d ? 0.45 : 1} />}
            </g>
          );
        })}
      </svg>
      <table className="sr-only"><caption>{title}</caption><tbody>{days.map((d) => <tr key={d.day}><th>{label(d)}</th><td>{d.count}</td></tr>)}</tbody></table>
    </div>
  );
}

const FEEDS = [
  { id: "", label: "Everything" },
  { id: "admin.", label: "Staff actions" },
  { id: "billing.", label: "Plans given" },
  { id: "tenant_limits.", label: "Contracts & trials" },
  { id: "staff.", label: "Staff rights" },
  { id: "org.", label: "Organisations" },
];

function Activity({ apiBase, auth, notify }) {
  const [a, setA] = useState(null);
  const [feed, setFeed] = useState("");
  useEffect(() => {
    setA(null);
    api(apiBase, `/admin/activity?action=${encodeURIComponent(feed)}`, { auth }).then(setA).catch((err) => { notify("Couldn't load activity", err.message, true); setA({ sign_ups: [], organisations: [], queries: [], feed: [] }); });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken, feed]);
  if (!a) return <div className="card p-5"><Skeleton className="h-4 w-1/2" /></div>;
  return (
    <div className="space-y-6">
      <PageHeader title="Activity" description="What's been happening across Memro." />
      <div className="grid lg:grid-cols-2 gap-4">
        <DayBars title="Sign-ups per day" days={a.sign_ups} />
        <DayBars title="New organisations per day" days={a.organisations} />
      </div>
      {a.queries.length > 0 && (
        <Panel title="Queries per month" bodyClass="p-0">
          <table className="w-full text-[13px]"><tbody>{a.queries.map((q) => <tr key={q.month} className="border-b border-line last:border-0"><td className="px-5 py-2.5">{MONTH(q.month)}</td><td className="px-5 py-2.5 text-right tabular-nums">{num(q.value)}</td></tr>)}</tbody></table>
        </Panel>
      )}
      <Panel title="Feed" description="The newest 100 events from the audit log."
        action={<select className="input h-8 w-auto" value={feed} onChange={(e) => setFeed(e.target.value)} aria-label="Filter the feed">{FEEDS.map((f) => <option key={f.id} value={f.id}>{f.label}</option>)}</select>}>
        {a.feed.length ? <AuditList items={a.feed} /> : <p className="text-[13px] text-subtle">Nothing yet.</p>}
      </Panel>
    </div>
  );
}

function Revenue({ apiBase, auth, notify }) {
  const [r, setR] = useState(undefined);
  useEffect(() => {
    billing(apiBase, auth, "/billing/admin/revenue").then(setR).catch((err) => { notify("Couldn't load revenue", err.message, true); setR(null); });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken]);
  if (r === undefined) return null;
  if (r === null) return <Panel title="Revenue"><p className="text-[13px] text-subtle">Billing isn't running on this server (it comes with Memro Cloud).</p></Panel>;
  return (
    <Panel title="Revenue" description="Monthly recurring revenue from tiers paid through Dodo; contracts at Enterprise list price.">
      <div className="grid sm:grid-cols-3 gap-4 text-[13px]">
        <div><p className="text-muted">Dodo MRR</p><p className="text-[24px] font-semibold tabular">{usd0(r.mrr_cents)}</p>
          <p className="text-subtle">{(r.paying || []).map((p) => `${num(p.accounts)} ${p.plan}`).join(" · ") || "no paying accounts"}</p></div>
        <div><p className="text-muted">Contracts</p><p className="text-[24px] font-semibold tabular">{usd0(r.contract_mrr_cents)}</p><p className="text-subtle">{num(r.contracts)} on Enterprise or custom</p></div>
        <div><p className="text-muted">Not paying</p><p className="text-[24px] font-semibold tabular">{num(r.comped + r.prepaid)}</p><p className="text-subtle">{num(r.comped)} complimentary · {num(r.prepaid)} prepaid (x402) · {num(r.on_hold)} payment failing</p></div>
      </div>
    </Panel>
  );
}

const usd0 = (cents) => `$${Math.round((cents || 0) / 100).toLocaleString()}`;
