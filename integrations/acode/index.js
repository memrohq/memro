/*
Memro MCP Plugin for Acode
Minimal MCP client implemented in plain JS for the Acode plugin environment.
This uses the MCP server exposed endpoints (stdio/SSE) via the memro-mcp bridge.
Note: This is a starting scaffold; adapt to Acode's exact plugin API as needed.
*/
(function () {
  const DEFAULT_BASE_URL = 'http://localhost:8081';
  const KEY_AGENT = 'memro_agent_id';
  const KEY_PRIV = 'memro_private_key';
  const KEY_BASE = 'memro_base_url';

  let agentId = (typeof localStorage !== 'undefined') ? localStorage.getItem(KEY_AGENT) : null;
  let privateKey = (typeof localStorage !== 'undefined') ? localStorage.getItem(KEY_PRIV) : null;
  let baseUrl = (typeof localStorage !== 'undefined') ? localStorage.getItem(KEY_BASE) : null;

  async function ensureCreds() {
    if (agentId && privateKey) return;
    await showCredentialDialog();
  }

  function applyModernStyles(el) {
    el.style.background = 'rgba(255, 255, 255, 0.1)';
    el.style.backdropFilter = 'blur(12px) saturate(180%)';
    el.style.webkitBackdropFilter = 'blur(12px) saturate(180%)';
    el.style.border = '1px solid rgba(255, 255, 255, 0.2)';
    el.style.borderRadius = '12px';
    el.style.boxShadow = '0 8px 32px 0 rgba(31, 38, 135, 0.37)';
    el.style.color = '#fff';
    el.style.fontFamily = 'Inter, sans-serif';
  }

  function createCredentialModal() {
    const overlay = document.createElement('div');
    overlay.style.position = 'fixed';
    overlay.style.top = 0; overlay.style.left = 0; overlay.style.right = 0; overlay.style.bottom = 0;
    overlay.style.background = 'rgba(0,0,0,0.6)';
    overlay.style.display = 'flex'; overlay.style.alignItems = 'center'; overlay.style.justifyContent = 'center';
    overlay.style.zIndex = '9999';

    const panel = document.createElement('div');
    applyModernStyles(panel);
    panel.style.padding = '24px';
    panel.style.width = '340px';
    
    panel.innerHTML = `
      <h3 style="margin-top:0; color: #60a5fa;">Memro MCP Credentials</h3>
      <div style="margin-bottom:12px;">
        <label style="font-size:12px; opacity:0.8;">Agent ID</label>
        <input id="memro-agent" style="width: 100%; background: rgba(0,0,0,0.2); border: 1px solid rgba(255,255,255,0.1); color: #fff; padding: 8px; border-radius: 6px;" />
      </div>
      <div style="margin-bottom:12px;">
        <label style="font-size:12px; opacity:0.8;">Private Key</label>
        <input id="memro-key" type="password" style="width: 100%; background: rgba(0,0,0,0.2); border: 1px solid rgba(255,255,255,0.1); color: #fff; padding: 8px; border-radius: 6px;" />
      </div>
      <div style="margin-bottom:20px;">
        <label style="font-size:12px; opacity:0.8;">Base URL</label>
        <input id="memro-base" value="${baseUrl || DEFAULT_BASE_URL}" style="width: 100%; background: rgba(0,0,0,0.2); border: 1px solid rgba(255,255,255,0.1); color: #fff; padding: 8px; border-radius: 6px;" />
      </div>
      <button id="memro-save" style="width:100%; padding: 10px; background: #3b82f6; border: none; border-radius: 6px; color: #fff; font-weight: bold; cursor: pointer;">Save Credentials</button>
    `;
    overlay.appendChild(panel);
    document.body.appendChild(overlay);
    return {
      overlay,
      saveButton: panel.querySelector('#memro-save'),
      agentInput: panel.querySelector('#memro-agent'),
      keyInput: panel.querySelector('#memro-key'),
      baseInput: panel.querySelector('#memro-base')
    };
  }

  let mcpEventSource = null;
  let mcpPostUrl = null;

  async function connectSSE() {
    if (mcpEventSource) return;
    await ensureCreds();
    
    const url = new URL(baseUrl); // baseUrl is the SSE endpoint (e.g. http://...:8080/sse)
    url.searchParams.set('agent_id', agentId);
    url.searchParams.set('token', privateKey);

    mcpEventSource = new EventSource(url.toString());
    
    mcpEventSource.addEventListener('endpoint', (e) => {
      // The server sends the specific POST endpoint for this session
      mcpPostUrl = new URL(e.data, url.origin).toString();
      console.log('Memro SSE Connected. Post URL:', mcpPostUrl);
    });

    mcpEventSource.onerror = (e) => {
      console.error('Memro SSE Error:', e);
      mcpEventSource.close();
      mcpEventSource = null;
      mcpPostUrl = null;
    };
  }

  async function callTool(name, args) {
    if (!mcpPostUrl) await connectSSE();
    
    // Wait a bit for the endpoint if we just connected
    for (let i = 0; i < 10 && !mcpPostUrl; i++) {
       await new Promise(r => setTimeout(r, 500));
    }

    if (!mcpPostUrl) throw new Error('Could not establish Memro connection');

    try {
      const body = {
        jsonrpc: '2.0',
        id: Date.now(),
        method: 'tools/call',
        params: { name, arguments: args }
      };
      
      const resp = await fetch(mcpPostUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });

      if (!resp.ok) throw new Error(`HTTP Error ${resp.status}`);
      
      // Note: In MCP-SSE, the response comes back via the EventSource,
      // but some servers might also return it in the POST response for convenience.
      // We'll check the POST response first; if empty, we'd normally wait for the SSE.
      const data = await resp.json();
      return data?.result ?? data;
    } catch (e) {
      console.error('Memro MCP Error:', e);
      throw e;
    }
  }

  window.memroMCP = {
    remember: async (content, type = 'episodic', visibility = 'private') => {
      return await callTool('remember', { content, type, visibility });
    },
    recall: async (query, limit = 5, type) => {
      return await callTool('recall', { query, limit, type });
    },
    getRecent: async (limit = 10, type) => {
      return await callTool('get_recent_memories', { limit, type });
    },
    deleteMemory: async (memory_id) => {
      return await callTool('delete_memory', { memory_id });
    },
    exportMemories: async () => {
      return await callTool('export_memories', {});
    },
    queryGraph: async (query) => {
      return await callTool('query_graph', { query });
    }
  };

  // Integration with Acode APIs
  if (typeof acode !== 'undefined') {
    acode.setRepo('memro-mcp-plugin', 'https://github.com/your-repo/memro-acode');
    
    // Add context menu item
    acode.addContextMenu('Remember to Memro', async () => {
      const activeFile = editorManager.activeFile;
      const { editor } = activeFile;
      const selectedText = editor.getSelectedText();
      if (selectedText) {
        try {
          const fileName = activeFile.filename || 'unknown';
          const contextHeader = `[Context: ${fileName}]\n\n`;
          await window.memroMCP.remember(contextHeader + selectedText);
          acode.toast('Memory stored successfully!');
        } catch (e) {
          acode.alert('Memro Error', e.message);
        }
      } else {
        acode.toast('Please select some text first.');
      }
    }, (file) => true);
  }

  // Floating Buttons (Revised)
  if (typeof document !== 'undefined') {
    const container = document.createElement('div');
    container.style.position = 'fixed'; container.style.bottom = '20px'; container.style.right = '20px';
    container.style.zIndex = '1000';
    container.style.display = 'flex'; container.style.flexDirection = 'column'; container.style.gap = '10px';

    const createBtn = (text, icon, onClick) => {
      const btn = document.createElement('div');
      btn.innerHTML = `<span style="font-size:20px;">${icon}</span>`;
      btn.title = text;
      applyModernStyles(btn);
      btn.style.width = '48px'; btn.style.height = '48px';
      btn.style.display = 'flex'; btn.style.alignItems = 'center'; btn.style.justifyContent = 'center';
      btn.style.cursor = 'pointer';
      btn.addEventListener('click', onClick);
      return btn;
    };

    container.appendChild(createBtn('Memro Settings', '⚙️', showSettingsPanel));
    container.appendChild(createBtn('Memro Memories', '🧠', showMemoryBrowserPanel));
    document.body.appendChild(container);
  }

  function showSettingsPanel() {
    const existing = document.getElementById('memro-settings-overlay');
    if (existing) { existing.remove(); return; }

    const overlay = document.createElement('div');
    overlay.id = 'memro-settings-overlay';
    overlay.style.position = 'fixed'; overlay.style.top = 0; overlay.style.left = 0; overlay.style.right = 0; overlay.style.bottom = 0;
    overlay.style.background = 'rgba(0,0,0,0.6)';
    overlay.style.zIndex = '9999';

    const panel = document.createElement('div');
    applyModernStyles(panel);
    panel.style.padding = '24px'; panel.style.width = '380px';
    panel.style.position = 'absolute'; panel.style.top = '50%'; panel.style.left = '50%'; panel.style.transform = 'translate(-50%, -50%)';
    
    panel.innerHTML = `
      <h3 style="margin-top:0; color: #60a5fa; display:flex; align-items:center; gap:8px;">
        <span>🧠</span> Memro MCP Settings
      </h3>
      <div style="margin-bottom:12px;">
        <label style="font-size:12px; opacity:0.8;">Base URL</label>
        <input id="memro-base-input" style="width:100%; background: rgba(0,0,0,0.2); border: 1px solid rgba(255,255,255,0.1); color: #fff; padding: 8px; border-radius: 6px;" value="${baseUrl || DEFAULT_BASE_URL}"/>
      </div>
      <div style="margin-bottom:12px;">
        <label style="font-size:12px; opacity:0.8;">Agent ID</label>
        <input id="memro-agent-input" style="width:100%; background: rgba(0,0,0,0.2); border: 1px solid rgba(255,255,255,0.1); color: #fff; padding: 8px; border-radius: 6px;" value="${agentId || ''}"/>
      </div>
      <div style="margin-bottom:20px;">
        <label style="font-size:12px; opacity:0.8;">Private Key</label>
        <input id="memro-key-input" style="width:100%; background: rgba(0,0,0,0.2); border: 1px solid rgba(255,255,255,0.1); color: #fff; padding: 8px; border-radius: 6px;" value="${privateKey || ''}" type="password"/>
      </div>
      <div style="display: flex; gap: 8px;">
        <button id="memro-save-btn" style="flex:2; padding: 10px; background: #3b82f6; border: none; border-radius: 6px; color: #fff; font-weight: bold; cursor: pointer;">Save</button>
        <button id="memro-close-btn" style="flex:1; padding: 10px; background: rgba(255,255,255,0.1); border: none; border-radius: 6px; color: #fff; cursor: pointer;">Close</button>
        <button id="memro-reset-btn" style="padding: 10px; background: #ef4444; border: none; border-radius: 6px; color: #fff; cursor: pointer;">Reset</button>
      </div>
    `;
    overlay.appendChild(panel);
    document.body.appendChild(overlay);

    panel.querySelector('#memro-save-btn').addEventListener('click', () => {
      baseUrl = (panel.querySelector('#memro-base-input').value).trim();
      agentId = (panel.querySelector('#memro-agent-input').value).trim();
      privateKey = (panel.querySelector('#memro-key-input').value).trim();
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem('memro_base_url', baseUrl);
        localStorage.setItem('memro_agent_id', agentId);
        localStorage.setItem('memro_private_key', privateKey);
      }
      overlay.remove();
      if (typeof acode !== 'undefined') acode.toast('Settings saved!');
    });
    panel.querySelector('#memro-close-btn').addEventListener('click', () => overlay.remove());
    panel.querySelector('#memro-reset-btn').addEventListener('click', () => {
      localStorage.clear();
      agentId = null; privateKey = null; baseUrl = null;
      overlay.remove();
    });
  }

  async function showMemoryBrowserPanel() {
    const overlay = document.createElement('div');
    overlay.style.position = 'fixed'; overlay.style.top = 0; overlay.style.left = 0; overlay.style.right = 0; overlay.style.bottom = 0;
    overlay.style.background = 'rgba(0,0,0,0.6)';
    overlay.style.zIndex = '9999';

    const panel = document.createElement('div');
    applyModernStyles(panel);
    panel.style.padding = '20px'; panel.style.width = '90%'; panel.style.maxWidth = '450px'; panel.style.maxHeight = '85%'; panel.style.overflow = 'hidden';
    panel.style.display = 'flex'; panel.style.flexDirection = 'column';
    panel.style.position = 'absolute'; panel.style.top = '50%'; panel.style.left = '50%'; panel.style.transform = 'translate(-50%, -50%)';
    
    panel.innerHTML = `
      <h3 style="margin-top:0; color: #60a5fa; display:flex; justify-content:space-between; align-items:center;">
        Memro Explorer <span id="memro-close-browser" style="cursor:pointer; opacity:0.6;">✕</span>
      </h3>
      <div style="margin-bottom:12px;">
        <input id="memro-search-input" placeholder="Search memories..." style="width:100%; background:rgba(0,0,0,0.2); border:1px solid rgba(255,255,255,0.1); color:#fff; padding:10px; border-radius:8px; outline:none;"/>
      </div>
      <div id="memro-list" style="flex:1; overflow-y:auto; margin-bottom:12px;">
        <div style="text-align:center; padding:20px; opacity:0.6;">Initializing...</div>
      </div>
      <div style="display:flex; gap:8px;">
        <button id="memro-refresh" style="flex:1; padding: 10px; background: rgba(59, 130, 246, 0.1); border: 1px solid #3b82f6; border-radius: 8px; color: #60a5fa; font-size:12px; cursor: pointer;">Refresh</button>
      </div>
    `;
    overlay.appendChild(panel);
    document.body.appendChild(overlay);

    const listEl = panel.querySelector('#memro-list');
    const searchInput = panel.querySelector('#memro-search-input');
    let searchTimeout;

    function renderCards(results) {
      if (!results || results.length === 0) {
        listEl.innerHTML = '<div style="text-align:center; padding:20px; opacity:0.5;">No results found.</div>';
        return;
      }

      listEl.innerHTML = '';
      results.forEach(res => {
        const text = res.text || '';
        const lines = text.split('\n');
        const title = lines[0].replace(/^\[Context: /, '').replace(/\]$/, '');
        const body = lines.slice(1).join('\n').trim() || text;

        const card = document.createElement('div');
        card.style.background = 'rgba(255,255,255,0.05)';
        card.style.border = '1px solid rgba(255,255,255,0.1)';
        card.style.borderRadius = '8px';
        card.style.padding = '12px';
        card.style.marginBottom = '10px';

        card.innerHTML = `
          <div style="display:flex; justify-content:space-between; font-size:10px; opacity:0.6; margin-bottom:4px;">
            <span style="color:#60a5fa;">${title || 'Memory'}</span>
            <span style="background:rgba(255,255,255,0.1); padding:2px 4px; border-radius:4px;">EPISODIC</span>
          </div>
          <div style="font-size:13px; line-height:1.4; color:rgba(255,255,255,0.9); margin-bottom:8px;">${body}</div>
          <div style="display:flex; gap:8px;">
            <button class="memro-insert" style="padding:4px 10px; font-size:11px; background:#3b82f6; border:none; border-radius:4px; color:#fff; cursor:pointer;">Insert</button>
            <button class="memro-copy" style="padding:4px 10px; font-size:11px; background:rgba(255,255,255,0.1); border:none; border-radius:4px; color:#fff; cursor:pointer;">Copy</button>
          </div>
        `;

        card.querySelector('.memro-insert').onclick = () => {
          if (typeof editorManager !== 'undefined') {
            editorManager.activeFile.editor.insert(text);
            acode.toast('Inserted!');
          }
        };

        card.querySelector('.memro-copy').onclick = () => {
          cordova.plugins.clipboard.copy(text);
          acode.toast('Copied!');
        };

        listEl.appendChild(card);
      });
    }

    async function load(query = '') {
      listEl.innerHTML = '<div style="text-align:center; padding:20px; opacity:0.6;">Syncing...</div>';
      try {
        let res;
        if (query) {
          res = await window.memroMCP.recall(query, 15);
        } else {
          res = await window.memroMCP.getRecent(15);
        }
        renderCards(res?.content || []);
      } catch (e) {
        listEl.innerHTML = `<div style="color:#ef4444; padding:20px; font-size:12px;">Error: ${e.message}</div>`;
      }
    }

    searchInput.addEventListener('input', () => {
      clearTimeout(searchTimeout);
      searchTimeout = setTimeout(() => load(searchInput.value), 400);
    });

    panel.querySelector('#memro-refresh').addEventListener('click', () => load(searchInput.value));
    panel.querySelector('#memro-close-browser').addEventListener('click', () => overlay.remove());
    
    await load();
  }
})();
