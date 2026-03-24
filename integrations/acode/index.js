(function() {
    const PLUGIN_ID = 'memro-mcp';
    const DEFAULT_IP = '192.168.0.100';
    const PORT = '8001';
    const PATH = '/v1/chat/completions';

    let activeReader = null;
    let isGenerating = false;
    let conversationHistory = [];

    const mdToHtml = (str) => {
        if (!str) return "";
        let html = str
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/\\n/g, '\n');
        html = html.replace(/```(\w+)?\n([\s\S]*?)```/g, '<pre style="background:#111; padding:10px; border-radius:8px; overflow-x:auto; border:1px solid #333; margin:10px 0; font-family:monospace; font-size:12px; color:#c9d1d9;"><code>$2</code></pre>');
        html = html.replace(/`([^`]+)`/g, '<code style="background:#333; padding:2px 4px; border-radius:4px; font-family:monospace;">$1</code>');
        html = html.replace(/^### (.*$)/gm, '<h3 style="color:#fff; margin-top:15px; margin-bottom:5px;">$1</h3>');
        html = html.replace(/^## (.*$)/gm, '<h2 style="color:#fff; border-bottom:1px solid #333; padding-bottom:5px; margin-top:20px;">$1</h2>');
        html = html.replace(/^# (.*$)/gm, '<h1 style="color:#fff; border-bottom:1px solid #333; padding-bottom:5px;">$1</h1>');
        html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
        html = html.replace(/\n/g, '<br/>');
        return html;
    };

    const buildFullUrl = (input) => {
        let clean = input?.trim().replace(/^https?:\/\//i, '').split('/')[0].split(':')[0];
        if (!clean || clean.length < 3) clean = DEFAULT_IP;
        return `http://${clean}:${PORT}`;
    };

    let currentBackendUrl = buildFullUrl(localStorage.getItem('memro-ai-ip'));
    let contextFolderUrl = localStorage.getItem('memro-ai-folder-url') || null;
    let contextFolderName = localStorage.getItem('memro-ai-folder-name') || "None";
    let currentMode = localStorage.getItem('memro-ai-mode') || 'agent'; // 'agent' or 'chat'
    let pageRef = null;
    let initialized = false;

    const getSystemPrompt = () => `You are Memro AI, a friendly coding assistant for Acode on Android.
CURRENT WORKSPACE: ${contextFolderName} (All paths must be relative to this folder)

CONVERSATION RULES:
1. ALWAYS start your response with a brief, friendly greeting or explanation in plain text.
2. NEVER send only a JSON block.
3. Be clear about whether you are just talking or about to perform an action.

ACTION RULES:
- Action "scaffold": Create/edit files. {"action": "scaffold", "files": [{"path": "...", "content": "..."}]}
- Action "delete": Delete files. {"action": "delete", "paths": ["..."]}
- ONLY include JSON when actually performing the task. Put it at the VERY END.`;

    const injectStyles = () => {
        if (document.getElementById('memro-styles')) return;
        const style = document.createElement('style');
        style.id = 'memro-styles';
        style.innerHTML = `
            .memro-page-container { height:100%; display:flex; flex-direction:column; justify-content:flex-end; background:rgba(0,0,0,0.2); }
            .memro-bottom-sheet { background:#1a1a1a; border-top:1px solid #333; border-radius:20px 20px 0 0; height:76%; display:flex; flex-direction:column; box-shadow:0 -10px 40px rgba(0,0,0,0.5); animation:mSlideIn 0.3s ease-out; }
            @keyframes mSlideIn { from{transform:translateY(100%)} to{transform:translateY(0)} }
            .memro-log { flex:1; overflow-y:auto; padding:15px 20px; display:flex; flex-direction:column; gap:12px; background:#121212; }
            .memro-msg { padding:10px 14px; border-radius:18px; font-size:14px; max-width:85%; line-height:1.4; word-wrap:break-word; }
            .memro-msg.user { background:#2563eb; color:white; align-self:flex-end; }
            .memro-msg.ai { background:#2a2a2a; color:#eee; align-self:flex-start; border:1px solid #333; }
            .memro-msg.system-note { background:transparent; color:#888; align-self:flex-start; font-size:12px; padding:4px 8px; }
            .memro-file-badge { display:inline-block; background:#1a3a2a; color:#4ade80; border:1px solid #166534; border-radius:6px; padding:2px 8px; font-size:11px; font-family:monospace; margin:2px; }
        `;
        document.head.appendChild(style);
    };

    const getFolders = () => {
        try {
            const m = acode.require('openFolder');
            if (!m) return [];
            const list = m.addedFolder || m.folders || m.list || [];
            // Handle if list is an object instead of array
            let arr = Array.isArray(list) ? list : Object.values(list || {});
            
            // Filter and map to standard {url, name} format
            return arr.filter(f => f && (f.url || f.path || f.uri)).map(f => ({
                url: f.url || f.path || f.uri,
                name: f.name || f.label || (f.url || f.path || "").split('/').pop() || "Folder"
            }));
        } catch(e) { return []; }
    };

    const getOpenFolderUrl = () => {
        try {
            const folders = getFolders();
            if (contextFolderUrl) {
                const stillAdded = folders.some(f => f.url === contextFolderUrl);
                if (stillAdded) return contextFolderUrl;
            }
            // Auto-detect: If no context set but exactly 1 folder open, use it
            if (folders.length === 1) {
                const f = folders[0];
                contextFolderUrl = f.url;
                contextFolderName = f.name;
                localStorage.setItem('memro-ai-folder-url', contextFolderUrl);
                localStorage.setItem('memro-ai-folder-name', contextFolderName);
                return contextFolderUrl;
            }
            return null;
        } catch(e) { return null; }
    };

    const reloadSidebar = () => {
        try {
            const m = acode.require('openFolder');
            const url = getOpenFolderUrl();
            const folders = m?.addedFolder || m?.folders || [];
            // Find the original folder object to call reload()
            let list = Array.isArray(folders) ? folders : Object.values(folders);
            const folder = list.find(f => f && (f.url === url || f.path === url));
            if (folder && typeof folder.reload === 'function') folder.reload();
        } catch(e) {}
    };

    const scaffoldSingleFile = async (fs, rootUrl, file, onProgress) => {
        const relPath = file.path.replace(/^\//,'');
        const parts = relPath.split('/');
        const fileName = parts.pop();
        let currentUrl = rootUrl.replace(/\/$/, '');
        for (const dirName of parts) {
            const nextUrl = currentUrl + '/' + dirName;
            try {
                if (!(await fs(nextUrl).exists().catch(() => false))) {
                    await fs(currentUrl).createDirectory(dirName);
                    reloadSidebar();
                    onProgress && onProgress(`📁 Created folder: ${dirName}`);
                }
            } catch(e) {}
            currentUrl = nextUrl;
        }
        await fs(currentUrl + '/' + fileName).writeFile(file.content);
        reloadSidebar();
        onProgress && onProgress(`📄 Synced: ${relPath}`);
    };

    const scaffoldFiles = async (files, onProgress) => {
        try {
            const fs = acode.require('fsOperation');
            const rootUrl = getOpenFolderUrl();
            if (!rootUrl) throw new Error("No workspace selected. Set one in the chat header first!");
            for (const file of files) await scaffoldSingleFile(fs, rootUrl, file, onProgress);
            return { success: true, count: files.length };
        } catch(e) { return { success: false, error: e.message }; }
    };

    const deleteFiles = async (paths, onProgress) => {
        try {
            const fs = acode.require('fsOperation');
            const rootUrl = getOpenFolderUrl();
            if (!rootUrl) throw new Error("No workspace selected. Set one in the chat header first!");
            const root = rootUrl.replace(/\/$/, '');
            let deleted = 0;
            for (const relPath of paths) {
                const fullUrl = root + '/' + relPath.replace(/^\//,'');
                if (await fs(fullUrl).exists().catch(() => false)) {
                    await fs(fullUrl).delete();
                    deleted++;
                    onProgress && onProgress(`🗑️ Deleted: ${relPath}`);
                    reloadSidebar();
                }
            }
            return { success: true, count: deleted };
        } catch(e) { return { success: false, error: e.message }; }
    };

    const buildUI = (page) => {
        if (initialized) return;
        injectStyles();
        page.innerHTML = `
            <div class="memro-page-container">
                <div style="flex:1" onclick="this.parentElement.parentElement.hide()"></div>
                <div class="memro-bottom-sheet">
                    <div style="height:24px; display:flex; justify-content:center; align-items:center;"><div style="width:40px; height:4px; background:#444; border-radius:2px;"></div></div>
                    <div style="padding:0 20px 10px; display:flex; justify-content:space-between; align-items:center; gap:8px;">
                        <span style="font-weight:600; color:#fff; flex:1;">🧠 Memro AI</span>
                        <div id="m-status" style="font-size:10px; color:#888;"></div>
                        <div style="display:flex; gap:6px; align-items:center;">
                            <div id="m-mode" style="background:#333; padding:2px 8px; border-radius:10px; font-size:10px; color:#fff; cursor:pointer; border:1px solid #444;" title="Switch Mode">
                                ${currentMode === 'agent' ? '🤖 Agent' : '💬 Chat'}
                            </div>
                            <div id="m-folder" style="color:${contextFolderName === "None" ? "#ef4444" : "#10b981"}; font-size:11px; cursor:pointer; max-width:85px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; border:1px solid #444; padding:2px 8px; border-radius:10px; background:#1a1a1a;" title="Change Workspace">
                                📂 ${contextFolderName}
                            </div>
                            <span id="m-config" style="color:#3b82f6; font-size:12px; cursor:pointer; padding:4px;">⚙️</span>
                            <span style="color:#888; font-size:24px; cursor:pointer; line-height:1; padding:4px;" onclick="this.closest('.page').hide()">&times;</span>
                        </div>
                    </div>
                    <div id="m-log" class="memro-log"></div>
                    <div style="padding:15px 18px 25px; background:#1a1a1a !important; display:flex !important; gap:10px !important; align-items:flex-end !important; border-top:1px solid #222 !important;">
                        <textarea id="m-in" placeholder="Ask Memro to code..." rows="1" style="flex:1 !important; background:#2a2a2a !important; color:#fff !important; border:1px solid #444 !important; padding:12px 16px !important; border-radius:25px !important; outline:none !important; resize:none !important; font-size:14px !important; max-height:100px !important; box-sizing:border-box !important;"></textarea>
                        <button id="m-stop" style="display:none; background:#dc2626 !important; color:#fff !important; border:none; width:44px; height:44px; border-radius:50%; align-items:center; justify-content:center;">⏹</button>
                        <button id="m-send" style="background:#3b82f6 !important; color:#fff !important; border:none; width:44px; height:44px; border-radius:50%; display:flex; align-items:center; justify-content:center;">➤</button>
                    </div>
                </div>
            </div>
        `;

        const log = page.querySelector('#m-log');
        const input = page.querySelector('#m-in');
        const sendBtn = page.querySelector('#m-send');
        const stopBtn = page.querySelector('#m-stop');
        const statusEl = page.querySelector('#m-status');
        const configBtn = page.querySelector('#m-config');
        const folderBtn = page.querySelector('#m-folder');
        const modeBtn = page.querySelector('#m-mode');

        const addMsg = (role, text) => {
            const m = document.createElement('div'); m.className = `memro-msg ${role}`;
            if (role === 'ai' || role === 'system-note') m.innerHTML = mdToHtml(text);
            else m.innerText = text;
            log.appendChild(m); log.scrollTop = log.scrollHeight;
            return m;
        };

        const setGenerating = (val) => {
            isGenerating = val;
            sendBtn.style.display = val ? 'none' : 'flex';
            stopBtn.style.display = val ? 'flex' : 'none';
            statusEl.textContent = val ? 'streaming...' : '';
        };

        modeBtn.onclick = () => {
            currentMode = currentMode === 'agent' ? 'chat' : 'agent';
            localStorage.setItem('memro-ai-mode', currentMode);
            modeBtn.innerHTML = currentMode === 'agent' ? '🤖 Agent' : '💬 Chat';
            addMsg('system-note', `🔄 Switched to <b>${currentMode === 'agent' ? 'Agent Mode' : 'Chat Mode'}</b>`);
        };

        folderBtn.onclick = async () => {
             const m = acode.require('openFolder');
             let folders = getFolders();
             
             // Get current file's folder if any
             let curFolder = null;
             try {
                const active = editorManager?.activeFile;
                if (active && active.uri) {
                    const parts = active.uri.split('/');
                    parts.pop();
                    curFolder = { url: parts.join('/'), name: "Current File's Folder" };
                }
             } catch(e) {}

             const opts = [
                 ...(curFolder ? [[curFolder.url, `📍 ${curFolder.name}`]] : []),
                 ...folders.map(f => [f.url, `📁 ${f.name}`]),
                 ['__BROWSE__', '📂 Browse Device...'],
                 ['__MANUAL__', '🖋️ Enter URL Manually...'],
                 ['__CLEAR__', '🧹 Clear (Reset to None)']
             ];
             
             const res = await acode.select("Select Context Workspace", opts);
             if (!res) return;

             if (res === '__CLEAR__') {
                 contextFolderUrl = null;
                 contextFolderName = "None";
                 localStorage.removeItem('memro-ai-folder-url');
                 localStorage.removeItem('memro-ai-folder-name');
                 folderBtn.textContent = `📂 None`;
                 folderBtn.style.color = '#ef4444';
                 return;
             }

             if (res === '__MANUAL__') {
                 const url = await acode.prompt("Enter Folder URL", contextFolderUrl || "content://...", "text");
                 if (url) {
                     contextFolderUrl = url.trim();
                     contextFolderName = contextFolderUrl.split('/').pop() || "Folder";
                     localStorage.setItem('memro-ai-folder-url', contextFolderUrl);
                     localStorage.setItem('memro-ai-folder-name', contextFolderName);
                     folderBtn.textContent = `📂 ${contextFolderName}`;
                     folderBtn.style.color = '#10b981';
                     addMsg('system-note', `✅ Manual Workspace set: <b>${contextFolderName}</b>`);
                 }
                 return;
             }

             if (res === '__BROWSE__') {
                 try {
                     let picked = null;
                     // Primary: selectFolder with starting path if possible
                     if (typeof acode.selectFolder === 'function') {
                         picked = await acode.selectFolder({ title: "Select Workspace" });
                     } else {
                         const browser = acode.require('fileBrowser');
                         if (browser) {
                            // Fallback to callback-based open for better Android support
                            if (typeof browser.open === 'function') {
                                picked = await new Promise((resolve) => {
                                    browser.open('dir', (res) => resolve(res), () => resolve(null));
                                });
                            } else if (typeof browser.select === 'function') {
                                picked = await browser.select('dir');
                            }
                         }
                     }

                     if (picked && picked.url) {
                         const url = picked.url;
                         if (m && !folders.some(f => f.url === url)) m.add(url, url.split('/').pop());
                         contextFolderUrl = url;
                         contextFolderName = url.split('/').pop() || "Folder";
                         localStorage.setItem('memro-ai-folder-url', contextFolderUrl);
                         localStorage.setItem('memro-ai-folder-name', contextFolderName);
                         folderBtn.textContent = `📂 ${contextFolderName}`;
                         folderBtn.style.color = '#10b981';
                         addMsg('system-note', `✅ Workspace set: <b>${contextFolderName}</b>`);
                     }
                 } catch(e) { 
                     acode.alert("Error", `Browsing failed: ${e.message}. Use "Enter URL Manually" as a workaround.`); 
                 }
                 return;
             }

             if (res) {
                 contextFolderUrl = res;
                 const folder = folders.find(f => f.url === res) || (curFolder?.url === res ? curFolder : null);
                 contextFolderName = folder?.name || res.split('/').pop();
                 localStorage.setItem('memro-ai-folder-url', contextFolderUrl);
                 localStorage.setItem('memro-ai-folder-name', contextFolderName);
                 folderBtn.textContent = `📂 ${contextFolderName}`;
                 folderBtn.style.color = '#10b981';
                 addMsg('system-note', `✅ Workspace switched: <b>${contextFolderName}</b>`);
             }
        };

        configBtn.onclick = async () => {
            let host = currentBackendUrl.split('//')[1]?.split(':')[0] || DEFAULT_IP;
            let val = await acode.prompt("Enter Laptop IP", host, "text");
            if (val) {
                currentBackendUrl = buildFullUrl(val);
                localStorage.setItem('memro-ai-ip', val.trim());
                acode.alert("Updated", `Using: ${currentBackendUrl}`);
            }
        };

        stopBtn.onclick = () => {
            if (activeReader) { activeReader.cancel().catch(() => {}); activeReader = null; }
            setGenerating(false);
            addMsg('system-note', '⏹ Generation stopped.');
        };

        input.oninput = () => { input.style.height = 'auto'; input.style.height = Math.min(input.scrollHeight, 100) + 'px'; };
        input.onkeydown = (e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendBtn.click(); } };

        sendBtn.onclick = async () => {
            const val = input.value.trim(); if (!val || isGenerating) return;
            const originalVal = val; // Keep for sanity check
            input.value = ''; input.style.height = 'auto';
            conversationHistory.push({ role: 'user', content: val });
            addMsg('user', val);
            const aiMsg = addMsg('ai', 'Thinking...');
            setGenerating(true);

            let fullContent = "";
            try {
                // Inject Mode-specific System Instructions
            const modePrompt = currentMode === 'agent' 
                ? " [MODE: AGENT - You can perform file actions via JSON.]" 
                : " [MODE: CHAT - DO NOT EXAMINE OR CHANGE FILES. No JSON blocks. Text/code only.]";
            
            const reqMessages = [
                { role: 'system', content: getSystemPrompt() + modePrompt },
                ...conversationHistory
            ];

            const response = await fetch(`${currentBackendUrl}${PATH}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ messages: reqMessages, stream: true })
            });

                if (!response.ok) throw new Error(`HTTP ${response.status}`);
                aiMsg.innerText = "";
                activeReader = response.body.getReader();
                const decoder = new TextDecoder();
                while (true) {
                    const { done, value } = await activeReader.read();
                    if (done) break;
                    const chunk = decoder.decode(value, { stream: true });
                    for (const line of chunk.split('\n')) {
                        if (line.startsWith('data: ')) {
                            const dataStr = line.replace('data: ', '').trim();
                            if (dataStr === "[DONE]") continue;
                            try {
                                const json = JSON.parse(dataStr);
                                const token = json.choices[0].delta.content || "";
                                fullContent += token;
                                const actionIdx = fullContent.indexOf('{"action');
                                if (actionIdx !== -1) {
                                    const displayText = fullContent.slice(0, actionIdx).trim();
                                    aiMsg.innerHTML = displayText ? mdToHtml(displayText) : '<i style="color:#888;">Performing action...</i>';
                                } else if (fullContent.trim().startsWith('{')) {
                                    aiMsg.innerHTML = '<i style="color:#888;">Thinking...</i>';
                                } else {
                                    aiMsg.innerHTML = mdToHtml(fullContent);
                                }
                                log.scrollTop = log.scrollHeight;
                            } catch(e) {}
                        }
                    }
                }
                activeReader = null;
                conversationHistory.push({ role: 'assistant', content: fullContent });

                const blocks = fullContent.match(/\{[\s\S]*?"action":\s*"(?:scaffold|delete)"[\s\S]*?\}/g) || [];
                for (const block of blocks) {
                    try {
                        const action = JSON.parse(block);
                        if (action.action === 'scaffold') {
                            const sm = addMsg('system-note', `⚙️ Scaffolding ${action.files.length} file(s)...`);
                            await scaffoldFiles(action.files, (m) => sm.innerHTML += `<br/><span class="memro-file-badge">${m}</span>`);
                        }
                        if (action.action === 'delete') {
                            // SANITY CHECK: Ignore if AI says to delete the user's greeting "hi" or exactly mirrors last input
                            const cleanPaths = (action.paths || []).filter(p => p.toLowerCase() !== originalVal.toLowerCase());
                            if (cleanPaths.length === 0) continue;
                            const confirmed = await acode.confirm("Delete?", `Delete: ${cleanPaths.join(', ')}?`);
                            if (confirmed) {
                                const sm = addMsg('system-note', `🗑️ Deleting ${cleanPaths.length} item(s)...`);
                                await deleteFiles(cleanPaths, (m) => sm.innerHTML += `<br/><span class="memro-file-badge">${m}</span>`);
                            }
                        }
                    } catch(e) {}
                }
            } catch (err) { aiMsg.innerHTML = `<b>Error:</b> ${err.message}`; } finally { setGenerating(false); }
        };
        initialized = true;
    };

    const openChat = () => { if (pageRef) { buildUI(pageRef); pageRef.show(); } };

    if (typeof acode !== 'undefined') {
        acode.setPluginInit(PLUGIN_ID, (bu, $page) => {
            pageRef = $page;
            // Attempt to restore context if possible
            if (!contextFolderUrl) {
                try {
                    const m = acode.require('openFolder');
                    if (m?.addedFolder?.length > 0) {
                        contextFolderUrl = m.addedFolder[0].url;
                        contextFolderName = m.addedFolder[0].name || contextFolderUrl.split('/').pop();
                    }
                } catch(e) {}
            }
            setInterval(() => {
                if (document.getElementById('m-side-btn')) return;
                const pz = document.querySelector('.icon.extension') || document.querySelector('.puzzle');
                if (pz?.closest('div, li, aside, nav')?.parentElement) {
                    const btn = document.createElement('div'); btn.id = 'm-side-btn';
                    btn.style.cssText = 'width:100%;height:45px;display:flex;align-items:center;justify-content:center;cursor:pointer;';
                    btn.innerHTML = `<span>🧠</span>`; btn.onclick = (e) => { e.stopPropagation(); openChat(); };
                    pz.closest('div, li, aside, nav').parentElement.prepend(btn);
                }
            }, 1000);
            try { acode.require("commands").addCommand({ name: "memro:open", description: "Open Memro", exec: openChat }); } catch(e) {}
        });
        acode.setPluginUnmount(PLUGIN_ID, () => { if (pageRef) pageRef.hide(); initialized = false; });
    }
})();
