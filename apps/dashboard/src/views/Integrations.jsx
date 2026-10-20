import { useCallback, useEffect, useMemo, useState } from "react";
import { Panel, Icon, PageHeader, Modal, Drawer, Empty, Skeleton, timeAgo } from "../components/ui";
import { api } from "../lib/api";

const SlackMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={className} aria-hidden="true">
    <rect x="9.5" y="2" width="3.5" height="9" rx="1.75" fill="#36C5F0" />
    <rect x="2" y="11" width="9" height="3.5" rx="1.75" fill="#2EB67D" transform="translate(0 -1.5)" />
    <rect x="11" y="13" width="3.5" height="9" rx="1.75" fill="#ECB22E" />
    <rect x="13" y="9.5" width="9" height="3.5" rx="1.75" fill="#E01E5A" />
  </svg>
);

const GitHubMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 16 16" className={`${className} text-ink`} aria-hidden="true" fill="currentColor">
    <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
  </svg>
);

const LinearMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-[#5E6AD2]`} aria-hidden="true" fill="currentColor">
    <path d="M2.5 12c0-5.247 4.253-9.5 9.5-9.5 1.528 0 2.97.36 4.25 1.002L4.002 15.75A9.458 9.458 0 0 1 2.5 12zm1.25 5.25L17.25 3.75A9.458 9.458 0 0 1 21.5 12c0 5.247-4.253 9.5-9.5 9.5a9.458 9.458 0 0 1-8.25-4.25z" />
  </svg>
);

const SentryMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-[#FF4961]`} aria-hidden="true" fill="currentColor">
    <path d="M12.986 2.014a1.2 1.2 0 0 0-1.972 0l-9.8 14.7a1.2 1.2 0 0 0 .986 1.857h3.6a.6.6 0 0 0 .5-.267L12 9.47l5.686 8.834a.6.6 0 0 0 .5.267h3.6a1.2 1.2 0 0 0 .986-1.857l-9.8-14.7z" />
  </svg>
);

const NotionMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-ink`} aria-hidden="true" fill="currentColor">
    <path d="M4.459 4.208c.746.606 1.026.56 2.428.466l11.458-.84c1.12-.093 1.213-.42 1.4-.84l.653-1.4c.094-.187.28-.28.467-.28h1.68c.28 0 .466.186.466.466v17.432c0 .373-.186.653-.56.746l-4.106 1.867c-.28.093-.56 0-.746-.28l-8.587-12.787v11.201c0 1.213.653 1.587 2.053 1.68l.467.093c.28 0 .467.187.467.467v1.12c0 .28-.187.467-.467.467H4.46c-.28 0-.467-.187-.467-.467v-1.12c0-.28.187-.467.467-.467l.56-.093c1.4-.093 1.867-.653 1.867-1.773V7.288c0-1.027-.373-1.493-1.68-1.587l-.746-.093c-.28 0-.467-.187-.467-.467v-1.12c0-.28.187-.467.467-.467h1.493l-.467.653zm11.2 12.321V6.635L9.693 15.688l5.966.841z" />
  </svg>
);

const JiraMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-[#0052CC]`} aria-hidden="true" fill="currentColor">
    <path d="M11.53 2c0 5.26 4.27 9.53 9.53 9.53v.94c-5.26 0-9.53 4.27-9.53 9.53h-.94c0-5.26-4.27-9.53-9.53-9.53v-.94c5.26 0 9.53-4.27 9.53-9.53h.94z" />
  </svg>
);

const GitLabMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-[#FC6D26]`} aria-hidden="true" fill="currentColor">
    <path d="m23.6 9.59-1.01-3.13a.9.9 0 0 0-1.72 0l-1.01 3.13H4.14L3.13 6.46a.9.9 0 0 0-1.72 0L.4 9.59a1.8 1.8 0 0 0 .65 2.01L12 20.33l10.95-8.73a1.8 1.8 0 0 0 .65-2.01z" />
  </svg>
);

const GoogleMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={className} aria-hidden="true">
    <path d="M7.71 3.5 1.15 15l3.43 6 6.55-11.5z" fill="#0066DA" />
    <path d="m14.29 3.5-6.58 11.5 3.43 6 6.58-11.5z" fill="#00AC47" />
    <path d="m22.85 15-3.43-6H6.57L10 15z" fill="#EA4335" />
    <path d="m10 15 3.43 6h6.86l3.43-6z" fill="#FFBA00" />
  </svg>
);

const DiscordMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-[#5865F2]`} aria-hidden="true" fill="currentColor">
    <path d="M20.317 4.37a19.791 19.791 0 0 0-4.885-1.515.074.074 0 0 0-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 0 0-5.487 0 12.64 12.64 0 0 0-.617-1.25.077.077 0 0 0-.079-.037A19.736 19.736 0 0 0 3.677 4.37a.07.07 0 0 0-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 0 0 .031.057 19.9 19.9 0 0 0 5.993 3.03.078.078 0 0 0 .084-.028c.462-.63.874-1.295 1.226-1.994.021-.041.001-.09-.041-.106a13.107 13.107 0 0 1-1.872-.892.077.077 0 0 1-.008-.128 10.2 10.2 0 0 0 .372-.292.074.074 0 0 1 .077-.01c3.929 1.793 8.18 1.793 12.061 0a.074.074 0 0 1 .078.01c.12.098.246.198.373.292a.077.077 0 0 1-.006.127 12.299 12.299 0 0 1-1.873.894.077.077 0 0 0-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 0 0 .084.028 19.839 19.839 0 0 0 6.002-3.03.077.077 0 0 0 .032-.054c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 0 0-.031-.028zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.956-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.955-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.946 2.418-2.157 2.418z" />
  </svg>
);

const DCEMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-[#10B981]`} aria-hidden="true" fill="currentColor">
    <path d="M12 2 2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
  </svg>
);

const PROVIDERS = {
  slack: {
    name: "Slack", Mark: SlackMark, unit: "channel",
    blurb: "Conversations from the channels you choose, kept up to date as people post, edit and delete.",
    privacy: "Memro only reads channels you turn on. Private channels appear only if you're a member and have invited the Memro app.",
  },
  github: {
    name: "GitHub", Mark: GitHubMark, unit: "repository",
    blurb: "Issues and pull requests with their discussion, from the repositories you share with the Memro app.",
    privacy: "Read-only access. You choose the repositories on GitHub, and Memro only syncs those your account can read.",
  },
  linear: {
    name: "Linear", Mark: LinearMark, unit: "team",
    blurb: "Issues, initiatives, priorities, and discussion from the teams you choose.",
    privacy: "Read-only access. Memro only syncs issues and comments from the teams you turn on.",
  },
  jira: {
    name: "Jira", Mark: JiraMark, unit: "project",
    blurb: "Epics, stories, bugs, comments, and sprint changes from your Atlassian Cloud workspace.",
    privacy: "Read-only access via Atlassian 3LO. Memro only reads projects and issues you authorize.",
  },
  sentry: {
    name: "Sentry", Mark: SentryMark, unit: "project",
    blurb: "Production issues, exceptions, stack traces, and culprit files from the projects you choose.",
    privacy: "Read-only access. Memro only syncs issues and stack traces from the projects you turn on.",
  },
  notion: {
    name: "Notion", Mark: NotionMark, unit: "database",
    blurb: "Product specs, architectural decisions, RFCs, and engineering knowledge bases.",
    privacy: "Read-only access. Memro only syncs databases and pages you explicitly share with the integration.",
  },
  gitlab: {
    name: "GitLab", Mark: GitLabMark, unit: "project",
    blurb: "Merge requests, issues, commits, and pipeline logs from your GitLab repositories.",
    privacy: "Read-only access. Memro only syncs projects and branches you select.",
  },
  google: {
    name: "Google Drive", Mark: GoogleMark, unit: "folder",
    blurb: "Design docs, RFCs, meeting notes, and spreadsheets from Google Docs and Drive.",
    privacy: "Read-only access. Memro only indexes files and folders you explicitly grant.",
  },
  discord: {
    name: "Discord", Mark: DiscordMark, unit: "channel",
    blurb: "Developer channels, incident threads, and community discussions from your Discord guild.",
    privacy: "Memro only reads messages in text channels you permit the bot to join.",
  },
  dce: {
    name: "Custom (DCE)", Mark: DCEMark, unit: "connector",
    blurb: "Connect any internal or 3rd-party REST API via declarative YAML manifest.",
    privacy: "Declarative runtime with client-managed credentials and local policy execution.",
  },
};

const plural = (unit, n) => (n === 1 ? unit : unit.endsWith("y") ? `${unit.slice(0, -1)}ies` : `${unit}s`);

const STATUS = {
  active: { label: "Active", dot: "bg-ok" },
  paused: { label: "Paused", dot: "bg-warn" },
  error: { label: "Needs attention", dot: "bg-danger" },
  revoked: { label: "Disconnected", dot: "bg-danger" },
};

function Status({ conn }) {
  const syncing = conn.status === "active" && conn.pending_jobs > 0;
  const s = STATUS[conn.status] || STATUS.error;
  return (
    <span className="inline-flex items-center gap-1.5 h-6 px-2.5 rounded-full border border-line bg-surface text-[12px] whitespace-nowrap">
      {syncing ? <span className="size-2.5 rounded-full border-2 border-accent/30 border-t-accent animate-spin" /> : <span className={`size-2 rounded-full ${s.dot}`} />}
      {syncing ? "Syncing" : s.label}
    </span>
  );
}

/** Clears the OAuth result from the address bar once it has been shown. */
const readOAuthResult = () => {
  if (!window.location.pathname.startsWith("/integrations")) return null;
  const q = new URLSearchParams(window.location.search);
  if (!q.get("provider")) return null;
  const res = { provider: q.get("provider"), connected: q.get("connected") === "1", error: q.get("error") };
  window.history.replaceState(null, "", "/integrations");
  return res;
};

export default function Integrations({ apiBase, auth, notify, onOpenMemories }) {
  const [data, setData] = useState(null);
  const [busy, setBusy] = useState(null);
  const [channelsFor, setChannelsFor] = useState(null);
  const [removing, setRemoving] = useState(null);

  const load = useCallback(async () => {
    try {
      setData(await api(apiBase, "/connectors", { auth }));
    } catch (err) {
      notify("Couldn't load integrations", err.message, true);
      setData((d) => d || { providers: [], connections: [] });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, auth.sessionToken]);

  useEffect(() => {
    const res = readOAuthResult();
    if (res) {
      const name = PROVIDERS[res.provider]?.name || res.provider;
      if (res.connected) notify(`${name} connected`, res.provider === "slack" ? "Choose the channels to sync." : "Syncing your repositories now.");
      else notify(`Couldn't connect ${name}`, res.error || "Please try again.", true);
    }
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load]);

  // Refresh while anything is syncing.
  const syncing = data?.connections?.some((c) => c.status === "active" && c.pending_jobs > 0);
  useEffect(() => {
    if (!syncing) return;
    const id = setInterval(load, 4000);
    return () => clearInterval(id);
  }, [syncing, load]);

  const connect = async (provider) => {
    setBusy(`connect-${provider}`);
    try {
      const { url } = await api(apiBase, `/connectors/${provider}/authorize`, { method: "POST", auth });
      window.location.assign(url);
    } catch (err) {
      notify(`Couldn't start ${PROVIDERS[provider]?.name || provider} sign-in`, err.message, true);
      setBusy(null);
    }
  };

  const syncNow = async (conn) => {
    setBusy(`sync-${conn.id}`);
    try {
      await api(apiBase, `/connectors/${conn.id}/sync`, { method: "POST", auth });
      notify("Sync started", conn.account_name);
      load();
    } catch (err) {
      notify("Couldn't sync", err.message, true);
    } finally {
      setBusy(null);
    }
  };

  const enabled = useMemo(() => Object.fromEntries((data?.providers || []).map((p) => [p.name, p.enabled])), [data]);
  const anyEnabled = Object.values(enabled).some(Boolean);

  return (
    <div className="space-y-6 max-w-4xl">
      <PageHeader title="Integrations" description="Bring knowledge from the tools your team already uses into memory, so every connected agent can recall it." />

      <div className="grid sm:grid-cols-2 gap-4">
        {Object.entries(PROVIDERS).map(([id, p]) => {
          const on = enabled[id];
          return (
            <div key={id} className="card p-5 flex flex-col">
              <div className="flex items-center gap-3">
                <div className="size-10 rounded-xl border border-line bg-surface flex items-center justify-center shadow-card"><p.Mark /></div>
                <div className="min-w-0">
                  <p className="text-[14.5px] font-semibold">{p.name}</p>
                  <p className="text-[12px] text-subtle">{data ? (on ? "Available" : "Not configured on this server") : "…"}</p>
                </div>
              </div>
              <p className="text-[13px] text-muted mt-3 flex-1">{p.blurb}</p>
              <p className="text-[12px] text-subtle mt-2 flex gap-1.5"><Icon name="lock" className="!text-[14px] mt-px" />{p.privacy}</p>
              <div className="mt-4">
                <button className="btn-primary" disabled={!on || busy === `connect-${id}`} onClick={() => connect(id)}>
                  <Icon name="add_link" className="!text-[18px]" />
                  {busy === `connect-${id}` ? "Opening…" : (data?.connections || []).some((c) => c.provider === id) ? `Connect another` : `Connect ${p.name}`}
                </button>
              </div>
            </div>
          );
        })}
      </div>

      <Panel title={`Connections${data ? ` · ${data.connections.length}` : ""}`} bodyClass="p-0">
        {data === null ? (
          <div className="p-5 space-y-3"><Skeleton className="h-4 w-1/2" /><Skeleton className="h-4 w-1/3" /></div>
        ) : data.connections.length === 0 ? (
          <Empty icon="hub" title="Nothing connected yet">
            {anyEnabled ? "Connect Slack or GitHub above to start syncing." : "Ask your Memro administrator to configure the Slack or GitHub connector."}
          </Empty>
        ) : (
          <ul>
            {data.connections.map((c) => {
              const p = PROVIDERS[c.provider] || { name: c.provider, Mark: () => <Icon name="hub" />, unit: "source" };
              const needsChannels = c.provider === "slack" && c.status === "active" && c.sources_enabled === 0;
              return (
                <li key={c.id} className="px-5 py-4 border-b border-line last:border-0">
                  <div className="flex items-start gap-4">
                    <div className="size-9 rounded-lg border border-line bg-surface flex items-center justify-center shrink-0 mt-0.5"><p.Mark className="size-5" /></div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2 flex-wrap">
                        <p className="text-[14px] font-medium truncate">{c.account_name || c.account_id}</p>
                        <Status conn={c} />
                      </div>
                      <p className="text-[12.5px] text-subtle mt-1">
                        {p.name} · as {c.external_user_name || c.external_user_id} · {c.sources_enabled} of {c.sources_total} {plural(p.unit, c.sources_total)} · {" "}
                        <button className="underline decoration-line hover:text-ink" onClick={onOpenMemories}>{c.memories.toLocaleString()} {c.memories === 1 ? "memory" : "memories"}</button>
                        {" "}· synced {timeAgo(c.last_synced_at)}
                      </p>
                      {c.status_detail && (
                        <p className={`text-[12.5px] mt-2 ${c.status === "paused" ? "text-warn" : "text-danger"}`}>{c.status_detail}</p>
                      )}
                      {needsChannels && (
                        <p className="text-[12.5px] mt-2 text-accent">Choose which channels to sync — nothing is read until you do.</p>
                      )}
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0 flex-wrap justify-end">
                      {c.provider === "slack" && c.status !== "revoked" && (
                        <button className={needsChannels ? "btn-primary" : "btn-secondary"} onClick={() => setChannelsFor(c)}>Channels</button>
                      )}
                      {c.manage_url && c.status !== "revoked" && (
                        <a className="btn-secondary" href={c.manage_url} target="_blank" rel="noopener noreferrer">
                          Repositories <Icon name="arrow_outward" className="!text-[15px]" />
                        </a>
                      )}
                      {c.status === "revoked" ? (
                        <button className="btn-primary" onClick={() => connect(c.provider)} disabled={!enabled[c.provider]}>Reconnect</button>
                      ) : (
                        <button className="btn-ghost" onClick={() => syncNow(c)} disabled={busy === `sync-${c.id}`}
                          title={c.status === "paused" ? "Resume and sync" : "Sync now"}>
                          <Icon name="sync" className="!text-[18px]" />{c.status === "paused" || c.status === "error" ? "Resume" : ""}
                        </button>
                      )}
                      <button className="btn-ghost text-danger" onClick={() => setRemoving(c)}>Disconnect</button>
                    </div>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </Panel>

      <ChannelPicker apiBase={apiBase} auth={auth} notify={notify} conn={channelsFor} onClose={() => setChannelsFor(null)} onSaved={load} />
      <DisconnectModal apiBase={apiBase} auth={auth} notify={notify} conn={removing} onClose={() => setRemoving(null)} onDone={load} />
    </div>
  );
}

function ChannelPicker({ apiBase, auth, notify, conn, onClose, onSaved }) {
  const [sources, setSources] = useState(null);
  const [chosen, setChosen] = useState({});
  const [filter, setFilter] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!conn) return;
    setSources(null); setFilter("");
    api(apiBase, `/connectors/${conn.id}/sources`, { auth })
      .then((s) => { setSources(s); setChosen(Object.fromEntries(s.map((x) => [x.id, x.enabled]))); })
      .catch((err) => { notify("Couldn't load channels", err.message, true); onClose(); });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [conn?.id]);

  const shown = (sources || []).filter((s) => s.name.toLowerCase().includes(filter.trim().toLowerCase().replace(/^#/, "")));
  const changes = Object.fromEntries((sources || []).filter((s) => !!chosen[s.id] !== s.enabled).map((s) => [s.id, !!chosen[s.id]]));
  const changed = Object.keys(changes).length;

  const save = async () => {
    setSaving(true);
    try {
      await api(apiBase, `/connectors/${conn.id}/sources`, { method: "PUT", auth, json: { enabled: changes } });
      const on = Object.values(changes).filter(Boolean).length;
      notify("Channels updated", on ? `Syncing ${on} new ${on === 1 ? "channel" : "channels"}.` : "Syncing stopped for the channels you turned off.");
      onSaved();
      onClose();
    } catch (err) {
      notify("Couldn't update channels", err.message, true);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Drawer open={!!conn} title={`Channels · ${conn?.account_name || ""}`} onClose={onClose}
      footer={<>
        <button className="btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn-primary" onClick={save} disabled={!changed || saving}>{saving ? "Saving…" : changed ? `Save ${changed} ${changed === 1 ? "change" : "changes"}` : "Save"}</button>
      </>}>
      <p className="text-[13px] text-muted">
        Messages from these channels become memories. Turning a channel off stops syncing; memories already made from it stay until you delete them.
        To add a private channel, invite the Memro app to it in Slack, then reopen this list.
      </p>
      <div className="relative mt-4">
        <Icon name="search" className="absolute left-3 top-1/2 -translate-y-1/2 text-subtle !text-[18px]" />
        <input className="input pl-9" placeholder="Filter channels" value={filter} onChange={(e) => setFilter(e.target.value)} aria-label="Filter channels" />
      </div>
      <ul className="mt-3 -mx-2">
        {sources === null ? (
          <li className="p-2 space-y-2"><Skeleton className="h-4 w-1/2" /><Skeleton className="h-4 w-1/3" /></li>
        ) : shown.length === 0 ? (
          <li className="px-2 py-6 text-[13px] text-subtle text-center">{sources.length ? "No channels match." : "No channels yet. If you just connected, give it a moment and reopen this list."}</li>
        ) : shown.map((s) => (
          <li key={s.id}>
            <label className="flex items-center gap-3 px-2 py-2 rounded-lg hover:bg-panel cursor-pointer">
              <input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={!!chosen[s.id]}
                onChange={(e) => setChosen((c) => ({ ...c, [s.id]: e.target.checked }))} />
              <Icon name={s.private ? "lock" : "tag"} className="!text-[16px] text-subtle" />
              <span className="text-[13.5px] flex-1 truncate">{s.name}</span>
              {s.enabled && <span className="text-[11.5px] text-subtle">{s.last_synced_at ? `synced ${timeAgo(s.last_synced_at)}` : "first sync…"}</span>}
            </label>
          </li>
        ))}
      </ul>
    </Drawer>
  );
}

function DisconnectModal({ apiBase, auth, notify, conn, onClose, onDone }) {
  const [purge, setPurge] = useState(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => { setPurge(false); }, [conn?.id]);

  const remove = async () => {
    setBusy(true);
    try {
      const res = await api(apiBase, `/connectors/${conn.id}${purge ? "?delete_memories=true" : ""}`, { method: "DELETE", auth });
      notify("Disconnected", purge ? `${conn.account_name} · ${res.deleted_memories} memories deleted` : conn.account_name);
      onClose();
      onDone();
    } catch (err) {
      notify("Couldn't disconnect", err.message, true);
    } finally {
      setBusy(false);
    }
  };

  const name = PROVIDERS[conn?.provider]?.name || conn?.provider;
  return (
    <Modal open={!!conn} title={`Disconnect ${conn?.account_name || ""}?`} onClose={onClose}>
      <p className="text-[13px] text-muted">
        Memro stops syncing from {name} and gives up its access.
        {conn?.provider === "github" && " To remove the app from your GitHub account entirely, uninstall it on GitHub too."}
      </p>
      {conn?.memories > 0 && (
        <label className="flex items-start gap-2.5 mt-4 p-3 rounded-xl border border-line cursor-pointer">
          <input type="checkbox" className="size-4 mt-0.5 accent-[var(--color-danger)]" checked={purge} onChange={(e) => setPurge(e.target.checked)} />
          <span className="text-[13px]">
            Also delete the {conn.memories.toLocaleString()} {conn.memories === 1 ? "memory" : "memories"} it created
            <span className="block text-[12px] text-subtle mt-0.5">Otherwise they stay in your memory and can be deleted individually.</span>
          </span>
        </label>
      )}
      <div className="flex justify-end gap-2 mt-5">
        <button className="btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn-danger" onClick={remove} disabled={busy}>{busy ? "Disconnecting…" : "Disconnect"}</button>
      </div>
    </Modal>
  );
}
