import { useCallback, useEffect, useRef, useState } from "react";
import Login from "./components/Login";
import { Icon, Logo, Modal, Field, StatusPill } from "./components/ui";
import { api } from "./lib/api";
import Overview from "./views/Overview";
import Memories from "./views/Memories";
import Search from "./views/Search";
import Agent from "./views/Agent";
import Activity from "./views/Activity";
import Settings from "./views/Settings";
import Connect from "./views/Connect";
import Keys from "./views/Keys";

const store = {
  get: (k) => localStorage.getItem(k),
  set: (k, v) => (v == null ? localStorage.removeItem(k) : localStorage.setItem(k, v)),
};

const NAV = [
  { group: "Main menu", items: [
    { id: "overview", label: "Overview", icon: "space_dashboard" },
    { id: "memories", label: "Memories", icon: "stacks" },
    { id: "search", label: "Search", icon: "search" },
  ] },
  { group: "Connect", items: [
    { id: "connect", label: "Connect tools", icon: "cable" },
    { id: "keys", label: "API keys", icon: "vpn_key" },
  ] },
  { group: "Account", items: [
    { id: "agent", label: "Account", icon: "person" },
    { id: "activity", label: "Activity", icon: "history" },
  ] },
  { group: "Settings", items: [
    { id: "settings", label: "Settings", icon: "settings" },
  ] },
];
const TITLES = Object.fromEntries(NAV.flatMap((g) => g.items.map((i) => [i.id, i.label])));

// `npx memro connect` opens /connect?code=XXXX-XXXX.
const initialConnectCode = () =>
  window.location.pathname.startsWith("/connect") ? new URLSearchParams(window.location.search).get("code") : null;

export default function App() {
  const [sessionToken, setSessionToken] = useState(() => store.get("memro_session_token"));
  const [apiBase, setApiBase] = useState(() => store.get("memro_api_base") || import.meta.env.VITE_API_BASE_URL || "/api");
  const [agentId, setAgentId] = useState(() => store.get("memro_did"));
  const [email, setEmail] = useState(() => store.get("memro_email"));
  const [connectCode, setConnectCode] = useState(initialConnectCode);

  const [view, setView] = useState(() => (window.location.pathname.startsWith("/connect") ? "connect" : "overview"));
  const [navOpen, setNavOpen] = useState(false);
  const [health, setHealth] = useState({});
  const [memories, setMemories] = useState([]);
  const [stats, setStats] = useState(null);
  const [usage, setUsage] = useState(null);
  const [events, setEvents] = useState([]);
  const [toast, setToast] = useState(null);
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [draft, setDraft] = useState({ content: "", type: "episodic", visibility: "private" });
  const [searchSeed, setSearchSeed] = useState({ q: "", n: 0 });
  const searchRef = useRef(null);

  const auth = { sessionToken };
  const initialized = !!agentId;

  const notify = useCallback((title, detail = "", error = false) => {
    setEvents((prev) => [{ id: crypto.randomUUID(), at: new Date().toISOString(), title, detail, error }, ...prev].slice(0, 200));
    setToast({ title, detail, error });
  }, []);

  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 3500);
    return () => clearTimeout(t);
  }, [toast]);

  const signOut = useCallback(async () => {
    if (sessionToken) {
      try { await api(apiBase, "/auth/logout", { method: "DELETE", auth: { sessionToken } }); } catch { /* best effort */ }
    }
    ["memro_session_token", "memro_did", "memro_pk", "memro_email"].forEach((k) => store.set(k, null));
    setSessionToken(null); setAgentId(null); setEmail(null);
    setMemories([]); setStats(null); setUsage(null); setView("overview");
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
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [apiBase, agentId, sessionToken]);

  useEffect(() => { load(); }, [load]);

  // ⌘K / Ctrl+K focuses the global search.
  useEffect(() => {
    const onKey = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") { e.preventDefault(); searchRef.current?.focus(); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const go = (id) => {
    setView(id); setNavOpen(false);
    if (id !== "connect" && window.location.pathname !== "/") { setConnectCode(null); window.history.replaceState(null, "", "/"); }
  };

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
      setDraft({ content: "", type: "episodic", visibility: "private" });
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

  const upload = async (event) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    const isImage = file.type.startsWith("image/");
    if (!isImage && file.type !== "application/pdf") return notify("Unsupported file", "Upload an image or a PDF.", true);
    const form = new FormData();
    form.append(isImage ? "image" : "document", file);
    setUploading(true);
    try {
      await api(apiBase, isImage ? "/upload/image" : "/upload/document", { method: "POST", auth, form });
      notify("File processed", file.name);
      load();
    } catch (err) {
      notify("Upload failed", err.message, true);
    } finally {
      setUploading(false);
    }
  };

  const search = async (query, mode) => {
    const res = await api(apiBase, `/search?agent_id=${agentId}&query=${encodeURIComponent(query)}&mode=${mode}&limit=20`, { auth });
    notify("Searched", `“${query}” · ${res.results.length} results`);
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
    <div className="h-full md:p-3">
      <div className="h-full md:card md:rounded-2xl flex overflow-hidden bg-surface">
        {/* Sidebar */}
        <aside
          className={`${navOpen ? "fixed inset-0 z-40 flex" : "hidden"} md:static md:flex w-full md:w-[240px] shrink-0 flex-col bg-panel md:border-r border-line`}
        >
          <div className="p-3">
            <div className="card flex items-center gap-2.5 px-2.5 py-2">
              <div className="size-8 rounded-lg bg-surface border border-line flex items-center justify-center"><Logo className="size-6" /></div>
              <div className="min-w-0 flex-1">
                <p className="text-[11px] text-subtle leading-tight">Workspace</p>
                <p className="text-[13px] font-medium leading-tight truncate">Memro</p>
              </div>
              <button className="md:hidden btn-ghost size-8 px-0" onClick={() => setNavOpen(false)} aria-label="Close menu"><Icon name="close" /></button>
            </div>
          </div>

          <nav className="flex-1 overflow-y-auto scroll-thin px-3 pb-3">
            {NAV.map((g) => (
              <div key={g.group} className="mt-3 first:mt-1">
                <p className="text-[12px] font-medium text-muted px-2 mb-1">{g.group}</p>
                {g.items.map((item) => (
                  <button
                    key={item.id}
                    onClick={() => go(item.id)}
                    className={`w-full flex items-center gap-2.5 h-9 px-2 rounded-lg text-[13px] transition-colors ${
                      view === item.id ? "bg-accent-soft border border-accent-line text-accent-strong font-medium" : "text-muted hover:text-ink hover:bg-surface border border-transparent"
                    }`}
                  >
                    <Icon name={item.icon} className={view === item.id ? "text-accent" : "text-subtle"} />
                    {item.label}
                  </button>
                ))}
              </div>
            ))}
            <div className="mt-3">
              <p className="text-[12px] font-medium text-muted px-2 mb-1">Resources</p>
              <a href="https://github.com/memrohq/memro/tree/main/docs#readme" target="_blank" rel="noopener" className="w-full flex items-center gap-2.5 h-9 px-2 rounded-lg text-[13px] text-muted hover:text-ink">
                <Icon name="menu_book" className="text-subtle" /> Docs & SDKs
                <Icon name="north_east" className="ml-auto text-subtle !text-[14px]" />
              </a>
            </div>
          </nav>

          <div className="p-3 border-t border-line">
            <div className="card flex items-center gap-2.5 px-2.5 py-2">
              <div className="size-8 rounded-full bg-accent-soft border border-accent-line text-accent-strong flex items-center justify-center text-[13px] font-medium">{initial}</div>
              <div className="min-w-0 flex-1">
                <p className="text-[13px] font-medium truncate">{email || "Signed in"}</p>
                <p className="text-[11px] text-subtle truncate font-mono">{initialized ? `${agentId.slice(0, 10)}…` : "No agent yet"}</p>
              </div>
              <button className="btn-ghost size-8 px-0" onClick={signOut} aria-label="Sign out" title="Sign out"><Icon name="logout" /></button>
            </div>
          </div>
        </aside>

        {/* Main */}
        <div className="flex-1 min-w-0 flex flex-col">
          <header className="h-14 shrink-0 flex items-center gap-3 px-4 md:px-6 border-b border-line">
            <button className="md:hidden btn-ghost size-9 px-0" onClick={() => setNavOpen(true)} aria-label="Open menu"><Icon name="menu" /></button>
            <nav className="flex items-center gap-1.5 text-[13px] min-w-0">
              <span className="text-subtle hidden sm:inline">Dashboard</span>
              <Icon name="chevron_right" className="text-subtle !text-[16px] hidden sm:inline" />
              <span className="font-medium truncate">{TITLES[view]}</span>
            </nav>
            <form
              className="ml-auto relative w-full max-w-[280px] hidden sm:block"
              onSubmit={(e) => {
                e.preventDefault();
                const q = searchRef.current.value.trim();
                if (q) { setSearchSeed((s) => ({ q, n: s.n + 1 })); setView("search"); searchRef.current.value = ""; }
              }}
            >
              <Icon name="search" className="absolute left-2.5 top-1/2 -translate-y-1/2 text-subtle !text-[16px]" />
              <input ref={searchRef} className="input h-9 pl-8 pr-14 bg-panel" placeholder="Search memories…" />
              <kbd className="absolute right-2 top-1/2 -translate-y-1/2 font-mono text-[11px] text-subtle border border-line rounded px-1.5 bg-surface">⌘K</kbd>
            </form>
            <div className="ml-auto sm:ml-0">
              <StatusPill ok={health.api === "ok"} okText="Connected" badText="Offline" />
            </div>
          </header>

          <main className="flex-1 overflow-y-auto scroll-thin">
            <div className="max-w-[1400px] mx-auto p-4 md:p-6">
              {view === "connect" ? (
                <Connect apiBase={apiBase} auth={auth} code={connectCode} onDone={() => go("overview")} onCopy={copy} notify={notify} />
              ) : view === "keys" ? (
                <Keys apiBase={apiBase} auth={auth} onCopy={copy} notify={notify} />
              ) : !initialized && view !== "settings" ? (
                <p className="text-[13px] text-muted">Loading your account…</p>
              ) : view === "overview" ? (
                <Overview email={email} stats={stats} usage={usage} memories={memories} onOpenMemories={() => go("memories")} onAdd={() => setAddOpen(true)} onDelete={deleteMemory} />
              ) : view === "memories" ? (
                <Memories memories={memories} onAdd={() => setAddOpen(true)} onDelete={deleteMemory} onUpload={upload} uploading={uploading} />
              ) : view === "search" ? (
                <Search key={searchSeed.n} initialQuery={searchSeed.q} onSearch={search} />
              ) : view === "agent" ? (
                <Agent agentId={agentId} email={email} onCopy={copy} onExport={exportData} onSignOut={signOut} onOpenKeys={() => go("keys")} />
              ) : view === "activity" ? (
                <Activity events={events} />
              ) : (
                <Settings apiBaseUrl={apiBase} health={health} onSave={saveApiBase} />
              )}
            </div>
          </main>
        </div>
      </div>

      <Modal open={addOpen} title="Add memory" onClose={() => setAddOpen(false)}>
        <form onSubmit={addMemory} className="space-y-4">
          <Field label="Content">
            <textarea
              autoFocus
              required
              maxLength={10000}
              className="input h-28 py-2 resize-none"
              placeholder="e.g. The user prefers short answers with bullet points."
              value={draft.content}
              onChange={(e) => setDraft({ ...draft, content: e.target.value })}
            />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Type">
              <select className="input" value={draft.type} onChange={(e) => setDraft({ ...draft, type: e.target.value })}>
                <option value="episodic">Episodic: what happened</option>
                <option value="semantic">Semantic: what's true</option>
                <option value="procedural">Procedural: how to act</option>
                <option value="profile">Profile: who they are</option>
              </select>
            </Field>
            <Field label="Visibility">
              <select className="input" value={draft.visibility} onChange={(e) => setDraft({ ...draft, visibility: e.target.value })}>
                <option value="private">Private</option>
                <option value="shared">Shared</option>
                <option value="public">Public</option>
              </select>
            </Field>
          </div>
          <div className="flex justify-end gap-2 pt-1">
            <button type="button" className="btn-ghost" onClick={() => setAddOpen(false)}>Cancel</button>
            <button className="btn-primary" disabled={busy || !draft.content.trim()}>{busy ? "Saving…" : "Add memory"}</button>
          </div>
        </form>
      </Modal>

      {toast && (
        <div role="status" className="fixed bottom-4 right-4 z-50 card px-4 py-3 shadow-[0_12px_32px_-12px_rgba(0,0,0,0.3)] max-w-sm flex gap-3 items-start">
          <span className={`mt-1.5 size-2 rounded-full shrink-0 ${toast.error ? "bg-danger" : "bg-ok"}`} />
          <div className="min-w-0">
            <p className="text-[13px] font-medium">{toast.title}</p>
            {toast.detail && <p className="text-[12px] text-muted truncate">{toast.detail}</p>}
          </div>
        </div>
      )}
    </div>
  );
}
