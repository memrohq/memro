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

const JiraMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-[#0052CC]`} aria-hidden="true" fill="currentColor">
    <path d="M11.53 2c0 5.26 4.27 9.53 9.53 9.53H22V2h-10.47zm0 10.47c-5.26 0-9.53 4.27-9.53 9.53H2v-9.53h9.53z" />
  </svg>
);

const SentryMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-[#362D59] dark:text-[#9B89E5]`} aria-hidden="true" fill="currentColor">
    <path d="M13.2 2.2c-.6-.3-1.4-.3-2 0L2.8 6.9c-.6.3-1 .9-1 1.6v9.4c0 .7.4 1.3 1 1.6l8.4 4.7c.6.3 1.4.3 2 0l8.4-4.7c.6-.3 1-.9 1-1.6V8.5c0-.7-.4-1.3-1-1.6L13.2 2.2zM12 4.1l7.1 4-7.1 4-7.1-4 7.1-4zm-8 6.1l7.1 4v7.7L4 17.9v-7.7zm9.1 11.7v-7.7l7.1-4v7.7l-7.1 4z" />
  </svg>
);

const NotionMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-ink`} aria-hidden="true" fill="currentColor">
    <path d="M4.459 4.208c.746.606 1.026.56 2.428.466l13.215-.793c.28 0 .047-.28-.046-.326L17.86 1.77c-.42-.326-.98-.747-2.054-.653L2.733 2.19c-.466.046-.56.326-.373.513l2.099 1.505zm.747 4.155v12.277c0 .746.373 1.026 1.213.98l14.288-.84c.84-.047 1.026-.513 1.026-1.166V7.476c0-.653-.28-.98-.933-.933l-14.66.887c-.654.047-.934.373-.934.933zm11.388 1.447c.093.42.093.84-.28.887l-.7.093v7.329c0 .793-.373 1.073-.933 1.12l-2.428.14c-.373.047-.607-.186-.607-.56v-2.333H9.827v2.427c0 .373-.187.56-.56.607l-2.52.14c-.374.047-.654-.187-.654-.56V11.49c0-.466.234-.653.607-.7l2.8-.233c.42-.047.653.187.653.56v2.193h2.8v-2.286c0-.42.234-.653.607-.7l2.8-.233c.373-.047.607.186.7.606z" />
  </svg>
);

const GitLabMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={className} aria-hidden="true">
    <path d="m23.6 9.6-1.5-4.5c-.2-.5-.8-.7-1.3-.4-.2.1-.4.3-.5.6l-2.3 7H5.7l-2.3-7c-.2-.5-.8-.7-1.3-.4-.2.1-.4.3-.5.6L.1 9.6c-.2.5 0 1.1.4 1.4L12 19.9l11.2-8.9c.4-.3.6-.9.4-1.4z" fill="#E24329" />
  </svg>
);

const DiscordMark = ({ className = "size-6" }) => (
  <svg viewBox="0 0 24 24" className={`${className} text-[#5865F2]`} aria-hidden="true" fill="currentColor">
    <path d="M20.317 4.37a19.791 19.791 0 0 0-4.885-1.515.074.074 0 0 0-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 0 0-5.487 0 12.64 12.64 0 0 0-.617-1.25.077.077 0 0 0-.079-.037A19.736 19.736 0 0 0 3.677 4.37a.07.07 0 0 0-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 0 0 .031.057 19.9 19.9 0 0 0 5.993 3.03.078.078 0 0 0 .084-.028c.462-.63.874-1.295 1.226-1.994.021-.041.001-.09-.041-.106a13.107 13.107 0 0 1-1.872-.892.077.077 0 0 1-.008-.128 10.2 10.2 0 0 0 .372-.292.074.074 0 0 1 .077-.01c3.929 1.793 8.18 1.793 12.061 0a.074.074 0 0 1 .078.01c.12.098.246.198.373.292a.077.077 0 0 1-.006.127 12.299 12.299 0 0 1-1.873.894.077.077 0 0 0-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 0 0 .084.028 19.839 19.839 0 0 0 6.002-3.03.077.077 0 0 0 .032-.054c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 0 0-.031-.028zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.956-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.955-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.946 2.418-2.157 2.418z" />
  </svg>
);

const KNOWN_MARKS = {
  slack: SlackMark,
  github: GitHubMark,
  linear: LinearMark,
  jira: JiraMark,
  sentry: SentryMark,
  notion: NotionMark,
  gitlab: GitLabMark,
  discord: DiscordMark,
};

const CATEGORIES = [
  { id: "all", label: "All" },
  { id: "work", label: "Work Management" },
  { id: "engineering", label: "Engineering & Ops" },
  { id: "knowledge", label: "Knowledge & Docs" },
  { id: "communication", label: "Communication & CRM" },
];

const BRAND_COLORS = {
  asana: "from-[#F06A6A]/20 to-[#F06A6A]/5 text-[#F06A6A]",
  monday: "from-[#00CA72]/20 to-[#00CA72]/5 text-[#00CA72]",
  trello: "from-[#0079BF]/20 to-[#0079BF]/5 text-[#0079BF]",
  clickup: "from-[#7B68EE]/20 to-[#7B68EE]/5 text-[#7B68EE]",
  datadog: "from-[#632CA6]/20 to-[#632CA6]/5 text-[#632CA6]",
  pagerduty: "from-[#06AC38]/20 to-[#06AC38]/5 text-[#06AC38]",
  confluence: "from-[#0052CC]/20 to-[#0052CC]/5 text-[#0052CC]",
  airtable: "from-[#FCB400]/20 to-[#FCB400]/5 text-[#FCB400]",
  coda: "from-[#F35940]/20 to-[#F35940]/5 text-[#F35940]",
  hubspot: "from-[#FF7A59]/20 to-[#FF7A59]/5 text-[#FF7A59]",
  intercom: "from-[#1F8DED]/20 to-[#1F8DED]/5 text-[#1F8DED]",
  zendesk: "from-[#03363D]/20 to-[#03363D]/5 text-[#03363D] dark:text-[#5FD1A5]",
  supabase: "from-[#3ECF8E]/20 to-[#3ECF8E]/5 text-[#3ECF8E]",
  vercel: "from-zinc-500/20 to-zinc-500/5 text-ink",
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
  const [tokenModal, setTokenModal] = useState(null);
  const [category, setCategory] = useState("all");
  const [search, setSearch] = useState("");

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
      const p = (data?.providers || []).find((x) => x.name === res.provider);
      const name = p?.display_name || res.provider;
      if (res.connected) notify(`${name} connected`, res.provider === "slack" ? "Choose the channels to sync." : "Syncing your items now.");
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

  const connectOAuth = async (provider) => {
    setBusy(`connect-${provider.id}`);
    try {
      const { url } = await api(apiBase, `/connectors/${provider.id}/authorize`, { method: "POST", auth });
      window.location.assign(url);
    } catch (err) {
      notify(`Couldn't start ${provider.name} sign-in`, err.message, true);
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

  const allProviders = useMemo(() => {
    if (!data?.providers) return [];
    return data.providers.map((p) => {
      const id = p.name;
      const Mark = KNOWN_MARKS[id];
      const name = p.display_name || (id.charAt(0).toUpperCase() + id.slice(1));
      const kind = p.kind || (id === "slack" || id === "github" ? "oauth" : "token");
      const unit = id === "slack" ? "channel" : id === "github" || id === "gitlab" ? "repository" : "item";
      return {
        id,
        name,
        category: p.category || "work",
        kind,
        unit,
        enabled: p.enabled,
        blurb: p.description || `Sync tasks, updates, and discussions from ${name} directly into memory.`,
        privacy: kind === "oauth"
          ? "Read-only access. You choose which resources Memro syncs."
          : "Read-only token. Sealed securely at rest with AES-256-GCM encryption.",
        secretRef: p.secret_ref || "API_TOKEN",
        Mark: Mark || null,
        initials: (name || id).slice(0, 2).toUpperCase(),
        colorStyle: BRAND_COLORS[id] || "from-accent/20 to-accent/5 text-accent",
      };
    });
  }, [data]);

  const filteredProviders = useMemo(() => {
    return allProviders.filter((p) => {
      const matchesCategory = category === "all" || p.category === category;
      const q = search.trim().toLowerCase();
      const matchesSearch = !q || p.name.toLowerCase().includes(q) || p.blurb.toLowerCase().includes(q);
      return matchesCategory && matchesSearch;
    });
  }, [allProviders, category, search]);

  const categoryCounts = useMemo(() => {
    const counts = { all: allProviders.length };
    for (const p of allProviders) {
      counts[p.category] = (counts[p.category] || 0) + 1;
    }
    return counts;
  }, [allProviders]);

  const anyEnabled = allProviders.some((p) => p.enabled);

  return (
    <div className="space-y-6 max-w-4xl pb-12">
      <PageHeader
        title="Integrations"
        description="Bring team context from 50 developer and collaboration tools directly into memory, so every connected agent can recall it."
      />

      {/* Category Pills & Instant Search */}
      <div className="flex flex-col sm:flex-row gap-3 items-stretch sm:items-center justify-between">
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0 scrollbar-none">
          {CATEGORIES.map((cat) => {
            const count = categoryCounts[cat.id] || 0;
            const active = category === cat.id;
            return (
              <button
                key={cat.id}
                onClick={() => setCategory(cat.id)}
                className={`px-3 py-1.5 rounded-lg text-[13px] font-medium transition-colors whitespace-nowrap flex items-center gap-1.5 ${
                  active ? "bg-accent text-white shadow-sm" : "bg-panel text-muted hover:text-ink hover:bg-panel/80"
                }`}
              >
                <span>{cat.label}</span>
                <span className={`text-[11px] px-1.5 py-0.2 rounded-full ${active ? "bg-white/20 text-white" : "bg-surface text-subtle"}`}>
                  {count}
                </span>
              </button>
            );
          })}
        </div>

        <div className="relative min-w-[200px]">
          <Icon name="search" className="absolute left-3 top-1/2 -translate-y-1/2 text-subtle !text-[18px]" />
          <input
            className="input pl-9 text-[13px] h-9"
            placeholder="Search connectors…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label="Search connectors"
          />
          {search && (
            <button
              onClick={() => setSearch("")}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-subtle hover:text-ink text-xs p-0.5"
            >
              <Icon name="close" className="!text-[14px]" />
            </button>
          )}
        </div>
      </div>

      {/* Connector Grid */}
      {data === null ? (
        <div className="grid sm:grid-cols-2 gap-4">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="card p-5 space-y-3">
              <div className="flex items-center gap-3">
                <Skeleton className="size-10 rounded-xl" />
                <div className="space-y-1.5 flex-1">
                  <Skeleton className="h-4 w-1/3" />
                  <Skeleton className="h-3 w-1/4" />
                </div>
              </div>
              <Skeleton className="h-10 w-full" />
            </div>
          ))}
        </div>
      ) : filteredProviders.length === 0 ? (
        <Empty icon="search_off" title="No connectors found">
          {search ? `No connectors match "${search}". Try searching for another name.` : "No connectors in this category."}
        </Empty>
      ) : (
        <div className="grid sm:grid-cols-2 gap-4">
          {filteredProviders.map((p) => {
            const isConnected = (data?.connections || []).some((c) => c.provider === p.id);
            const isBusy = busy === `connect-${p.id}`;

            return (
              <div key={p.id} className="card p-5 flex flex-col justify-between hover:border-line-hover transition-colors">
                <div>
                  <div className="flex items-center gap-3">
                    <div className={`size-10 rounded-xl border border-line bg-gradient-to-br ${p.colorStyle} flex items-center justify-center shadow-card shrink-0`}>
                      {p.Mark ? <p.Mark className="size-6" /> : <span className="text-xs font-bold font-mono tracking-tight">{p.initials}</span>}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <p className="text-[14.5px] font-semibold truncate">{p.name}</p>
                        {p.kind === "token" && (
                          <span className="text-[10px] uppercase font-mono px-1.5 py-0.5 rounded bg-surface border border-line text-subtle">
                            Token
                          </span>
                        )}
                      </div>
                      <p className="text-[12px] text-subtle">
                        {p.enabled ? "Available" : "Not configured on server"}
                      </p>
                    </div>
                  </div>

                  <p className="text-[13px] text-muted mt-3 line-clamp-2 leading-relaxed">{p.blurb}</p>
                  <p className="text-[11.5px] text-subtle mt-2 flex gap-1.5 items-center">
                    <Icon name="lock" className="!text-[13px] shrink-0" />
                    <span className="truncate">{p.privacy}</span>
                  </p>
                </div>

                <div className="mt-4 pt-2">
                  <button
                    className="btn-primary w-full justify-center"
                    disabled={!p.enabled || isBusy}
                    onClick={() => (p.kind === "oauth" ? connectOAuth(p) : setTokenModal(p))}
                  >
                    <Icon name="add_link" className="!text-[18px]" />
                    {isBusy ? "Opening…" : isConnected ? "Connect another" : `Connect ${p.name}`}
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Active Connections Panel */}
      <Panel title={`Connected Accounts${data ? ` · ${data.connections.length}` : ""}`} bodyClass="p-0">
        {data === null ? (
          <div className="p-5 space-y-3">
            <Skeleton className="h-4 w-1/2" />
            <Skeleton className="h-4 w-1/3" />
          </div>
        ) : data.connections.length === 0 ? (
          <Empty icon="hub" title="Nothing connected yet">
            {anyEnabled
              ? "Select any connector above to connect your account and start syncing."
              : "Ask your Memro administrator to configure connectors."}
          </Empty>
        ) : (
          <ul className="divide-y divide-line">
            {data.connections.map((c) => {
              const p = allProviders.find((x) => x.id === c.provider) || {
                name: c.provider,
                unit: "item",
                Mark: null,
                initials: c.provider.slice(0, 2).toUpperCase(),
                colorStyle: "bg-surface text-ink",
              };
              const needsChannels = c.provider === "slack" && c.status === "active" && c.sources_enabled === 0;

              return (
                <li key={c.id} className="px-5 py-4">
                  <div className="flex items-start gap-4">
                    <div className="size-9 rounded-lg border border-line bg-surface flex items-center justify-center shrink-0 mt-0.5">
                      {p.Mark ? <p.Mark className="size-5" /> : <span className="text-[11px] font-bold font-mono text-muted">{p.initials}</span>}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2 flex-wrap">
                        <p className="text-[14px] font-medium truncate">{c.account_name || c.account_id}</p>
                        <Status conn={c} />
                      </div>
                      <p className="text-[12.5px] text-subtle mt-1">
                        {p.name} · as {c.external_user_name || c.external_user_id} · {c.sources_enabled} of {c.sources_total} {plural(p.unit, c.sources_total)} · {" "}
                        <button className="underline decoration-line hover:text-ink" onClick={onOpenMemories}>
                          {c.memories.toLocaleString()} {c.memories === 1 ? "memory" : "memories"}
                        </button>
                        {" "}· synced {timeAgo(c.last_synced_at)}
                      </p>
                      {c.status_detail && (
                        <p className={`text-[12.5px] mt-2 ${c.status === "paused" ? "text-warn" : "text-danger"}`}>
                          {c.status_detail}
                        </p>
                      )}
                      {needsChannels && (
                        <p className="text-[12.5px] mt-2 text-accent">Choose which channels to sync — nothing is read until you do.</p>
                      )}
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0 flex-wrap justify-end">
                      {c.provider === "slack" && c.status !== "revoked" && (
                        <button className={needsChannels ? "btn-primary" : "btn-secondary"} onClick={() => setChannelsFor(c)}>
                          Channels
                        </button>
                      )}
                      {c.manage_url && c.status !== "revoked" && (
                        <a className="btn-secondary" href={c.manage_url} target="_blank" rel="noopener noreferrer">
                          Manage <Icon name="arrow_outward" className="!text-[15px]" />
                        </a>
                      )}
                      {c.status === "revoked" ? (
                        <button
                          className="btn-primary"
                          onClick={() => (p.kind === "oauth" ? connectOAuth(p) : setTokenModal(p))}
                          disabled={!p.enabled}
                        >
                          Reconnect
                        </button>
                      ) : (
                        <button
                          className="btn-ghost"
                          onClick={() => syncNow(c)}
                          disabled={busy === `sync-${c.id}`}
                          title={c.status === "paused" ? "Resume and sync" : "Sync now"}
                        >
                          <Icon name="sync" className="!text-[18px]" />
                          {c.status === "paused" || c.status === "error" ? "Resume" : ""}
                        </button>
                      )}
                      <button className="btn-ghost text-danger" onClick={() => setRemoving(c)}>
                        Disconnect
                      </button>
                    </div>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </Panel>

      <TokenModal
        apiBase={apiBase}
        auth={auth}
        notify={notify}
        provider={tokenModal}
        onClose={() => setTokenModal(null)}
        onDone={load}
      />
      <ChannelPicker
        apiBase={apiBase}
        auth={auth}
        notify={notify}
        conn={channelsFor}
        onClose={() => setChannelsFor(null)}
        onSaved={load}
      />
      <DisconnectModal
        apiBase={apiBase}
        auth={auth}
        notify={notify}
        conn={removing}
        onClose={() => setRemoving(null)}
        onDone={load}
      />
    </div>
  );
}

function TokenModal({ apiBase, auth, notify, provider, onClose, onDone }) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [showToken, setShowToken] = useState(false);

  useEffect(() => {
    setToken("");
    setShowToken(false);
  }, [provider?.id]);

  if (!provider) return null;

  const submit = async (e) => {
    e.preventDefault();
    if (!token.trim()) return;
    setBusy(true);
    try {
      await api(apiBase, `/connectors/${provider.id}/token`, {
        method: "POST",
        auth,
        json: { token: token.trim() },
      });
      notify(`Connected ${provider.name}`, "Sync job has been scheduled.");
      onClose();
      onDone();
    } catch (err) {
      notify(`Couldn't connect ${provider.name}`, err.message, true);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={!!provider} title={`Connect ${provider.name}`} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <p className="text-[13px] text-muted">
          Paste your API token or personal access token with read permissions. Memro seals it with AES-256-GCM encryption and uses it only to sync your items into memory.
        </p>

        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <label className="text-[12px] font-medium text-ink">
              {provider.secretRef || "API Token"}
            </label>
            <button
              type="button"
              className="text-[11.5px] text-subtle hover:text-ink cursor-pointer"
              onClick={() => setShowToken(!showToken)}
            >
              {showToken ? "Hide" : "Show"}
            </button>
          </div>
          <input
            type={showToken ? "text" : "password"}
            className="input font-mono text-[13px]"
            placeholder={`Paste your ${provider.name} token…`}
            value={token}
            onChange={(e) => setToken(e.target.value)}
            autoFocus
            required
          />
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" className="btn-ghost" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button type="submit" className="btn-primary" disabled={busy || !token.trim()}>
            {busy ? "Connecting…" : `Connect ${provider.name}`}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function ChannelPicker({ apiBase, auth, notify, conn, onClose, onSaved }) {
  const [sources, setSources] = useState(null);
  const [chosen, setChosen] = useState({});
  const [filter, setFilter] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!conn) return;
    setSources(null);
    setFilter("");
    api(apiBase, `/connectors/${conn.id}/sources`, { auth })
      .then((s) => {
        setSources(s);
        setChosen(Object.fromEntries(s.map((x) => [x.id, x.enabled])));
      })
      .catch((err) => {
        notify("Couldn't load channels", err.message, true);
        onClose();
      });
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
    <Drawer
      open={!!conn}
      title={`Channels · ${conn?.account_name || ""}`}
      onClose={onClose}
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>Cancel</button>
          <button className="btn-primary" onClick={save} disabled={!changed || saving}>
            {saving ? "Saving…" : changed ? `Save ${changed} ${changed === 1 ? "change" : "changes"}` : "Save"}
          </button>
        </>
      }
    >
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
          <li className="px-2 py-6 text-[13px] text-subtle text-center">
            {sources.length ? "No channels match." : "No channels yet. If you just connected, give it a moment and reopen this list."}
          </li>
        ) : (
          shown.map((s) => (
            <li key={s.id}>
              <label className="flex items-center gap-3 px-2 py-2 rounded-lg hover:bg-panel cursor-pointer">
                <input
                  type="checkbox"
                  className="size-4 accent-[var(--color-accent)]"
                  checked={!!chosen[s.id]}
                  onChange={(e) => setChosen((c) => ({ ...c, [s.id]: e.target.checked }))}
                />
                <Icon name={s.private ? "lock" : "tag"} className="!text-[16px] text-subtle" />
                <span className="text-[13.5px] flex-1 truncate">{s.name}</span>
                {s.enabled && (
                  <span className="text-[11.5px] text-subtle">{s.last_synced_at ? `synced ${timeAgo(s.last_synced_at)}` : "first sync…"}</span>
                )}
              </label>
            </li>
          ))
        )}
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

  const name = conn?.provider;
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
