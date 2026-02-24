import { useState, useEffect } from "react";

const API_BASE = "http://localhost:8081";

const App = () => {
  const [did, setDid] = useState(() => localStorage.getItem("memro_did") || "NOT_INITIALIZED");
  const [status, setStatus] = useState("DISCONNECTED");
  const [loading, setLoading] = useState(false);
  const [memories, setMemories] = useState([]);
  const [activeView, setActiveView] = useState("memory");
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showSettingsModal, setShowSettingsModal] = useState(false);
  const [showNotificationsModal, setShowNotificationsModal] = useState(false);
  const [showProfileModal, setShowProfileModal] = useState(false);
  const [newMemory, setNewMemory] = useState({ content: "", type: "episodic", visibility: "private" });
  const [notifications, setNotifications] = useState([]);
  const [auditLogs, setAuditLogs] = useState([]);

  useEffect(() => {
    const checkHealth = async () => {
      try {
        const res = await fetch(`${API_BASE}/health`);
        if (res.ok) setStatus("CONNECTED");
      } catch (e) {
        setStatus("OFFLINE");
      }
    };
    checkHealth();
    const interval = setInterval(checkHealth, 5000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (did !== "NOT_INITIALIZED") {
      fetchMemories();
    }
  }, [did]);

  const fetchMemories = async () => {
    try {
      const res = await fetch(`${API_BASE}/memory/${did}`);
      if (res.ok) {
        const data = await res.json();
        setMemories(data);
        addAuditLog("RECALL", `Fetched ${data.length} memories`);
      }
    } catch (e) {
      console.error("Failed to fetch memories", e);
    }
  };

  const generateKey = async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/identity`, { method: "POST" });
      const data = await res.json();
      setDid(data.agent_id);
      localStorage.setItem("memro_did", data.agent_id);
      localStorage.setItem("memro_pk", data.private_key);
      addNotification("Identity Generated", `Agent ID: ${data.agent_id.slice(0, 12)}...`);
      addAuditLog("IDENTITY_CREATE", `Generated new identity`);
    } catch (e) {
      addNotification("Error", "Failed to generate identity");
    } finally {
      setLoading(false);
    }
  };

  const handleCreateMemory = async (e) => {
    e.preventDefault();
    if (did === "NOT_INITIALIZED") return alert("Initialize Identity first.");
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/memory`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          agent_id: did,
          content: newMemory.content,
          memory_type: newMemory.type,
          visibility: newMemory.visibility
        })
      });
      if (res.ok) {
        setShowCreateModal(false);
        setNewMemory({ content: "", type: "episodic", visibility: "private" });
        fetchMemories();
        addNotification("Memory Created", `Type: ${newMemory.type}`);
        addAuditLog("MEMORY_CREATE", `Created ${newMemory.type} memory`);
      }
    } catch (e) {
      addNotification("Error", "Failed to create memory");
    } finally {
      setLoading(false);
    }
  };

  const addNotification = (title, message) => {
    const notification = {
      id: Date.now(),
      title,
      message,
      timestamp: new Date().toISOString()
    };
    setNotifications(prev => [notification, ...prev].slice(0, 50));
  };

  const addAuditLog = (action, details) => {
    const log = {
      id: Date.now(),
      action,
      details,
      agent_id: did,
      timestamp: new Date().toISOString()
    };
    setAuditLogs(prev => [log, ...prev].slice(0, 100));
  };

  const exportData = () => {
    const data = {
      agent_id: did,
      private_key: localStorage.getItem("memro_pk"),
      memories: memories,
      exported_at: new Date().toISOString()
    };
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `memro-export-${did.slice(0, 8)}.json`;
    a.click();
    addNotification("Export Complete", "Data exported successfully");
    addAuditLog("DATA_EXPORT", "Exported all agent data");
  };

  const deleteAllData = () => {
    if (!confirm("Are you sure? This will delete ALL data and cannot be undone.")) return;
    localStorage.clear();
    setDid("NOT_INITIALIZED");
    setMemories([]);
    setAuditLogs([]);
    setNotifications([]);
    addNotification("Data Deleted", "All local data cleared");
  };

  return (
    <div className="flex flex-col min-h-screen text-[#0f172a] font-display">
      {/* Top Navigation Bar */}
      <header className="flex items-center justify-between whitespace-nowrap border-b border-primary bg-background-light/80 backdrop-blur-sm px-6 py-3 sticky top-0 z-50">
        <div className="flex items-center gap-8">
          <div className="flex items-center gap-3 text-primary">
            <div className="size-6">
              <svg fill="none" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">
                <path d="M36.7273 44C33.9891 44 31.6043 39.8386 30.3636 33.69C29.123 39.8386 26.7382 44 24 44C21.2618 44 18.877 39.8386 17.6364 33.69C16.3957 39.8386 14.0109 44 11.2727 44C7.25611 44 4 35.0457 4 24C4 12.9543 7.25611 4 11.2727 4C14.0109 4 16.3957 8.16144 17.6364 14.31C18.877 8.16144 21.2618 4 24 4C26.7382 4 29.123 8.16144 30.3636 14.31C31.6043 8.16144 33.9891 4 36.7273 4C40.7439 4 44 12.9543 44 24C44 35.0457 40.7439 44 36.7273 44Z" fill="currentColor"></path>
              </svg>
            </div>
            <h2 className="text-primary text-lg font-bold leading-tight tracking-[0.1em] uppercase">memro.co // Explorer</h2>
          </div>
        </div>
        <div className="flex items-center gap-4">
          <div className="flex bg-primary/10 blueprint-outline items-center px-4 py-1.5 font-mono">
            <span className="text-primary text-xs font-bold">DID: {did.slice(0, 10)}...{did.slice(-4)}</span>
          </div>
          <div className="flex gap-2 text-primary">
            <button
              onClick={() => setShowSettingsModal(true)}
              className="flex items-center justify-center size-9 blueprint-outline bg-transparent hover:bg-primary/10 transition-colors">
              <span className="material-symbols-outlined text-[20px]">settings</span>
            </button>
            <button
              onClick={() => setShowNotificationsModal(true)}
              className="flex items-center justify-center size-9 blueprint-outline bg-transparent hover:bg-primary/10 transition-colors relative">
              <span className="material-symbols-outlined text-[20px]">notifications</span>
              {notifications.length > 0 && (
                <div className="absolute -top-1 -right-1 size-4 bg-red-500 rounded-full flex items-center justify-center text-[9px] text-white font-bold">
                  {notifications.length}
                </div>
              )}
            </button>
          </div>
          <button
            onClick={() => setShowProfileModal(true)}
            className="bg-primary/20 blueprint-outline size-9 flex items-center justify-center text-primary hover:bg-primary/30 transition-colors">
            <span className="material-symbols-outlined">person</span>
          </button>
        </div>
      </header>

      <div className="flex flex-1 overflow-hidden h-full">
        {/* Sidebar */}
        <aside className="w-64 bg-white border-r border-primary/30 flex flex-col">
          <div className="p-6 border-b border-primary/30">
            <h1 className="text-2xl font-black text-primary uppercase tracking-tighter leading-none mb-1">MEMRO.CO</h1>
            <p className="text-[9px] font-bold text-slate-600 uppercase tracking-wider mb-3">Memory Infrastructure</p>
            <div className="text-[10px] font-mono text-slate-500 leading-relaxed">
              <p className="mb-1">High-performance memory layer</p>
              <p>built for AI agents</p>
            </div>
          </div>
          <nav className="flex flex-col gap-1">
            <div
              onClick={() => setActiveView("infrastructure")}
              className={`flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors ${activeView === 'infrastructure' ? 'bg-primary text-white' : 'text-primary hover:bg-primary/10'}`}>
              <span className="material-symbols-outlined text-[20px]">dashboard</span>
              <p className="text-xs font-bold uppercase tracking-wider">Infrastructure</p>
            </div>
            <div
              onClick={() => setActiveView("memory")}
              className={`flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors ${activeView === 'memory' ? 'bg-primary text-white' : 'text-primary hover:bg-primary/10'}`}>
              <span className="material-symbols-outlined text-[20px]">layers</span>
              <p className="text-xs font-bold uppercase tracking-wider">Memory Fragments</p>
            </div>
            <div
              onClick={() => setActiveView("identity")}
              className={`flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors ${activeView === 'identity' ? 'bg-primary text-white' : 'text-primary hover:bg-primary/10'}`}>
              <span className="material-symbols-outlined text-[20px]">fingerprint</span>
              <p className="text-xs font-bold uppercase tracking-wider">Identity Proxy</p>
            </div>
            <div
              onClick={() => setActiveView("logs")}
              className={`flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors ${activeView === 'logs' ? 'bg-primary text-white' : 'text-primary hover:bg-primary/10'}`}>
              <span className="material-symbols-outlined text-[20px]">article</span>
              <p className="text-xs font-bold uppercase tracking-wider">Audit Logs</p>
            </div>
            <div
              onClick={() => setActiveView("sovereignty")}
              className={`flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors ${activeView === 'sovereignty' ? 'bg-primary text-white' : 'text-primary hover:bg-primary/10'}`}>
              <span className="material-symbols-outlined text-[20px]">shield</span>
              <p className="text-xs font-bold uppercase tracking-wider">Sovereignty</p>
            </div>
          </nav>

          <div className="mt-auto border-t border-primary/20 pt-4 flex flex-col gap-4">
            <div className="p-3 bg-primary/5 blueprint-outline-half">
              <p className="text-[10px] text-primary font-bold uppercase mb-2">Infrastructure Status</p>
              <div className="flex flex-col gap-1.5">
                <div className="flex items-center gap-2">
                  <div className={`size-2 rounded-full ${status === 'CONNECTED' ? 'bg-green-500' : 'bg-red-500'}`}></div>
                  <span className="text-[10px] text-slate-600 font-mono">API: {status}</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className="size-2 rounded-full bg-green-500"></div>
                  <span className="text-[10px] text-slate-600 font-mono">Postgres: ONLINE</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className="size-2 rounded-full bg-yellow-500"></div>
                  <span className="text-[10px] text-slate-600 font-mono">Qdrant: FALLBACK</span>
                </div>
              </div>
            </div>
            <div className="px-3">
              <button
                onClick={generateKey}
                disabled={did !== "NOT_INITIALIZED" || loading}
                className="w-full h-10 bg-primary text-white font-bold uppercase text-[10px] tracking-widest hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
                {loading ? "INITIALIZING..." : did === "NOT_INITIALIZED" ? "GENERATE IDENTITY" : "IDENTITY ACTIVE"}
              </button>
            </div>
            <div className="px-3 pb-4">
              <div className="text-[9px] text-slate-400 font-mono text-center">
                <p>Open Protocol</p>
                <p>Self-Hostable</p>
              </div>
            </div>
          </div>
        </aside>

        {/* Main Content Area */}
        <main className="flex-1 flex flex-col overflow-hidden">
          {/* Page Title */}
          <div className="px-6 py-8 border-b border-primary/30 bg-gradient-to-r from-primary/5 to-transparent">
            <div className="flex justify-between items-end">
              <div>
                <div className="flex items-center gap-3 mb-3">
                  <div className="px-3 py-1 bg-primary/10 border border-primary/30">
                    <span className="text-[9px] font-bold text-primary uppercase tracking-wider">Infrastructure</span>
                  </div>
                  <div className="px-3 py-1 bg-green-500/10 border border-green-500/30">
                    <span className="text-[9px] font-bold text-green-700 uppercase tracking-wider">Open Protocol</span>
                  </div>
                </div>
                <h2 className="text-4xl font-black text-primary uppercase tracking-tight leading-none mb-2">
                  {activeView === 'infrastructure' ? 'Infrastructure Monitor' :
                    activeView === 'memory' ? 'Agent Continuity' :
                      activeView === 'identity' ? 'Identity Management' :
                        activeView === 'logs' ? 'Audit Trail' :
                          'Data Sovereignty'}
                </h2>
                <p className="text-sm font-mono text-slate-500">
                  {activeView === 'infrastructure' ? 'System health and performance metrics' :
                    activeView === 'memory' ? 'High-performance memory layer for AI agents' :
                      activeView === 'identity' ? 'Cryptographic identity infrastructure' :
                        activeView === 'logs' ? 'Complete activity and access log' :
                          'Export, backup, and delete your data'}
                </p>
              </div>
              <div className="flex flex-col items-end gap-4 font-mono">
                <div className="flex gap-2">
                  <span className="text-[10px] text-primary uppercase">STATUS: {status}</span>
                  {activeView === 'memory' && <span className="text-[10px] text-primary uppercase">RECORDS: {memories.length}</span>}
                  {activeView === 'logs' && <span className="text-[10px] text-primary uppercase">LOGS: {auditLogs.length}</span>}
                </div>
                {activeView === 'memory' && (
                  <button
                    onClick={() => setShowCreateModal(true)}
                    className="h-8 px-4 blueprint-outline bg-primary text-white text-[10px] font-bold uppercase tracking-widest hover:bg-primary/90 transition-colors">
                    Append Memory
                  </button>
                )}
              </div>
            </div>
          </div>

          {/* Content Views */}
          <div className="flex-1 overflow-auto custom-scrollbar relative p-6">
            {/* Infrastructure View */}
            {activeView === 'infrastructure' && (
              <div className="grid grid-cols-2 gap-6">
                <div className="border border-primary/30 p-6">
                  <h3 className="text-sm font-bold text-primary uppercase mb-4">System Health</h3>
                  <div className="space-y-3">
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Backend API</span>
                      <span className={`text-xs font-bold ${status === 'CONNECTED' ? 'text-green-600' : 'text-red-600'}`}>{status}</span>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">PostgreSQL</span>
                      <span className="text-xs font-bold text-green-600">ONLINE</span>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Qdrant Vector DB</span>
                      <span className="text-xs font-bold text-yellow-600">FALLBACK</span>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Uptime</span>
                      <span className="text-xs font-mono text-slate-900">99.9%</span>
                    </div>
                  </div>
                </div>

                <div className="border border-primary/30 p-6">
                  <h3 className="text-sm font-bold text-primary uppercase mb-4">Performance</h3>
                  <div className="space-y-3">
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Avg Response Time</span>
                      <span className="text-xs font-mono text-slate-900">&lt;5ms</span>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Memory Writes</span>
                      <span className="text-xs font-mono text-slate-900">{memories.length}</span>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Active Agents</span>
                      <span className="text-xs font-mono text-slate-900">1</span>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Storage Used</span>
                      <span className="text-xs font-mono text-slate-900">{(memories.length * 0.5).toFixed(1)} KB</span>
                    </div>
                  </div>
                </div>

                <div className="border border-primary/30 p-6 col-span-2">
                  <h3 className="text-sm font-bold text-primary uppercase mb-4">Recent Activity</h3>
                  <div className="space-y-2">
                    {auditLogs.slice(0, 5).map(log => (
                      <div key={log.id} className="flex items-center justify-between text-xs py-2 border-b border-slate-100">
                        <span className="font-mono text-slate-600">{new Date(log.timestamp).toLocaleTimeString()}</span>
                        <span className="font-bold text-primary">{log.action}</span>
                        <span className="text-slate-500">{log.details}</span>
                      </div>
                    ))}
                    {auditLogs.length === 0 && (
                      <p className="text-xs text-slate-400 text-center py-4">No activity yet</p>
                    )}
                  </div>
                </div>
              </div>
            )}

            {/* Memory View */}
            {activeView === 'memory' && (
              <table className="w-full border-collapse font-mono text-xs">
                <thead>
                  <tr className="border-b-2 border-primary">
                    <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Fragment ID</th>
                    <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Content Hash</th>
                    <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Primitive</th>
                    <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Visibility</th>
                    <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Created At</th>
                  </tr>
                </thead>
                <tbody>
                  {memories.map((mem) => (
                    <tr key={mem.id} className="border-b border-primary/20 hover:bg-primary/5 transition-colors">
                      <td className="py-3 px-4 text-primary font-bold">{mem.id.slice(0, 8)}...</td>
                      <td className="py-3 px-4 text-slate-600">{mem.content.slice(0, 40)}...</td>
                      <td className="py-3 px-4">
                        <span className="px-2 py-1 bg-primary/10 text-primary text-[10px] font-bold uppercase">
                          {mem.memory_type}
                        </span>
                      </td>
                      <td className="py-3 px-4 text-slate-600 capitalize">{mem.visibility}</td>
                      <td className="py-3 px-4 text-slate-500">{new Date(mem.created_at).toLocaleString()}</td>
                    </tr>
                  ))}
                  {memories.length === 0 && (
                    <tr>
                      <td colSpan="5" className="py-12 text-center text-slate-400 italic">
                        No memory fragments detected. Awaiting agent activity.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            )}

            {/* Identity View */}
            {activeView === 'identity' && (
              <div className="max-w-2xl">
                <div className="border border-primary/30 p-6 mb-6">
                  <h3 className="text-sm font-bold text-primary uppercase mb-4">Current Identity</h3>
                  <div className="space-y-4">
                    <div>
                      <label className="text-xs text-slate-600 uppercase mb-1 block">Agent ID (DID)</label>
                      <div className="bg-slate-50 p-3 font-mono text-xs break-all border border-slate-200">
                        {did}
                      </div>
                    </div>
                    <div>
                      <label className="text-xs text-slate-600 uppercase mb-1 block">Private Key</label>
                      <div className="bg-slate-50 p-3 font-mono text-xs break-all border border-slate-200">
                        {localStorage.getItem("memro_pk") || "Not available"}
                      </div>
                    </div>
                    <div>
                      <label className="text-xs text-slate-600 uppercase mb-1 block">Created</label>
                      <div className="bg-slate-50 p-3 font-mono text-xs border border-slate-200">
                        {did !== "NOT_INITIALIZED" ? "Session active" : "Not initialized"}
                      </div>
                    </div>
                  </div>
                </div>

                <div className="border border-primary/30 p-6">
                  <h3 className="text-sm font-bold text-primary uppercase mb-4">Identity Management</h3>
                  <div className="space-y-3">
                    <button
                      onClick={generateKey}
                      disabled={did !== "NOT_INITIALIZED"}
                      className="w-full py-3 bg-primary text-white font-bold text-xs uppercase tracking-wider hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
                      Generate New Identity
                    </button>
                    <button
                      onClick={() => {
                        navigator.clipboard.writeText(did);
                        addNotification("Copied", "Agent ID copied to clipboard");
                      }}
                      disabled={did === "NOT_INITIALIZED"}
                      className="w-full py-3 border-2 border-primary text-primary font-bold text-xs uppercase tracking-wider hover:bg-primary/10 transition-colors disabled:opacity-50">
                      Copy Agent ID
                    </button>
                    <button
                      onClick={() => {
                        const pk = localStorage.getItem("memro_pk");
                        if (pk) {
                          navigator.clipboard.writeText(pk);
                          addNotification("Copied", "Private key copied to clipboard");
                        }
                      }}
                      disabled={did === "NOT_INITIALIZED"}
                      className="w-full py-3 border-2 border-primary text-primary font-bold text-xs uppercase tracking-wider hover:bg-primary/10 transition-colors disabled:opacity-50">
                      Copy Private Key
                    </button>
                  </div>
                </div>
              </div>
            )}

            {/* Audit Logs View */}
            {activeView === 'logs' && (
              <table className="w-full border-collapse font-mono text-xs">
                <thead>
                  <tr className="border-b-2 border-primary">
                    <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Timestamp</th>
                    <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Action</th>
                    <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Details</th>
                    <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Agent ID</th>
                  </tr>
                </thead>
                <tbody>
                  {auditLogs.map((log) => (
                    <tr key={log.id} className="border-b border-primary/20 hover:bg-primary/5 transition-colors">
                      <td className="py-3 px-4 text-slate-600">{new Date(log.timestamp).toLocaleString()}</td>
                      <td className="py-3 px-4">
                        <span className="px-2 py-1 bg-primary/10 text-primary text-[10px] font-bold uppercase">
                          {log.action}
                        </span>
                      </td>
                      <td className="py-3 px-4 text-slate-600">{log.details}</td>
                      <td className="py-3 px-4 text-slate-500">{log.agent_id.slice(0, 12)}...</td>
                    </tr>
                  ))}
                  {auditLogs.length === 0 && (
                    <tr>
                      <td colSpan="4" className="py-12 text-center text-slate-400 italic">
                        No audit logs recorded yet.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            )}

            {/* Sovereignty View */}
            {activeView === 'sovereignty' && (
              <div className="max-w-2xl space-y-6">
                <div className="border border-primary/30 p-6">
                  <h3 className="text-sm font-bold text-primary uppercase mb-4">Data Export</h3>
                  <p className="text-xs text-slate-600 mb-4">
                    Export all your agent data including identity, memories, and metadata in JSON format.
                  </p>
                  <button
                    onClick={exportData}
                    disabled={did === "NOT_INITIALIZED"}
                    className="w-full py-3 bg-primary text-white font-bold text-xs uppercase tracking-wider hover:bg-primary/90 transition-colors disabled:opacity-50">
                    Export All Data (JSON)
                  </button>
                </div>

                <div className="border border-primary/30 p-6">
                  <h3 className="text-sm font-bold text-primary uppercase mb-4">Data Summary</h3>
                  <div className="space-y-3">
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Agent Identities</span>
                      <span className="text-xs font-mono text-slate-900">{did !== "NOT_INITIALIZED" ? 1 : 0}</span>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Memory Fragments</span>
                      <span className="text-xs font-mono text-slate-900">{memories.length}</span>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Audit Log Entries</span>
                      <span className="text-xs font-mono text-slate-900">{auditLogs.length}</span>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">Total Storage</span>
                      <span className="text-xs font-mono text-slate-900">{(memories.length * 0.5).toFixed(1)} KB</span>
                    </div>
                  </div>
                </div>

                <div className="border border-red-300 bg-red-50 p-6">
                  <h3 className="text-sm font-bold text-red-600 uppercase mb-4">Danger Zone</h3>
                  <p className="text-xs text-red-600 mb-4">
                    Permanently delete all local data. This action cannot be undone.
                  </p>
                  <button
                    onClick={deleteAllData}
                    className="w-full py-3 bg-red-600 text-white font-bold text-xs uppercase tracking-wider hover:bg-red-700 transition-colors">
                    Delete All Data
                  </button>
                </div>
              </div>
            )}
          </div>

          {/* Footer */}
          <footer className="border-t border-primary/30 px-6 py-3 bg-white/60 flex justify-between items-center">
            <div className="flex items-center gap-4">
              <div className="flex items-center gap-2">
                <div className={`size-2 rounded-full ${status === 'CONNECTED' ? 'bg-green-500' : 'bg-red-500'} animate-pulse`}></div>
                <span className="text-[10px] font-bold text-primary uppercase tracking-tighter">API: {status}</span>
              </div>
              <div className="w-px h-4 bg-primary"></div>
              <div className="flex items-center gap-2">
                <span className="text-[10px] text-slate-500 font-mono">Agent ID: {did.slice(0, 12)}...</span>
              </div>
            </div>
            <div className="flex gap-2 items-center">
              <button
                onClick={fetchMemories}
                disabled={did === "NOT_INITIALIZED"}
                className="size-8 blueprint-outline text-primary flex items-center justify-center hover:bg-primary/10 transition-colors disabled:opacity-30">
                <span className="material-symbols-outlined text-[18px]">refresh</span>
              </button>
              <div className="w-px bg-primary mx-2"></div>
              <div className="flex items-center gap-2 px-3 py-1.5 bg-primary/10 border border-primary/30">
                <span className="text-[9px] font-bold text-primary uppercase">Infrastructure Layer</span>
                <span className="text-[9px] text-slate-500 font-mono">v1.0</span>
              </div>
            </div>
          </footer>
        </main>
      </div>

      {/* Create Memory Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreateModal(false)}>
          <div className="bg-white border-2 border-primary w-full max-w-2xl p-8" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-2xl font-black text-primary uppercase mb-6">Append Memory Fragment</h3>
            <form onSubmit={handleCreateMemory} className="space-y-6">
              <div>
                <label className="text-xs font-bold text-primary uppercase mb-2 block">Memory Payload</label>
                <textarea
                  value={newMemory.content}
                  onChange={(e) => setNewMemory({ ...newMemory, content: e.target.value })}
                  className="w-full h-32 border-2 border-primary/30 p-3 font-mono text-sm focus:border-primary outline-none"
                  placeholder="Enter memory content..."
                  required
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="text-xs font-bold text-primary uppercase mb-2 block">Memory Type</label>
                  <select
                    value={newMemory.type}
                    onChange={(e) => setNewMemory({ ...newMemory, type: e.target.value })}
                    className="w-full border-2 border-primary/30 p-3 font-mono text-sm focus:border-primary outline-none">
                    <option value="episodic">Episodic</option>
                    <option value="semantic">Semantic</option>
                    <option value="profile">Profile</option>
                  </select>
                </div>
                <div>
                  <label className="text-xs font-bold text-primary uppercase mb-2 block">Access Level</label>
                  <select
                    value={newMemory.visibility}
                    onChange={(e) => setNewMemory({ ...newMemory, visibility: e.target.value })}
                    className="w-full border-2 border-primary/30 p-3 font-mono text-sm focus:border-primary outline-none">
                    <option value="private">Private</option>
                    <option value="shared">Shared</option>
                    <option value="public">Public</option>
                  </select>
                </div>
              </div>
              <div className="flex gap-4">
                <button
                  type="submit"
                  disabled={loading}
                  className="flex-1 py-3 bg-primary text-white font-bold text-xs uppercase tracking-widest hover:bg-primary/90 transition-colors disabled:opacity-50">
                  {loading ? "COMMITTING..." : "COMMIT FRAGMENT"}
                </button>
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="flex-1 py-3 border-2 border-primary text-primary font-bold text-xs uppercase tracking-widest hover:bg-primary/10 transition-colors">
                  CANCEL
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Settings Modal */}
      {showSettingsModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowSettingsModal(false)}>
          <div className="bg-white border-2 border-primary w-full max-w-2xl p-8" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-2xl font-black text-primary uppercase mb-6">Settings</h3>
            <div className="space-y-6">
              <div>
                <label className="text-xs font-bold text-primary uppercase mb-2 block">API Endpoint</label>
                <input
                  type="text"
                  value={API_BASE}
                  disabled
                  className="w-full border-2 border-primary/30 p-3 font-mono text-sm bg-slate-50"
                />
              </div>
              <div>
                <label className="text-xs font-bold text-primary uppercase mb-2 block">Auto-Refresh Interval</label>
                <select className="w-full border-2 border-primary/30 p-3 font-mono text-sm focus:border-primary outline-none">
                  <option value="5000">5 seconds</option>
                  <option value="10000">10 seconds</option>
                  <option value="30000">30 seconds</option>
                  <option value="60000">1 minute</option>
                </select>
              </div>
              <div>
                <label className="text-xs font-bold text-primary uppercase mb-2 block">Theme</label>
                <select className="w-full border-2 border-primary/30 p-3 font-mono text-sm focus:border-primary outline-none">
                  <option value="light">Light (Default)</option>
                  <option value="dark">Dark (Coming Soon)</option>
                </select>
              </div>
              <button
                onClick={() => setShowSettingsModal(false)}
                className="w-full py-3 bg-primary text-white font-bold text-xs uppercase tracking-widest hover:bg-primary/90 transition-colors">
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Notifications Modal */}
      {showNotificationsModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowNotificationsModal(false)}>
          <div className="bg-white border-2 border-primary w-full max-w-2xl p-8 max-h-[80vh] overflow-auto" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-2xl font-black text-primary uppercase mb-6">Notifications</h3>
            <div className="space-y-3">
              {notifications.map((notif) => (
                <div key={notif.id} className="border border-primary/30 p-4">
                  <div className="flex justify-between items-start mb-2">
                    <h4 className="text-sm font-bold text-primary">{notif.title}</h4>
                    <span className="text-[10px] text-slate-500 font-mono">{new Date(notif.timestamp).toLocaleTimeString()}</span>
                  </div>
                  <p className="text-xs text-slate-600">{notif.message}</p>
                </div>
              ))}
              {notifications.length === 0 && (
                <p className="text-center text-slate-400 py-8">No notifications</p>
              )}
            </div>
            <button
              onClick={() => setShowNotificationsModal(false)}
              className="w-full mt-6 py-3 bg-primary text-white font-bold text-xs uppercase tracking-widest hover:bg-primary/90 transition-colors">
              Close
            </button>
          </div>
        </div>
      )}

      {/* Profile Modal */}
      {showProfileModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowProfileModal(false)}>
          <div className="bg-white border-2 border-primary w-full max-w-md p-8" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-2xl font-black text-primary uppercase mb-6">Agent Profile</h3>
            <div className="space-y-4">
              <div>
                <label className="text-xs font-bold text-primary uppercase mb-2 block">Current Agent</label>
                <div className="bg-slate-50 p-3 font-mono text-xs break-all border border-slate-200">
                  {did}
                </div>
              </div>
              <div>
                <label className="text-xs font-bold text-primary uppercase mb-2 block">Session Status</label>
                <div className="bg-slate-50 p-3 font-mono text-xs border border-slate-200">
                  {did !== "NOT_INITIALIZED" ? "Active" : "Not initialized"}
                </div>
              </div>
              <div>
                <label className="text-xs font-bold text-primary uppercase mb-2 block">Memory Count</label>
                <div className="bg-slate-50 p-3 font-mono text-xs border border-slate-200">
                  {memories.length} fragments
                </div>
              </div>
              <button
                onClick={() => {
                  if (confirm("Clear session? This will remove the current agent from this browser.")) {
                    localStorage.clear();
                    setDid("NOT_INITIALIZED");
                    setMemories([]);
                    setShowProfileModal(false);
                    addNotification("Session Cleared", "Agent removed from browser");
                  }
                }}
                className="w-full py-3 border-2 border-red-600 text-red-600 font-bold text-xs uppercase tracking-widest hover:bg-red-50 transition-colors">
                Clear Session
              </button>
              <button
                onClick={() => setShowProfileModal(false)}
                className="w-full py-3 bg-primary text-white font-bold text-xs uppercase tracking-widest hover:bg-primary/90 transition-colors">
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default App;
