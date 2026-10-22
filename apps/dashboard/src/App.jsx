import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Login from "./components/Login";
import CommandPalette from "./components/CommandPalette";
import ExtensionView from "./views/ExtensionView";
import { loadExtensions, withExtensions } from "./lib/extensions";
import { Icon, Logo, Modal, Field, StatusPill, MEMORY_TYPES, TYPE_INFO } from "./components/ui";
import { api } from "./lib/api";
import Overview from "./views/Overview";
import Memories from "./views/Memories";
import Search from "./views/Search";
import Agent from "./views/Agent";
import Activity from "./views/Activity";
import Settings from "./views/Settings";
import Connect from "./views/Connect";
import Keys from "./views/Keys";
import Integrations from "./views/Integrations";

const store = {
  get: (k) => { try { return localStorage.getItem(k); } catch { return null; } },
  set: (k, v) => { try { v == null ? localStorage.removeItem(k) : localStorage.setItem(k, v); } catch { /* storage blocked */ } },
};

const NAV = [
  { group: "Memory", items: [
    { id: "overview", label: "Overview", icon: "space_dashboard" },
    { id: "memories", label: "Memories", icon: "stacks" },
    { id: "search", label: "Search", icon: "search" },
  ] },
  { group: "Connect", items: [
    { id: "connect", label: "Connect tools", icon: "cable" },
    { id: "integrations", label: "Integrations", icon: "hub" },
    { id: "keys", label: "API keys", icon: "key" },
  ] },
  { group: "Account", items: [
    { id: "agent", label: "Account", icon: "person" },
    { id: "activity", label: "Activity", icon: "history" },
    { id: "settings", label: "Settings", icon: "tune" },
  ] },
];
const DOCS_URL = "https://github.com/memrohq/memro/tree/main/docs#readme";

// Files the server can turn into memories: images are described, documents chunked.
const IMAGE_TYPES = /^image\//;
const DOC_EXT = /\.(pdf|docx|pptx|xlsx|txt|md|markdown|csv|json|ya?ml|html?)$/i;

// `npx memro-cli connect` opens /connect?code=XXXX-XXXX or /connect?user_code=XXXX-XXXX.
const initialConnectCode = () => {
  if (!window.location.pathname.startsWith("/connect")) return null;
  const p = new URLSearchParams(window.location.search);
  return p.get("code") || p.get("user_code");
};

function useTheme() {
  const [theme, setTheme] = useState(() => store.get("memro_theme") || "system");
  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => document.documentElement.classList.toggle("dark", theme === "dark" || (theme === "system" && mq.matches));
    apply();
    store.set("memro_theme", theme);
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, [theme]);
  return [theme, setTheme];
}

export default function App() {
  const [sessionToken, setSessionToken] = useState(() => store.get("memro_session_token"));
  const [apiBase, setApiBase] = useState(() => store.get("memro_api_base") || import.meta.env.VITE_API_BASE_URL || "/api");
  const [agentId, setAgentId] = useState(() => store.get("memro_did"));
  const [email, setEmail] = useState(() => store.get("memro_email"));
  const [connectCode, setConnectCode] = useState(initialConnectCode);
  const [theme, setTheme] = useTheme();
  const [extensions, setExtensions] = useState([]);
  const [systemDark, setSystemDark] = useState(() => window.matchMedia("(prefers-color-scheme: dark)").matches);

  // Deep links: /connect from `memro connect`, /integrations after Slack or GitHub sign-in.
  const [view, setView] = useState(() => {
    const path = window.location.pathname;
    return path.startsWith("/connect") ? "connect" : path.startsWith("/integrations") ? "integrations" : "overview";
  });
  const [navOpen, setNavOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [health, setHealth] = useState({});
  const [memories, setMemories] = useState([]);
  const [loaded, setLoaded] = useState(false);
  const [stats, setStats] = useState(null);
  const [usage, setUsage] = useState(null);
  const [events, setEvents] = useState([]);
  const [toast, setToast] = useState(null);
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(null);
  const [addOpen, setAddOpen] = useState(false);
  const [draft, setDraft] = useState({ content: "", type: "auto", visibility: "private" });
  const [searchSeed, setSearchSeed] = useState({ q: "", n: 0 });
  const fileRef = useRef(null);

  const auth = { sessionToken };
  const initialized = !!agentId;

  const notify = useCallback((title, detail = "", error = false) => {
    setEvents((prev) => [{ id: crypto.randomUUID(), at: new Date().toISOString(), title, detail, error }, ...prev].slice(0, 200));
    setToast({ id: Date.now(), title, detail, error });
  }, []);

  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 4000);
    return () => clearTimeout(t);
  }, [toast]);

  const signOut = useCallback(async () => {
    if (sessionToken) {
      try { await api(apiBase, "/auth/logout", { method: "DELETE", auth: { sessionToken } }); } catch { /* best effort */ }
    }
    ["memro_session_token", "memro_did", "memro_pk", "memro_email"].forEach((k) => store.set(k, null));
    setSessionToken(null); setAgentId(null); setEmail(null);
    setMemories([]); setStats(null); setUsage(null); setLoaded(false); setView("overview");
  }, [apiBase, sessionToken]);

  // Server health, polled.
  useEffect(() => {
    let alive = true;
    const check = async () => {
      try {
        const h = await api(apiBase, "/health");
        if (alive) setHealth({ api: "ok", db: h.db, vector: h.vector_store, embeddings: h.embeddings });
      } catch {
        if (alive) setHealth({ api: "error" });
      }
    };
    check();
    const id = setInterval(check, 15000);
    return () => { alive = false; clearInterval(id); };
  }, [apiBase]);

  // Older dashboards kept the agent's private key in localStorage; drop it.
  useEffect(() => { store.set("memro_pk", null); }, []);

  // The session says which agent it acts as.
  useEffect(() => {
    if (!sessionToken || agentId) return;
    api(apiBase, "/auth/whoami", { auth: { sessionToken } })
      .then((who) => { store.set("memro_did", who.agent_id); setAgentId(who.agent_id); })
      .catch((err) => { if (err.status === 401) signOut(); });
  }, [apiBase, sessionToken, agentId, signOut]);

  const load = useCallback(async () => {
    if (!sessionToken || !initialized) return;
    try {
      const [mems, st, us] = await Promise.all([
        api(apiBase, `/memory/agent/${agentId}?limit=100`, { auth }),
        api(apiBase, "/stats/temporal", { auth }).catch(() => null),
        api(apiBase, "/usage", { auth }).catch(() => null),
      ]);
      setMemories(mems || []); setStats(st); setUsage(us);
    } catch (err) {
      if (err.status === 401) {
        notify("Session expired", "Please sign in again.", true);
        signOut();
      } else {
        notify("Couldn't load memories", err.message, true);
      }
    } finally {
      setLoaded(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, agentId, sessionToken]);

  useEffect(() => { load(); }, [load]);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => setSystemDark(mq.matches);
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);
  const dark = theme === "dark" || (theme === "system" && systemDark);

  // Views a deployment adds through /extensions.json; /<id> or /x/<id> opens one.
  useEffect(() => {
    let alive = true;
    loadExtensions(NAV.flatMap((g) => g.items.map((i) => i.id))).then((views) => {
      if (!alive) return;
      setExtensions(views);
      const m = window.location.pathname.match(/^\/(?:x\/)?([a-z0-9-]+)\/?$/);
      const hit = m && views.find((v) => v.id === m[1]);
      if (hit) setView(`x:${hit.id}`);
    });
    return () => { alive = false; };
  }, []);
  const nav = useMemo(() => withExtensions(NAV, extensions), [extensions]);
  const titles = useMemo(() => Object.fromEntries(nav.flatMap((g) => g.items.map((i) => [i.id, i.label]))), [nav]);
  const activeExtension = extensions.find((v) => `x:${v.id}` === view);

  // ⌘K / Ctrl+K opens the command palette; "/" too, outside text fields.
  useEffect(() => {
    const onKey = (e) => {
      const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName) || e.target.isContentEditable;
      if (((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") || (e.key === "/" && !typing)) {
        e.preventDefault();
        setPaletteOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const go = useCallback((id) => {
    setView(id); setNavOpen(false);
    if (id.startsWith("x:")) { setConnectCode(null); window.history.replaceState(null, "", `/${id.slice(2)}`); return; }
    if (id !== "connect" && window.location.pathname !== "/") { setConnectCode(null); window.history.replaceState(null, "", "/"); }
  }, []);

  const runSearch = useCallback((q) => { setSearchSeed((s) => ({ q, n: s.n + 1 })); go("search"); }, [go]);

  const addMemory = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api(apiBase, "/memory", {
        method: "POST", auth,
        json: { agent_id: agentId, content: draft.content, memory_type: draft.type, visibility: draft.visibility },
      });
      notify("Memory added", draft.content.slice(0, 80));
      setAddOpen(false);
      setDraft({ content: "", type: "auto", visibility: "private" });
      load();
    } catch (err) {
      notify("Couldn't add memory", err.message, true);
    } finally {
      setBusy(false);
    }
  };

  const deleteMemory = async (id) => {
    try {
      await api(apiBase, `/memory/${id}`, { method: "DELETE", auth });
      notify("Memory deleted", String(id).slice(0, 8));
      setMemories((prev) => prev.filter((m) => m.id !== id));
      load();
    } catch (err) {
      notify("Couldn't delete memory", err.message, true);
    }
  };

  /** Images become a described memory; documents become chunked memories. */
  const uploadFiles = async (fileList) => {
    const files = Array.from(fileList || []);
    if (!files.length) return;
    for (const file of files) {
      const isImage = IMAGE_TYPES.test(file.type);
      if (!isImage && !DOC_EXT.test(file.name)) {
        notify("Unsupported file", `${file.name}: upload an image, PDF, Word, PowerPoint, Excel or text file.`, true);
        continue;
      }
      const form = new FormData();
      form.append(isImage ? "image" : "document", file);
      setUploading(file.name);
      try {
        const res = await api(apiBase, isImage ? "/upload/image" : "/upload/document", { method: "POST", auth, form });
        const n = Array.isArray(res) ? res.length : 1;
        notify("File added", `${file.name} · ${n} ${n === 1 ? "memory" : "memories"}`);
      } catch (err) {
        notify("Upload failed", `${file.name}: ${err.message}`, true);
      }
    }
    setUploading(null);
    load();
  };

  const search = async (query, mode) => {
    const res = await api(apiBase, `/search?agent_id=${agentId}&query=${encodeURIComponent(query)}&mode=${mode}&limit=20`, { auth });
    notify("Searched", `“${query}” · ${res.results.length} ${res.results.length === 1 ? "result" : "results"}`);
    return res;
  };

  const exportData = async () => {
    try {
      const data = await api(apiBase, `/export/${agentId}`, { auth });
      const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: "application/json" }));
      const a = Object.assign(document.createElement("a"), { href: url, download: `memro-export-${agentId.slice(0, 8)}.json` });
      a.click();
      URL.revokeObjectURL(url);
      notify("Export ready", `${data.total} memories`);
    } catch (err) {
      notify("Export failed", err.message, true);
    }
  };

  const copy = async (text, what) => {
    try { await navigator.clipboard.writeText(text); notify("Copied", what); }
    catch { notify("Copy failed", "Your browser blocked clipboard access.", true); }
  };

  const saveApiBase = (value) => {
    store.set("memro_api_base", value === "/api" ? null : value);
    setApiBase(value);
    notify("Settings saved", `API endpoint: ${value}`);
  };

  const cycleTheme = () => setTheme((t) => (t === "light" ? "dark" : t === "dark" ? "system" : "light"));
  const themeIcon = theme === "light" ? "light_mode" : theme === "dark" ? "dark_mode" : "contrast";

  const commands = useMemo(() => [
    ...nav.flatMap((g) => g.items.map((i) => ({ id: `go-${i.id}`, label: i.label, icon: i.icon, group: "Go to", run: () => go(i.id) }))),
    { id: "add", label: "Add memory", icon: "add", group: "Actions", hint: "N", run: () => setAddOpen(true) },
    { id: "upload", label: "Upload files", icon: "upload", group: "Actions", run: () => fileRef.current?.click() },
    { id: "export", label: "Export all memories", icon: "download", group: "Actions", run: exportData },
    { id: "theme", label: `Theme: ${theme === "system" ? "system" : theme} (switch)`, icon: themeIcon, group: "Preferences", run: cycleTheme },
    { id: "docs", label: "Open documentation", icon: "menu_book", group: "Help", run: () => window.open(DOCS_URL, "_blank", "noopener") },
    { id: "signout", label: "Sign out", icon: "logout", group: "Account", run: signOut },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [go, theme, signOut, agentId, nav]);

  if (!sessionToken) {
    return (
      <Login
        apiBaseUrl={apiBase}
        onLoginSuccess={(token, id, _userId, mail) => {
          store.set("memro_session_token", token); store.set("memro_email", mail);
          if (id) { store.set("memro_did", id); setAgentId(id); }
          setEmail(mail); setSessionToken(token);
        }}
      />
    );
  }

  const initial = (email || "M").charAt(0).toUpperCase();

  return (
    <div className="h-full flex">
      {/* Sidebar */}
      {navOpen && <div className="md:hidden fixed inset-0 z-30 bg-ink/20 dark:bg-black/50 fade-in" onClick={() => setNavOpen(false)} />}
      <aside
        className={`${navOpen ? "translate-x-0" : "-translate-x-full"} md:translate-x-0 fixed md:static inset-y-0 left-0 z-40 w-[260px] shrink-0 flex flex-col bg-canvas border-r border-line transition-transform duration-200`}
      >
        <div className="h-16 px-5 flex items-center gap-2.5">
          <Logo className="size-7" />
          <span className="text-[16px] font-semibold tracking-[-0.02em]">memro</span>
          <span className="ml-1 h-5 px-1.5 rounded-md border border-line text-[10.5px] font-medium text-muted flex items-center">Cloud</span>
          <button className="md:hidden btn-ghost size-8 px-0 ml-auto" onClick={() => setNavOpen(false)} aria-label="Close menu"><Icon name="close" /></button>
        </div>

        <div className="px-3 pb-2">
          <button onClick={() => setPaletteOpen(true)} className="w-full h-9 flex items-center gap-2 px-3 rounded-[10px] border border-line bg-surface text-[13px] text-subtle hover:border-line-strong hover:text-muted transition-colors shadow-card">
            <Icon name="search" className="!text-[17px]" />
            <span className="flex-1 text-left">Search…</span>
            <span className="kbd">⌘K</span>
          </button>
        </div>

        <nav className="flex-1 overflow-y-auto scroll-thin px-3 pb-3">
          {nav.map((g) => (
            <div key={g.group} className="mt-4">
              <p className="label px-3 mb-1.5">{g.group}</p>
              {g.items.map((item) => {
                const active = view === item.id;
                return (
                  <button
                    key={item.id}
                    onClick={() => go(item.id)}
                    aria-current={active ? "page" : undefined}
                    className={`relative w-full flex items-center gap-3 h-9 px-3 rounded-[10px] text-[13.5px] transition-colors ${
                      active ? "bg-surface text-ink font-medium shadow-card border border-line" : "text-muted hover:text-ink hover:bg-surface/60 border border-transparent"
                    }`}
                  >
                    <Icon name={item.icon} fill={active} className={`!text-[19px] ${active ? "text-accent" : "text-subtle"}`} />
                    {item.label}
                    {item.id === "memories" && stats?.total_memories > 0 && (
                      <span className="ml-auto text-[11.5px] tabular text-subtle">{stats.total_memories}</span>
                    )}
                  </button>
                );
              })}
            </div>
          ))}
          <div className="mt-4">
            <p className="label px-3 mb-1.5">Resources</p>
            <a href={DOCS_URL} target="_blank" rel="noopener" className="w-full flex items-center gap-3 h-9 px-3 rounded-[10px] text-[13.5px] text-muted hover:text-ink hover:bg-surface/60">
              <Icon name="menu_book" className="!text-[19px] text-subtle" /> Docs & SDKs
              <Icon name="arrow_outward" className="ml-auto text-subtle !text-[15px]" />
            </a>
          </div>
        </nav>

        {usage?.memory_limit > 0 && (
          <div className="mx-3 mb-3 p-3.5 rounded-xl border border-line bg-surface shadow-card">
            <div className="flex items-center justify-between text-[12px]">
              <span className="text-muted">Memory used</span>
              <span className="tabular font-medium">{usage.memory_nodes.toLocaleString()} / {usage.memory_limit.toLocaleString()}</span>
            </div>
            <div className="mt-2 h-1.5 rounded-full bg-panel overflow-hidden" role="progressbar" aria-valuemin={0}
              aria-valuemax={usage.memory_limit} aria-valuenow={usage.memory_nodes} aria-label="Memory used">
              <div className={`h-full rounded-full ${usage.warning ? "bg-warn" : "bg-accent"}`}
                style={{ width: `${Math.min(100, (usage.memory_nodes / usage.memory_limit) * 100)}%` }} />
            </div>
          </div>
        )}

        <div className="p-3 border-t border-line">
          <div className="flex items-center gap-2.5 px-1.5">
            <div className="size-8 rounded-full bg-gradient-to-br from-accent to-episodic text-on-accent flex items-center justify-center text-[13px] font-semibold shrink-0">{initial}</div>
            <div className="min-w-0 flex-1">
              <p className="text-[13px] font-medium truncate">{email || "Signed in"}</p>
              <p className="text-[11px] text-subtle truncate font-mono">{initialized ? `${agentId.slice(0, 12)}…` : "Setting up…"}</p>
            </div>
            <button className="btn-ghost size-8 px-0" onClick={cycleTheme} aria-label={`Theme: ${theme}`} title={`Theme: ${theme}`}><Icon name={themeIcon} className="!text-[18px]" /></button>
            <button className="btn-ghost size-8 px-0" onClick={signOut} aria-label="Sign out" title="Sign out"><Icon name="logout" className="!text-[18px]" /></button>
          </div>
        </div>
      </aside>

      {/* Main */}
      <div className="flex-1 min-w-0 flex flex-col">
        <header className="h-16 shrink-0 flex items-center gap-3 px-4 md:px-8 border-b border-line bg-canvas/80 backdrop-blur-md sticky top-0 z-20">
          <button className="md:hidden btn-ghost size-9 px-0" onClick={() => setNavOpen(true)} aria-label="Open menu"><Icon name="menu" /></button>
          <nav className="flex items-center gap-1.5 text-[13px] min-w-0" aria-label="Breadcrumb">
            <span className="text-subtle hidden sm:inline">Dashboard</span>
            <Icon name="chevron_right" className="text-subtle !text-[16px] hidden sm:inline" />
            <span className="font-medium truncate">{titles[view]}</span>
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <StatusPill ok={health.api === "ok"} okText="Operational" badText="Offline" />
            <button className="btn-ghost size-9 px-0" onClick={cycleTheme} aria-label={`Theme: ${theme}`} title={`Theme: ${theme}`}>
              <Icon name={themeIcon} className="!text-[19px]" />
            </button>
          </div>
        </header>

        <main className="flex-1 overflow-y-auto scroll-thin">
          {/* The connector catalog uses the full width; other pages keep a reading width. */}
          <div className={`${view === "integrations" ? "max-w-[1680px]" : "max-w-[1240px]"} mx-auto px-4 md:px-8 py-6 md:py-8`}>
            <div key={view} className="fade-in">
              {activeExtension ? (
                <ExtensionView view={activeExtension} dark={dark} />
              ) : view === "connect" ? (
                <Connect apiBase={apiBase} auth={auth} code={connectCode} onDone={() => go("overview")} onCopy={copy} notify={notify} />
              ) : view === "keys" ? (
                <Keys apiBase={apiBase} auth={auth} onCopy={copy} notify={notify} />
              ) : view === "integrations" ? (
                <Integrations apiBase={apiBase} auth={auth} notify={notify} onOpenMemories={() => { load(); go("memories"); }} />
              ) : !initialized && view !== "settings" ? (
                <p className="text-[13px] text-muted">Loading your account…</p>
              ) : view === "overview" ? (
                <Overview email={email} stats={stats} usage={usage} memories={memories} loaded={loaded}
                  onOpenMemories={() => go("memories")} onAdd={() => setAddOpen(true)} onDelete={deleteMemory}
                  onConnect={() => go("connect")} onUpload={() => fileRef.current?.click()} />
              ) : view === "memories" ? (
                <Memories memories={memories} loaded={loaded} onAdd={() => setAddOpen(true)} onDelete={deleteMemory}
                  onUploadFiles={uploadFiles} uploading={uploading} onCopy={copy} />
              ) : view === "search" ? (
                <Search key={searchSeed.n} initialQuery={searchSeed.q} onSearch={search} onDelete={deleteMemory} onCopy={copy} />
              ) : view === "agent" ? (
                <Agent agentId={agentId} email={email} onCopy={copy} onExport={exportData} onSignOut={signOut} onOpenKeys={() => go("keys")} />
              ) : view === "activity" ? (
                <Activity events={events} />
              ) : (
                <Settings apiBaseUrl={apiBase} health={health} onSave={saveApiBase} theme={theme} onTheme={setTheme} />
              )}
            </div>
          </div>
        </main>
      </div>

      <input ref={fileRef} type="file" multiple className="hidden"
        accept="image/*,.pdf,.docx,.pptx,.xlsx,.txt,.md,.csv,.json,.yaml,.yml,.html"
        onChange={(e) => { uploadFiles(e.target.files); e.target.value = ""; }} />

      <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} commands={commands} onSearch={runSearch} />

      <Modal open={addOpen} title="New memory" description="Something your agents should remember." onClose={() => setAddOpen(false)}>
        <form onSubmit={addMemory} className="space-y-5">
          <Field label="Content">
            <textarea
              autoFocus
              required
              maxLength={10000}
              className="input h-32 py-2.5 resize-none leading-relaxed"
              placeholder="e.g. Ana prefers short answers with code first."
              value={draft.content}
              onChange={(e) => setDraft({ ...draft, content: e.target.value })}
              onKeyDown={(e) => { if ((e.metaKey || e.ctrlKey) && e.key === "Enter") e.currentTarget.form.requestSubmit(); }}
            />
            <span className="flex justify-between text-[11.5px] text-subtle mt-1.5">
              <span><span className="kbd">⌘</span> <span className="kbd">↵</span> to save</span>
              <span className="tabular">{draft.content.length.toLocaleString()} / 10,000</span>
            </span>
          </Field>
          <div>
            <span className="block text-[12.5px] font-medium mb-1.5">Type</span>
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
              {["auto", ...MEMORY_TYPES].map((t) => (
                <button type="button" key={t} onClick={() => setDraft({ ...draft, type: t })}
                  className={`text-left px-3 py-2 rounded-[10px] border text-[12.5px] transition-colors ${draft.type === t ? "border-accent bg-accent-soft ring-4 ring-accent/10" : "border-line hover:border-line-strong"}`}>
                  <span className="flex items-center gap-1.5 font-medium">
                    {t === "auto" ? <Icon name="auto_awesome" className="!text-[14px] text-accent" /> : <span className={`size-2 rounded-full ${TYPE_INFO[t].fill}`} />}
                    {t === "auto" ? "Auto" : TYPE_INFO[t].label}
                  </span>
                  <span className="block text-[11px] text-muted mt-0.5">{t === "auto" ? "Classify for me" : TYPE_INFO[t].hint}</span>
                </button>
              ))}
            </div>
          </div>
          <div className="flex items-center justify-between gap-2 pt-1">
            <select className="input w-auto h-9" value={draft.visibility} onChange={(e) => setDraft({ ...draft, visibility: e.target.value })} aria-label="Visibility">
              <option value="private">Private</option>
              <option value="shared">Shared</option>
              <option value="public">Public</option>
            </select>
            <div className="flex gap-2">
              <button type="button" className="btn-ghost" onClick={() => setAddOpen(false)}>Cancel</button>
              <button className="btn-primary" disabled={busy || !draft.content.trim()}>{busy ? "Saving…" : "Save memory"}</button>
            </div>
          </div>
        </form>
      </Modal>

      {uploading && (
        <div role="status" className="fixed bottom-5 left-1/2 -translate-x-1/2 md:left-auto md:translate-x-0 md:right-5 z-50 bg-elevated rounded-xl shadow-pop px-4 py-3 flex items-center gap-3 pop-in">
          <span className="size-4 rounded-full border-2 border-accent/30 border-t-accent animate-spin" />
          <p className="text-[13px]">Reading <span className="font-medium">{uploading}</span>…</p>
        </div>
      )}
      {toast && !uploading && (
        <div key={toast.id} role="status" className="fixed bottom-5 left-1/2 -translate-x-1/2 md:left-auto md:translate-x-0 md:right-5 z-50 bg-elevated rounded-xl shadow-pop pl-3 pr-4 py-3 max-w-sm flex gap-3 items-start pop-in">
          <Icon name={toast.error ? "error" : "check_circle"} fill className={`!text-[20px] ${toast.error ? "text-danger" : "text-ok"}`} />
          <div className="min-w-0">
            <p className="text-[13px] font-medium">{toast.title}</p>
            {toast.detail && <p className="text-[12.5px] text-muted line-clamp-2 mt-0.5">{toast.detail}</p>}
          </div>
        </div>
      )}
    </div>
  );
}
