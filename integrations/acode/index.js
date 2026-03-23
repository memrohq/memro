/**
 * Memro AI - Project Scaffolder Edition
 * AI-driven project creation via local Qwen-Coder.
 * Fixed for Cross-Device (Mobile -> Laptop) connectivity.
 * STREAMING READY: Now supports real-time token streaming with typewriter effect.
 * MARKDOWN FIX: Proper rendering for code blocks, headers, and unescaping newlines.
 * RECURSIVE FOLDERS: Now automatically creates directories for any file path!
 */

(function() {
    const PLUGIN_ID = 'memro-mcp';
    const DEFAULT_IP = '192.168.0.111';
    const PORT = '8001';
    const PATH = '/v1/chat/completions';
    
    const mdToHtml = (str) => {
        if (!str) return "";
        let html = str
            .replace(/\\n/g, '\n')
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');
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
        if (!input || input.trim() === "" || input.trim().toLowerCase() === "http" || input.trim().toLowerCase() === "https") {
            return `http://${DEFAULT_IP}:${PORT}${PATH}`;
        }
        let clean = input.trim().replace(/^https?:\/\//i, '').replace(/^https?:\/\//i, '').split('/')[0].split(':')[0];
        if (!clean || clean.length < 3 || clean.toLowerCase() === "http") clean = DEFAULT_IP;
        return `http://${clean}:${PORT}${PATH}`;
    };

    let currentBackendUrl = buildFullUrl(localStorage.getItem('memro-ai-ip'));
    let pageRef = null;
    let initialized = false;

    const SYSTEM_PROMPT = `You are Memro AI, a coding assistant inside Acode on Android.
You can create projects by returning a JSON block at the VERY END.
Format: {"action": "scaffold", "files": [{"path": "src/main.py", "content": "..."}]}
Ensure paths are relative. You CAN use subdirectories like 'src/utils/tool.py'.
ONLY return JSON if asked to create a project structure.`;

    const injectStyles = () => {
        if (document.getElementById('memro-styles')) return;
        const style = document.createElement('style');
        style.id = 'memro-styles';
        style.innerHTML = `
            .memro-page-container { height: 100%; display: flex; flex-direction: column; justify-content: flex-end; background: rgba(0,0,0,0.2); }
            .memro-bottom-sheet { background: #1a1a1a; border-top: 1px solid #333; border-radius: 20px 20px 0 0; height: 75%; display: flex; flex-direction: column; box-shadow: 0 -10px 40px rgba(0,0,0,0.5); animation: mSlideIn 0.3s ease-out; }
            @keyframes mSlideIn { from { transform: translateY(100%); } to { transform: translateY(0); } }
            .memro-log { flex: 1; overflow-y: auto; padding: 15px 20px; display: flex; flex-direction: column; gap: 12px; background: #121212; }
            .memro-msg { padding: 10px 14px; border-radius: 18px; font-size: 14px; max-width: 90%; line-height: 1.5; word-wrap: break-word; color: #d1d5db; }
            .memro-msg.user { background: #2563eb; color: white; align-self: flex-end; }
            .memro-msg.ai { background: #2a2a2a; align-self: flex-start; border: 1px solid #333; }
        `;
        document.head.appendChild(style);
    };

    const scaffoldFiles = async (files) => {
        try {
            const fs = acode.require('fsOperation');
            const projectManager = acode.require('projectManager');
            const currentProject = projectManager.getCurrentProject();
            if (!currentProject) throw new Error("No active folder open. Please open a folder first!");

            const rootUrl = currentProject.url;
            
            // Recursive directory creator
            const ensureDir = async (folderUrl) => {
                try {
                    const stats = await fs(folderUrl).exists();
                    if (!stats) await fs(folderUrl.substring(0, folderUrl.lastIndexOf('/'))).mkdir(folderUrl.split('/').pop());
                } catch (e) { /* ignore if already exists */ }
            };

            for (const file of files) {
                let parts = file.path.replace(/^\//,'').split('/');
                let currentUrl = rootUrl;
                // Create subfolders if needed
                for (let i = 0; i < parts.length - 1; i++) {
                    let folderName = parts[i];
                    currentUrl = currentUrl.endsWith('/') ? currentUrl + folderName : currentUrl + '/' + folderName;
                    await ensureDir(currentUrl);
                }
                const fileUrl = rootUrl.endsWith('/') ? rootUrl + file.path.replace(/^\//,'') : rootUrl + '/' + file.path.replace(/^\//,'');
                await fs(fileUrl).writeFile(file.content);
            }
            acode.require('sidebar').refresh();
            return { success: true };
        } catch (e) { 
            console.error("Scaffold Error", e); 
            return { success: false, error: e.message }; 
        }
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
                        <span style="font-weight:600; color:#fff;">🧠 Memro AI (TURBO)</span>
                        <div>
                           <span id="m-config" style="color:#3b82f6; font-size:12px; margin-right:15px; cursor:pointer;">Set IP</span>
                           <span style="color:#888; font-size:28px; cursor:pointer;" onclick="this.closest('.page').hide()">&times;</span>
                        </div>
                    </div>
                    <div id="m-log" class="memro-log"></div>
                    <div style="padding:15px 18px 25px; background:#1a1a1a !important; display:flex !important; gap:10px !important; align-items:flex-end !important; border-top:1px solid #222 !important; width:100% !important; box-sizing:border-box !important;">
                        <textarea id="m-in" placeholder="Ask Memro to create a project..." rows="1" style="flex:1 !important; width:100% !important; min-width:0 !important; background:#2a2a2a !important; color:#ffffff !important; border:1px solid #444 !important; padding:12px 16px !important; border-radius:25px !important; outline:none !important; resize:none !important; font-size:14px !important; max-height:100px !important; appearance:none !important; -webkit-appearance:none !important; box-sizing:border-box !important;"></textarea>
                        <button id="m-send" style="background:#3b82f6 !important; color:#fff !important; border:none !important; width:44px !important; height:44px !important; border-radius:50% !important; display:flex !important; align-items:center !important; justify-content:center !important; flex-shrink:0 !important;">➤</button>
                    </div>
                </div>
            </div>
        `;

        const log = page.querySelector('#m-log');
        const input = page.querySelector('#m-in');
        const send = page.querySelector('#m-send');
        const configBtn = page.querySelector('#m-config');

        const addMsg = (role, text) => {
            const m = document.createElement('div'); m.className = `memro-msg ${role}`;
            if (role === 'ai') m.innerHTML = mdToHtml(text);
            else m.innerText = text;
            log.appendChild(m); log.scrollTop = log.scrollHeight;
            return m;
        };

        const updateIP = async () => {
            let currentHost = currentBackendUrl.split('//')[1]?.split(':')[0] || DEFAULT_IP;
            if (currentHost.toLowerCase() === "http") currentHost = DEFAULT_IP; 
            let val = await acode.prompt("Enter Laptop IP", currentHost, "text");
            if (val) {
                currentBackendUrl = buildFullUrl(val);
                const cleanIp = val.trim().replace(/^https?:\/\//i, '').split('/')[0].split(':')[0];
                localStorage.setItem('memro-ai-ip', cleanIp);
                acode.alert("Backend Updated", `Now using: ${currentBackendUrl}`);
            }
        };

        configBtn.onclick = updateIP;

        send.onclick = async () => {
            const val = input.value.trim(); if (!val) return;
            input.value = ''; input.style.height = 'auto';
            addMsg('user', val);
            const aiMsg = addMsg('ai', 'Thinking...');

            let fullContent = "";

            try {
                const response = await fetch(currentBackendUrl, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        messages: [
                            { role: 'system', content: SYSTEM_PROMPT },
                            { role: 'user', content: val }
                        ],
                        temperature: 0.2,
                        stream: true
                    })
                });

                if (!response.ok) throw new Error(`HTTP ${response.status}`);
                
                aiMsg.innerText = "";

                const reader = response.body.getReader();
                const decoder = new TextDecoder();
                
                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    
                    const chunk = decoder.decode(value, { stream: true });
                    const lines = chunk.split('\n');
                    
                    for (const line of lines) {
                        if (line.startsWith('data: ')) {
                            const dataStr = line.replace('data: ', '').trim();
                            if (dataStr === "[DONE]") continue;
                            try {
                                const json = JSON.parse(dataStr);
                                const token = json.choices[0].delta.content || "";
                                fullContent += token;
                                aiMsg.innerHTML = mdToHtml(fullContent);
                                log.scrollTop = log.scrollHeight;
                            } catch(e) {}
                        }
                    }
                }

                const jsonMatch = fullContent.match(/\{[\s\S]*"action":\s*"scaffold"[\s\S]*\}/);
                if (jsonMatch) {
                    try {
                        const action = JSON.parse(jsonMatch[0]);
                        if (action.files) {
                            aiMsg.innerHTML += "<br/><br/><b style='color:#3b82f6;'>⚙️ [System] Creating folders & files...</b>";
                            const result = await scaffoldFiles(action.files);
                            if (result.success) {
                                aiMsg.innerHTML += "<br/>✅ <b style='color:#10b981;'>Project created successfully!</b>";
                            } else {
                                aiMsg.innerHTML += `<br/>❌ <b style='color:#ef4444;'>Failed: ${result.error}</b>`;
                            }
                        }
                    } catch(e) { console.error("Action Parse Error", e); }
                }
            } catch (err) {
                aiMsg.innerHTML = `<b>Connection Failed (Stream).</b><br/>Reason: ${err.message}<br/>Attempted: ${currentBackendUrl}`;
                console.error("Stream Error:", err);
            }
        };
        initialized = true;
    };

    const openChat = () => { if (pageRef) { buildUI(pageRef); pageRef.show(); } };

    if (typeof acode !== 'undefined') {
        acode.setPluginInit(PLUGIN_ID, (bu, $page) => {
            pageRef = $page;
            if (!localStorage.getItem('memro-ai-ip') && bu && bu.startsWith('http')) {
                try {
                    const url = new URL(bu);
                    currentBackendUrl = buildFullUrl(url.hostname);
                } catch(e) {}
            }
            setInterval(() => {
                const id = 'm-side-btn';
                if (document.getElementById(id)) return;
                const pz = document.querySelector('.icon.extension') || document.querySelector('.puzzle');
                if (pz) {
                    const par = pz.closest('div, li, aside, nav');
                    if (par && par.parentElement) {
                        const btn = document.createElement('div'); btn.id = id;
                        btn.style.cssText = 'width:100%;height:45px;display:flex;align-items:center;justify-content:center;cursor:pointer;';
                        btn.innerHTML = `<span style="font-size:22px;">🧠</span>`;
                        btn.onclick = (e) => { e.stopPropagation(); openChat(); };
                        par.parentElement.prepend(btn);
                    }
                }
            }, 1000);
            try { 
                const cmd = acode.require("commands");
                if (cmd) cmd.addCommand({ name: "memro:open", description: "Open Memro AI", exec: openChat });
            } catch(e) {}
        });
        acode.setPluginUnmount(PLUGIN_ID, () => { if (pageRef) pageRef.hide(); });
    }
})();
