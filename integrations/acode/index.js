(function() {
    const PLUGIN_ID = 'memro-mcp';
    const DEFAULT_IP = 'localhost';
    const PORT = '8001';
    const PATH = '/v1/chat/completions';

    let isGenerating = false;
    let conversationHistory = [];

    const mdToHtml = (str) => {
        if (!str) return "";
        let html = str
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/\\n/g, '\n')
            .replace(/println¡¡¡/g, 'println!'); // Clean DeepSeek artifact
        
        // Code blocks with Copy button
        html = html.replace(/```(\w+)?\n([\s\S]*?)```/g, (match, lang, code) => {
            const id = 'code-' + Math.random().toString(36).substr(2, 9);
            return `<div style="position:relative; margin:10px 0;">
                <div style="position:absolute; right:8px; top:8px; background:#444; color:#fff; padding:2px 8px; border-radius:4px; font-size:10px; cursor:pointer;" onclick="memro_copy('${id}')">Copy</div>
                <pre id="${id}" style="background:#111; padding:15px 10px 10px; border-radius:8px; overflow-x:auto; border:1px solid #333; font-family:monospace; font-size:12px; color:#c9d1d9;"><code>${code}</code></pre>
            </div>`;
        });
        
        html = html.replace(/`([^`]+)`/g, '<code style="background:#333; padding:2px 4px; border-radius:4px; font-family:monospace;">$1</code>');
        html = html.replace(/^### (.*$)/gm, '<h3 style="color:#fff; margin-top:15px; margin-bottom:5px;">$1</h3>');
        html = html.replace(/^## (.*$)/gm, '<h2 style="color:#fff; border-bottom:1px solid #333; padding-bottom:5px; margin-top:20px;">$1</h2>');
        html = html.replace(/^# (.*$)/gm, '<h1 style="color:#fff; border-bottom:1px solid #333; padding-bottom:5px;">$1</h1>');
        html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
        html = html.replace(/\n/g, '<br/>');
        return html;
    };

    window.memro_copy = (id) => {
        const el = document.getElementById(id);
        if (el) {
            const text = el.innerText;
            if (typeof cordova !== 'undefined' && cordova.plugins && cordova.plugins.clipboard) {
                cordova.plugins.clipboard.copy(text);
            } else {
                const nav = navigator.clipboard;
                if (nav) nav.writeText(text);
            }
            acode.require('toast')("Copied to clipboard");
        }
    };

    const buildFullUrl = (input) => {
        let clean = input?.trim().replace(/^https?:\/\//i, '').split('/')[0].split(':')[0];
        if (!clean || clean.length < 3) clean = DEFAULT_IP;
        return `http://${clean}:${PORT}`;
    };

    let currentBackendUrl = buildFullUrl(localStorage.getItem('memro-ai-ip'));
    let currentMode = localStorage.getItem('memro-ai-mode') || 'chat';
    let pageRef = null;
    let initialized = false;

    const TOOLS = [
        {
            type: "function",
            function: {
                name: "scaffold",
                description: "Create files",
                parameters: {
                    type: "object",
                    properties: {
                        files: {
                            type: "array",
                            items: {
                                type: "object",
                                properties: {
                                    path: { type: "string" },
                                    content: { type: "string" }
                                }
                            }
                        }
                    },
                    required: ["files"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "multi_edit",
                description: "Edit multiple files (simple string replace)",
                parameters: {
                    type: "object",
                    properties: {
                        edits: {
                            type: "array",
                            items: {
                                type: "object",
                                properties: {
                                    path: { type: "string" },
                                    find: { type: "string" },
                                    replace: { type: "string" }
                                }
                            }
                        }
                    },
                    required: ["edits"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "ast_edit",
                description: "Edit specific function/class by name",
                parameters: {
                    type: "object",
                    properties: {
                        path: { type: "string" },
                        target: { type: "string" },
                        new_code: { type: "string" }
                    },
                    required: ["path", "target", "new_code"]
                }
            }
        },
        {
            type: "function",
            function: {
                name: "delete",
                description: "Delete files",
                parameters: {
                    type: "object",
                    properties: {
                        paths: {
                            type: "array",
                            items: { type: "string" }
                        }
                    },
                    required: ["paths"]
                }
            }
        }
    ];

    const getOpenFolderUrl = () => {
        try {
            const m = acode.require('openFolder');
            const list = m?.addedFolder || m?.folders || [];
            if (Array.isArray(list) && list.length > 0) return list[0].url;
            if (list && typeof list === 'object') return Object.values(list)[0]?.url;
            return null;
        } catch(e) { return null; }
    };

    const getSystemPrompt = () => {
        const root = getOpenFolderUrl();
        const rootName = root ? root.split('/').pop() : "None";
        const modeTxt = currentMode === 'agent' 
            ? "AGENT MODE: You MUST use tools (scaffold, multi_edit, ast_edit, delete) for all file actions. Always act on files. Never just say you can do it—DO IT."
            : "CHAT MODE: Read-only. Text responses only.";
            
        return `You are Memro AI, a expert code assistant.
${modeTxt}
Current Workspace: ${rootName} (Root: ${root || 'Not set'})
All file paths must be strictly relative to the Root. No leading slashes for relative paths.

Rules:
1. Be concise.
2. In Agent mode, ALWAYS conclude your thought process by calling the appropriate tools.
3. For multi-file changes, prefer 'scaffold' or 'multi_edit'. The user approves every change before it is applied.`;
    };

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

    const reloadSidebar = () => {
        try {
            const m = acode.require('openFolder');
            const url = getOpenFolderUrl();
            if (!url) return;
            const folders = m?.addedFolder || m?.folders || [];
            let list = Array.isArray(folders) ? folders : Object.values(folders);
            const folder = list.find(f => f && (f.url === url || f.path === url));
            if (folder && typeof folder.reload === 'function') folder.reload();
        } catch(e) {}
    };

    /** The model chooses these paths: keep them relative and inside the open folder. */
    const resolveInRoot = (root, p) => {
        const parts = String(p || '').replace(/\\/g, '/').split('/').filter(x => x && x !== '.');
        if (!parts.length || parts.includes('..') || /^[a-z]+:/i.test(String(p))) {
            throw new Error(`Refusing path outside the project: ${p}`);
        }
        return root.replace(/\/+$/, '') + '/' + parts.join('/');
    };

    const escapeRegExp = (t) => String(t).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

    /** Lists what a batch of tool calls will change and asks before doing it. */
    const confirmActions = async (calls) => {
        const lines = [];
        for (const { name, args } of calls) {
            if (name === 'scaffold') (args.files || []).forEach(f => lines.push(`write  ${f.path}`));
            if (name === 'multi_edit') (args.edits || []).forEach(e => lines.push(`edit   ${e.path}`));
            if (name === 'ast_edit') lines.push(`edit   ${args.path} (${args.target})`);
            if (name === 'delete') (args.paths || []).forEach(p => lines.push(`DELETE ${p}`));
        }
        if (!lines.length) return false;
        return !!(await acode.confirm('Apply these changes?', lines.join('\n')));
    };

    const scaffoldFiles = async (files, onProgress) => {
        const fs = acode.require('fsOperation');
        const root = getOpenFolderUrl(); if(!root) throw new Error("No open folder found in Acode.");
        for (const f of files) {
            const full = resolveInRoot(root, f.path);
            await fs(full).writeFile(f.content);
            onProgress && onProgress(`📄 Created/Updated: ${f.path}`);
        }
        reloadSidebar(); return { success: true };
    };

    const applyMultiEdit = async (edits, onProgress) => {
        const fs = acode.require('fsOperation');
        const root = getOpenFolderUrl(); if(!root) throw new Error("No open folder found in Acode.");
        for (const e of edits) {
            const full = resolveInRoot(root, e.path);
            let content = await fs(full).readFile();
            if (!content.includes(e.find)) throw new Error(`Text to replace not found in ${e.path}`);
            content = content.replace(e.find, () => e.replace);
            await fs(full).writeFile(content);
            onProgress && onProgress(`📝 Edited: ${e.path}`);
        }
        reloadSidebar(); return { success: true };
    };

    const applyASTEdit = async (path, target, newCode, onProgress) => {
        const fs = acode.require('fsOperation');
        const root = getOpenFolderUrl(); if(!root) throw new Error("No open folder found in Acode.");
        const full = resolveInRoot(root, path);
        let content = await fs(full).readFile();
        const regex = new RegExp(`(function\\s+${escapeRegExp(target)}\\s*\\([^)]*\\)\\s*\\{[\\s\\S]*?\\})`, "m");
        if (!regex.test(content)) throw new Error(`Could not find ${target} in ${path}`);
        content = content.replace(regex, () => newCode);
        await fs(full).writeFile(content);
        onProgress && onProgress(`⚙️ Refactored: ${target}`);
        reloadSidebar(); return { success: true };
    };

    const deletePaths = async (paths, onProgress) => {
        const fs = acode.require('fsOperation');
        const root = getOpenFolderUrl(); if(!root) throw new Error("No open folder found in Acode.");
        for (const p of paths) {
            const full = resolveInRoot(root, p);
            await fs(full).delete();
            onProgress && onProgress(`🗑️ Deleted: ${p}`);
        }
        reloadSidebar(); return { success: true };
    };

    const buildUI = (page) => {
        if (initialized) return;
        injectStyles();
        page.innerHTML = `
            <div class="memro-page-container">
                <div style="flex:1" onclick="this.parentElement.parentElement.hide()"></div>
                <div class="memro-bottom-sheet">
                    <div style="height:24px; display:flex; justify-content:center; align-items:center;"><div style="width:40px; height:4px; background:#444; border-radius:2px;"></div></div>
                    <div style="padding:0 20px 10px; display:flex; justify-content:space-between; align-items:center;">
                        <span style="font-weight:600; color:#fff;">🧠 Memro Agent</span>
                        <div id="m-status" style="font-size:10px; color:#888;"></div>
                        <div style="display:flex; gap:8px; align-items:center;">
                            <div id="m-clear" style="color:#ef4444; font-size:14px; cursor:pointer;" title="Clear History">🗑️</div>
                            <div id="m-mode" style="background:#333; padding:2px 10px; border-radius:12px; font-size:11px; color:#fff; cursor:pointer;" title="Mode Toggle">
                                ${currentMode === 'agent' ? '🤖 Agent' : '💬 Chat'}
                            </div>
                            <span id="m-config" style="color:#3b82f6; font-size:14px; cursor:pointer;" title="Set IP">⚙️</span>
                            <span style="color:#888; font-size:24px; cursor:pointer; line-height:1;" onclick="this.closest('.page').hide()">&times;</span>
                        </div>
                    </div>
                    <div id="m-log" class="memro-log"></div>
                    <div style="padding:15px; background:#1a1a1a; display:flex; gap:10px; align-items:center; border-top:1px solid #333;">
                        <textarea id="m-in" placeholder="Ask Memro to create or edit..." rows="1" style="flex:1; background:#2a2a2a; color:#fff; border:1px solid #444; padding:10px 15px; border-radius:20px; outline:none; resize:none; font-size:14px;"></textarea>
                        <button id="m-send" style="background:#3b82f6; color:#fff; border:none; width:40px; height:40px; border-radius:50%; display:flex; align-items:center; justify-content:center;">➤</button>
                    </div>
                </div>
            </div>
        `;

        const log = page.querySelector('#m-log');
        const input = page.querySelector('#m-in');
        const sendBtn = page.querySelector('#m-send');
        const statusEl = page.querySelector('#m-status');
        const configBtn = page.querySelector('#m-config');
        const modeBtn = page.querySelector('#m-mode');
        const clearBtn = page.querySelector('#m-clear');

        const addMsg = (role, text) => {
            const m = document.createElement('div'); m.className = `memro-msg ${role}`;
            if (role === 'ai' || role === 'system-note') m.innerHTML = mdToHtml(text);
            else m.innerText = text;
            log.appendChild(m); log.scrollTop = log.scrollHeight;
            return m;
        };

        modeBtn.onclick = () => {
            currentMode = currentMode === 'agent' ? 'chat' : 'agent';
            localStorage.setItem('memro-ai-mode', currentMode);
            modeBtn.innerHTML = currentMode === 'agent' ? '🤖 Agent' : '💬 Chat';
            addMsg('system-note', `🔄 Mode set to **${currentMode === 'agent' ? 'Agent' : 'Chat'}**${currentMode === 'agent' ? ' (file changes ask before they are applied)' : ''}`);
        };

        clearBtn.onclick = () => {
            conversationHistory = [];
            log.innerHTML = '';
            addMsg('system-note', '🗑️ Conversation cleared');
        };

        configBtn.onclick = async () => {
            let val = await acode.prompt("IP Address", localStorage.getItem('memro-ai-ip') || DEFAULT_IP, "text");
            if (val) {
                currentBackendUrl = buildFullUrl(val); localStorage.setItem('memro-ai-ip', val.trim());
                acode.alert("Synced", `Using: ${currentBackendUrl}`);
            }
        };

        input.onkeydown = (e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendBtn.click(); } };

        sendBtn.onclick = async () => {
            const val = input.value.trim(); if (!val || isGenerating) return;
            input.value = ''; conversationHistory.push({ role: 'user', content: val });
            addMsg('user', val); const aiMsg = addMsg('ai', 'Thinking...');
            isGenerating = true; statusEl.textContent = 'Thinking...';

            try {
                let iterations = 0;
                let activeMessages = [{ role: 'system', content: getSystemPrompt() }, ...conversationHistory];

                while (iterations < 6) {
                    const res = await fetch(`${currentBackendUrl}${PATH}`, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({
                            messages: activeMessages,
                            tools: currentMode === 'agent' ? TOOLS : undefined,
                            tool_choice: currentMode === 'agent' ? 'auto' : undefined
                        })
                    });
                    if (!res.ok) throw new Error(`HTTP ${res.status}`);
                    const data = await res.json();
                    const msg = data.choices[0].message;
                    activeMessages.push(msg);

                    // Only structured tool calls change files. Text that merely looks
                    // like an action is shown, never executed.
                    if (msg.content) aiMsg.innerHTML = mdToHtml(msg.content);

                    if (currentMode !== 'agent' || !msg.tool_calls || msg.tool_calls.length === 0) break;

                    const calls = msg.tool_calls.map(t => {
                        let args = {};
                        try { args = JSON.parse(t.function.arguments || '{}'); } catch (e) { /* reported below */ }
                        return { id: t.id, name: t.function.name, args };
                    });
                    const sm = addMsg('system-note', `🛠️ ${calls.length} tool call(s) proposed`);
                    const note = (m) => { sm.appendChild(document.createElement('br')); sm.appendChild(document.createTextNode(m)); };
                    const approved = await confirmActions(calls);
                    if (!approved) note('Not applied.');
                    for (const { id, name, args } of calls) {
                        let result = { success: false, error: 'The user declined these changes.' };
                        if (approved) {
                            try {
                                if (name === 'scaffold') result = await scaffoldFiles(args.files || [], note);
                                else if (name === 'multi_edit') result = await applyMultiEdit(args.edits || [], note);
                                else if (name === 'ast_edit') result = await applyASTEdit(args.path, args.target, args.new_code, note);
                                else if (name === 'delete') result = await deletePaths(args.paths || [], note);
                                else result = { success: false, error: `Unknown tool ${name}` };
                            } catch(e) { result = { success: false, error: e.message }; note(`⚠️ ${e.message}`); }
                        }
                        activeMessages.push({ role: 'tool', tool_call_id: id, name, content: JSON.stringify(result) });
                    }
                    if (!approved) break;
                    iterations++;
                }
                conversationHistory = activeMessages.filter(m => m.role !== 'system' && m.role !== 'tool' && !m.tool_calls);
            } catch (err) { aiMsg.textContent = `Error: ${err.message}`; } finally { isGenerating = false; statusEl.textContent = ''; }
        };
        initialized = true;
    };

    if (typeof acode !== 'undefined') {
        acode.setPluginInit(PLUGIN_ID, (bu, $page) => {
            pageRef = $page;
            setInterval(() => {
                if (document.getElementById('m-side-btn')) return;
                const pz = document.querySelector('.icon.extension') || document.querySelector('.puzzle');
                if (pz?.parentElement) {
                    const btn = document.createElement('div'); btn.id = 'm-side-btn';
                    btn.style.cssText = 'width:100%;height:45px;display:flex;align-items:center;justify-content:center;cursor:pointer;';
                    btn.innerHTML = `<span>🧠</span>`; btn.onclick = (e) => { e.stopPropagation(); if (pageRef) { buildUI(pageRef); pageRef.show(); } };
                    pz.parentElement.prepend(btn);
                }
            }, 1000);
        });
    }
})();