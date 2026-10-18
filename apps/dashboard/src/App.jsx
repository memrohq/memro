import { useState, useEffect } from "react";
import nacl from "tweetnacl";
import { decodeUTF8 } from "tweetnacl-util";
import Login from "./components/Login";

const toHex = (bytes) => Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('');
const fromHex = (hex) => {
  if (!hex || typeof hex !== 'string' || hex.length % 2 !== 0) return new Uint8Array(0);
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < hex.length; i += 2) {
    const val = parseInt(hex.slice(i, i + 2), 16);
    if (isNaN(val)) return new Uint8Array(0);
    bytes[i / 2] = val;
  }
  return bytes;
};

const getAuthHeaders = (agentId, privateKeyHex, bodyStr, sessionToken) => {
  if (sessionToken) {
    return {
      "Authorization": `Bearer ${sessionToken}`,
      "Content-Type": "application/json"
    };
  }

  const privateKeyBytes = fromHex(privateKeyHex);
  const timestamp = Math.floor(Date.now() / 1000).toString();
  const payload = new TextEncoder().encode(timestamp + bodyStr);

  if (privateKeyBytes.length === 32) {
    // If we only have the seed, expand it to the full secret key tweetnacl expects
    const expanded = nacl.sign.keyPair.fromSeed(privateKeyBytes).secretKey;
    const signature = nacl.sign.detached(payload, expanded);
    return {
      "X-Agent-Id": agentId,
      "X-Signature": toHex(signature),
      "X-Timestamp": timestamp,
      "Content-Type": "application/json"
    };
  }
  
  if (privateKeyBytes.length !== 64) {
    throw new Error(`Invalid private key length: ${privateKeyBytes.length} bytes (expected 64)`);
  }

  const signature = nacl.sign.detached(payload, privateKeyBytes);
  return {
    "X-Agent-Id": agentId,
    "X-Signature": toHex(signature),
    "X-Timestamp": timestamp,
    "Content-Type": "application/json"
  };
};

const App = () => {
  const [sessionToken, setSessionToken] = useState(() => localStorage.getItem("memro_session_token") || null);
  const [apiBaseUrl, setApiBaseUrl] = useState(() => localStorage.getItem("memro_api_base") || import.meta.env.VITE_API_BASE_URL || "/api");
  const [settingsApiUrl, setSettingsApiUrl] = useState(() => localStorage.getItem("memro_api_base") || import.meta.env.VITE_API_BASE_URL || "/api");
  const [did, setDid] = useState(() => localStorage.getItem("memro_did") || "NOT_INITIALIZED");
  const [status, setStatus] = useState("DISCONNECTED");
  const [dbStatus, setDbStatus] = useState("UNKNOWN");
  const [vStoreStatus, setVStoreStatus] = useState("UNKNOWN");
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [isSearching, setIsSearching] = useState(false);
  const [memories, setMemories] = useState([]);
  const [activeView, setActiveView] = useState("memory");
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showSettingsModal, setShowSettingsModal] = useState(false);
  const [showNotificationsModal, setShowNotificationsModal] = useState(false);
  const [showProfileModal, setShowProfileModal] = useState(false);
  const [showRestoreModal, setShowRestoreModal] = useState(false);
  const [restoreForm, setRestoreForm] = useState({ did: "", pk: "" });
  const [newMemory, setNewMemory] = useState({ content: "", type: "episodic", visibility: "private" });
  const [notifications, setNotifications] = useState([]);
  const [auditLogs, setAuditLogs] = useState([]);
  const [uploadingFile, setUploadingFile] = useState(false);

  useEffect(() => {
    const checkHealth = async () => {
      try {
        const res = await fetch(`${apiBaseUrl}/health`);
        if (res.ok) {
          setStatus("CONNECTED");
          const data = await res.json();
          setDbStatus(data.db || "error");
          setVStoreStatus(data.vector_store || "unavailable");
        } else {
          setStatus("OFFLINE");
          setDbStatus("error");
          setVStoreStatus("unavailable");
        }
      } catch (e) {
        setStatus("OFFLINE");
        setDbStatus("error");
        setVStoreStatus("unavailable");
      }
    };
    checkHealth();
    const interval = setInterval(checkHealth, 5000);
    return () => clearInterval(interval);
  }, [apiBaseUrl]);

  useEffect(() => {
    if (did !== "NOT_INITIALIZED") {
      fetchMemories();
    }
  }, [did]);

  const fetchMemories = async (agentId = did) => {
    try {
      const pk = localStorage.getItem("memro_pk");
      const headers = getAuthHeaders(agentId, pk, "", sessionToken);
      const res = await fetch(`${apiBaseUrl}/memory/agent/${agentId}`, { headers });
      if (res.ok) {
        const data = await res.json();
        setMemories(data);
        addAuditLog("RECALL", `Fetched ${data.length} memories`);
      }
    } catch (e) {
      console.error("Failed to fetch memories", e);
    }
  };

  const handleFileUpload = async (event) => {
    const file = event.target.files[0];
    if (!file) return;
    if (did === "NOT_INITIALIZED") return alert("Initialize Identity first.");

    setUploadingFile(true);
    try {
      const formData = new FormData();
      const isImage = file.type.startsWith('image/');
      const isPdf = file.type === 'application/pdf';

      if (!isImage && !isPdf) {
        addNotification("Unsupported File", "Only images and PDFs are currently supported.");
        return;
      }

      const endpoint = isImage ? "/upload/image" : "/upload/document";
      const fieldName = isImage ? "image" : "document";
      formData.append(fieldName, file);

      const headers = sessionToken ? { "Authorization": `Bearer ${sessionToken}` } : { "X-Agent-Id": did };

      const res = await fetch(`${apiBaseUrl}${endpoint}`, {
        method: "POST",
        headers,
        body: formData
      });

      if (res.ok) {
        addNotification("Upload Success", `Deeply indexed ${file.name}`);
        addAuditLog("FILE_UPLOAD", `Uploaded and processed ${file.name}`);
        fetchMemories();
      } else {
        const err = await res.text();
        addNotification("Upload Error", err);
      }
    } catch (e) {
      addNotification("Error", "Multimodal ingestion failed");
    } finally {
      setUploadingFile(false);
      event.target.value = ''; // Reset input
    }
  };

  const handleSearch = async (e) => {
    e.preventDefault();
    if (!searchQuery) return fetchMemories();
    setIsSearching(true);
    try {
      const pk = localStorage.getItem("memro_pk");
      const headers = sessionToken ? { "Authorization": `Bearer ${sessionToken}` } : getAuthHeaders(did, pk, "");
      const res = await fetch(`${apiBaseUrl}/search?agent_id=${did}&query=${encodeURIComponent(searchQuery)}`, { headers });
      if (res.ok) {
        const data = await res.json();
        const resultsAsMemories = (data.results || []).map(r => ({
          id: r.id || r.memory_id || Math.random().toString(36).slice(2),
          content: r.content || "",
          memory_type: r.memory_type || "episodic",
          score: r.score,
          created_at: r.created_at || new Date().toISOString()
        }));
        setMemories(resultsAsMemories);
        addAuditLog("SEARCH", `Searched for: ${searchQuery}`);
      }
    } catch (e) {
      console.error("Search failed", e);
    } finally {
      setIsSearching(false);
    }
  };

  const generateKey = async () => {
    setLoading(true);
    try {
      const keyPair = nacl.sign.keyPair();
      const publicKey = toHex(keyPair.publicKey);
      const privateKey = toHex(keyPair.secretKey);
      
      const res = await fetch(`${apiBaseUrl}/identity`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ public_key: publicKey })
      });
      const data = await res.json();
      setDid(data.agent_id);
      localStorage.setItem("memro_did", data.agent_id);
      localStorage.setItem("memro_pk", privateKey);
      addNotification("Identity Generated", `Agent ID: ${data.agent_id.slice(0, 12)}...`);
      addAuditLog("IDENTITY_CREATE", `Generated new identity`);
    } catch (e) {
      addNotification("Error", "Failed to generate identity");
    } finally {
      setLoading(false);
    }
  };

  const handleRestoreIdentity = (e) => {
    e.preventDefault();
    const { did: inputDid, pk: inputPk } = restoreForm;
    
    // Basic validation
    if (!inputDid || inputDid.length < 10) {
      return addNotification("Error", "Invalid Agent ID format");
    }
    if (!inputPk || (inputPk.length !== 64 && inputPk.length !== 128)) {
      return addNotification("Error", "Invalid Private Key format (must be hex)");
    }
    
    try {
      // Test the key by running fromHex
      fromHex(inputPk);
    } catch {
      return addNotification("Error", "Private Key must be valid hex");
    }

    localStorage.setItem("memro_did", inputDid);
    localStorage.setItem("memro_pk", inputPk);
    setDid(inputDid);
    setShowRestoreModal(false);
    setRestoreForm({ did: "", pk: "" });
    addNotification("Identity Restored", `Agent ID: ${inputDid.slice(0, 12)}...`);
    addAuditLog("IDENTITY_RESTORE", `Restored existing identity`);
    fetchMemories(inputDid);
  };

  const handleCreateMemory = async (e) => {
    e.preventDefault();
    if (did === "NOT_INITIALIZED") return alert("Initialize Identity first.");
    setLoading(true);
    try {
      const body = {
        agent_id: did,
        content: newMemory.content,
        memory_type: newMemory.type,
        visibility: newMemory.visibility
      };
      const bodyStr = JSON.stringify(body);
      const headers = getAuthHeaders(did, localStorage.getItem("memro_pk"), bodyStr, sessionToken);
      
      const res = await fetch(`${apiBaseUrl}/memory`, {
        method: "POST",
        headers,
        body: bodyStr
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

  const wipeIdentity = () => {
    if (!confirm("Wipe local credentials? You will need to regenerate an identity.")) return;
    localStorage.removeItem("memro_did");
    localStorage.removeItem("memro_pk");
    localStorage.removeItem("memro_session_token");
    setSessionToken(null);
    setDid("NOT_INITIALIZED");
    setMemories([]);
    addNotification("Wiped", "Local credentials cleared");
  };

  const handleLogout = async () => {
    if (sessionToken) {
      try {
        await fetch(`${apiBaseUrl}/auth/logout`, {
          method: 'DELETE',
          headers: { 'Authorization': `Bearer ${sessionToken}` }
        });
      } catch (e) {}
    }
    wipeIdentity();
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

  const saveSettings = () => {
    try {
      new URL(settingsApiUrl);
    } catch {
      return addNotification("Error", "Invalid API Endpoint URL format");
    }
    localStorage.setItem("memro_api_base", settingsApiUrl);
    setApiBaseUrl(settingsApiUrl);
    setShowSettingsModal(false);
    addNotification("Settings Saved", `API Endpoint updated`);
  };

  if (!sessionToken) {
    return <Login 
      apiBaseUrl={apiBaseUrl} 
      onLoginSuccess={(token, agentId, pk, userId, email) => {
        localStorage.setItem("memro_session_token", token);
        if (agentId) {
          localStorage.setItem("memro_did", agentId);
          setDid(agentId);
        }
        if (pk) localStorage.setItem("memro_pk", pk);
        setSessionToken(token);
        addNotification("Login Successful", `Welcome to Memro, ${email}!`);
      }} 
    />;
  }

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
                  <div className={`size-2 rounded-full ${dbStatus === 'ok' ? 'bg-green-500' : 'bg-red-500'}`}></div>
                  <span className="text-[10px] text-slate-600 font-mono">Postgres: {dbStatus === 'ok' ? 'ONLINE' : 'OFFLINE'}</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className={`size-2 rounded-full ${vStoreStatus === 'ok' ? 'bg-green-500' : 'bg-yellow-500'}`}></div>
                  <span className="text-[10px] text-slate-600 font-mono">pgvector: {vStoreStatus === 'ok' ? 'ONLINE' : 'FALLBACK'}</span>
                </div>
              </div>
            </div>
            <div className="px-3 flex flex-col gap-2">
              <button
                onClick={handleLogout}
                className="w-full h-10 border border-red-500/50 text-red-500 font-bold uppercase text-[10px] tracking-widest hover:bg-red-500/10 transition-colors">
                SIGN OUT
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
                  <button onClick={wipeIdentity} className="text-[10px] text-red-500 uppercase hover:underline">Wipe Identity</button>
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
                      <span className={`text-xs font-bold ${dbStatus === 'ok' ? 'text-green-600' : 'text-red-600'}`}>
                        {dbStatus === 'ok' ? 'ONLINE' : 'OFFLINE'}
                      </span>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-xs text-slate-600">pgvector (Postgres)</span>
                      <span className={`text-xs font-bold ${vStoreStatus === 'ok' ? 'text-green-600' : 'text-yellow-600'}`}>
                        {vStoreStatus === 'ok' ? 'ONLINE' : 'FALLBACK'}
                      </span>
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
              <div className="flex flex-col h-full">
                <div className="mb-6 flex gap-4">
                  <form onSubmit={handleSearch} className="flex-1 flex gap-2">
                    <input
                      type="text"
                      placeholder="Semantic Search / Memory Recall..."
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      className="flex-1 bg-white border border-primary/30 px-4 py-2 font-mono text-xs focus:outline-none focus:border-primary shadow-inner"
                    />
                    <button
                      type="submit"
                      disabled={isSearching}
                      className="px-6 bg-primary text-white font-bold uppercase text-[10px] tracking-widest hover:bg-primary/90 transition-colors">
                      {isSearching ? "SEARCHING..." : "RECALL"}
                    </button>
                    {searchQuery && (
                      <button
                        type="button"
                        onClick={() => { setSearchQuery(""); fetchMemories(); }}
                        className="px-4 border border-primary/30 text-primary font-bold uppercase text-[10px] tracking-widest hover:bg-primary/10">
                        CLEAR
                      </button>
                    )}
                  </form>
                </div>

                <div className="flex-1 overflow-auto">
                  <table className="w-full border-collapse font-mono text-xs">
                    <thead>
                      <tr className="border-b-2 border-primary">
                        <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Fragment ID</th>
                        <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Content Hash</th>
                        <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Primitive</th>
                        <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Score / Visibility</th>
                        <th className="text-left py-3 px-4 text-primary font-bold uppercase tracking-wider">Created At</th>
                      </tr>
                    </thead>
                    <tbody>
                      {memories.map((mem) => (
                        <tr key={mem.id} className="border-b border-primary/20 hover:bg-primary/5 transition-colors">
                          <td className="py-3 px-4 text-primary font-bold">
                            <div className="flex items-center gap-2">
                              <span className="material-symbols-outlined text-[16px]">
                                {mem.content.startsWith("[IMAGE ANALYSIS]") ? "image" : 
                                 mem.content.startsWith("[DOC CHUNK]") ? "description" : "notes"}
                              </span>
                              {(mem.id || "").slice(0, 8)}...
                            </div>
                          </td>
                          <td className="py-3 px-4 text-slate-600">{(mem.content || "").slice(0, 80)}...</td>
                          <td className="py-3 px-4">
                            <span className="px-2 py-1 bg-primary/10 text-primary text-[10px] font-bold uppercase">
                              {mem.memory_type || "episodic"}
                            </span>
                          </td>
                          <td className="py-3 px-4 font-bold text-primary">
                            {mem.score ? (mem.score * 100).toFixed(0) + "% MATCH" : (mem.visibility || "PRIVATE").toUpperCase()}
                          </td>
                          <td className="py-3 px-4 text-slate-500">{mem.created_at ? new Date(mem.created_at).toLocaleDateString() : 'N/A'}</td>
                        </tr>
                      ))}
                      {memories.length === 0 && (
                        <tr>
                          <td colSpan="5" className="py-20 text-center text-slate-400">
                            {searchQuery ? "No matches found for this query" : "No memory fragments found"}
                          </td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
              </div>
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
                      onClick={() => setShowRestoreModal(true)}
                      disabled={did !== "NOT_INITIALIZED"}
                      className="w-full py-3 border-2 border-primary text-primary font-bold text-xs uppercase tracking-wider hover:bg-primary/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
                      Restore Existing Identity
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
              
              <div className="border-t-2 border-primary/10 pt-6">
                <label className="text-xs font-bold text-primary uppercase mb-2 block">Multimodal Ingestion (Image/PDF)</label>
                <div className="flex items-center justify-center w-full">
                  <label className="flex flex-col items-center justify-center w-full h-24 border-2 border-primary/30 border-dashed cursor-pointer hover:bg-primary/5 transition-colors">
                    <div className="flex flex-col items-center justify-center pt-5 pb-6">
                      <span className="material-symbols-outlined text-primary mb-2">upload_file</span>
                      <p className="text-[10px] text-primary font-bold uppercase tracking-widest">
                        {uploadingFile ? "INGESTING..." : "UPLOAD MULTIMODAL SOURCE"}
                      </p>
                    </div>
                    <input type="file" className="hidden" onChange={handleFileUpload} disabled={uploadingFile} />
                  </label>
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

      {/* Restore Identity Modal */}
      {showRestoreModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowRestoreModal(false)}>
          <div className="bg-white border-2 border-primary w-full max-w-2xl p-8" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-2xl font-black text-primary uppercase mb-2">Restore Identity</h3>
            <p className="text-sm text-slate-500 font-mono mb-6">Enter your existing Agent ID and Private Key to resume your session.</p>
            <form onSubmit={handleRestoreIdentity} className="space-y-6">
              <div>
                <label className="text-xs font-bold text-primary uppercase mb-2 block">Agent ID (DID)</label>
                <input
                  type="text"
                  value={restoreForm.did}
                  onChange={(e) => setRestoreForm({ ...restoreForm, did: e.target.value })}
                  placeholder="e.g. 8e24b47823fc..."
                  className="w-full bg-slate-50 border border-primary/30 p-3 font-mono text-xs focus:outline-none focus:border-primary shadow-inner"
                  required
                />
              </div>

              <div>
                <label className="text-xs font-bold text-primary uppercase mb-2 block">Private Key (Hex)</label>
                <input
                  type="password"
                  value={restoreForm.pk}
                  onChange={(e) => setRestoreForm({ ...restoreForm, pk: e.target.value })}
                  placeholder="Paste 64 or 128 character hex seed/key"
                  className="w-full bg-slate-50 border border-primary/30 p-3 font-mono text-xs focus:outline-none focus:border-primary shadow-inner"
                  required
                />
              </div>

              <div className="flex gap-4 pt-4">
                <button
                  type="submit"
                  className="flex-1 py-4 bg-primary text-white font-bold uppercase tracking-widest hover:bg-primary/90 transition-colors">
                  RESTORE SESSION
                </button>
                <button
                  type="button"
                  onClick={() => setShowRestoreModal(false)}
                  className="px-8 border border-primary text-primary font-bold uppercase tracking-widest hover:bg-primary/10 transition-colors">
                  CANCEL
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Settings Modal */}
      {showSettingsModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => {
          setShowSettingsModal(false);
          setSettingsApiUrl(apiBaseUrl); // Reset to current on close
        }}>
          <div className="bg-white border-2 border-primary w-full max-w-2xl p-8" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-2xl font-black text-primary uppercase mb-6">Settings</h3>
            <div className="space-y-6">
              <div>
                <label className="text-xs font-bold text-primary uppercase mb-2 block">API Endpoint</label>
                <input
                  type="url"
                  value={settingsApiUrl}
                  onChange={(e) => setSettingsApiUrl(e.target.value)}
                  className="w-full border-2 border-primary/30 p-3 font-mono text-sm focus:border-primary outline-none"
                  placeholder="e.g. http://localhost:8081"
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
              <div className="flex gap-4 pt-4">
                <button
                  onClick={saveSettings}
                  className="flex-1 py-3 bg-primary text-white font-bold text-xs uppercase tracking-widest hover:bg-primary/90 transition-colors">
                  APPLY CHANGES
                </button>
                <button
                  onClick={() => {
                    setShowSettingsModal(false);
                    setSettingsApiUrl(apiBaseUrl); // Reset
                  }}
                  className="px-8 py-3 border border-primary text-primary font-bold text-xs uppercase tracking-widest hover:bg-primary/10 transition-colors">
                  CANCEL
                </button>
              </div>
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
