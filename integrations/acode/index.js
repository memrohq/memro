/**
 * Memro AI - Project Scaffolder Edition
 * AI-driven project creation via local Qwen-Coder.
 */

(function() {
    const PLUGIN_ID = 'memro-mcp';
    const BACKEND_URL = 'http://localhost:8000/v1/chat/completions';
    let pageRef = null;
    let initialized = false;

    const SYSTEM_PROMPT = `You are Memro AI, a coding assistant inside Acode.
You can create projects by returning a JSON block at the END of your message.
Format:
{"action": "scaffold", "files": [{"path": "filename.ext", "content": "..."}]}
Only use this when the user asks to create or scaffold a project.`;

    // --- Styles ---
    const injectStyles = () => {
        if (document.getElementById('memro-styles')) return;
        const style = document.createElement('style');
        style.id = 'memro-styles';
        style.innerHTML = `
            .memro-page-container { height: 100%; display: flex; flex-direction: column; justify-content: flex-end; background: rgba(0,0,0,0.2); }
            .memro-bottom-sheet { background: #1a1a1a; border-top: 1px solid #333; border-radius: 20px 20px 0 0; height: 75%; display: flex; flex-direction: column; box-shadow: 0 -10px 40px rgba(0,0,0,0.5); animation: mSlideIn 0.3s ease-out; }
            @keyframes mSlideIn { from { transform: translateY(100%); } to { transform: translateY(0); } }
            .memro-log { flex: 1; overflow-y: auto; padding: 15px 20px; display: flex; flex-direction: column; gap: 12px; background: #121212; }
            .memro-msg { padding: 10px 14px; border-radius: 18px; font-size: 14px; max-width: 85%; line-height: 1.4; }
            .memro-msg.user { background: #2563eb; color: white; align-self: flex-end; }
            .memro-msg.ai { background: #2a2a2a; color: #eee; align-self: flex-start; border: 1px solid #333; }
        `;
        document.head.appendChild(style);
    };

    // --- Acode FS Bridge ---
    const scaffoldFiles = async (files) => {
        try {
            const fs = acode.require('fsOperation');
            const projectManager = acode.require('projectManager');
            const currentProject = projectManager.getCurrentProject();
            if (!currentProject) throw new Error("No active project found.");
            
            const rootUrl = currentProject.url;
            for (const file of files) {
                const fileUrl = rootUrl.endsWith('/') ? rootUrl + file.path : rootUrl + '/' + file.path;
                // Simple write (assumes dirs exist or root-level for now)
                await fs(fileUrl).writeFile(file.content);
            }
            acode.require('sidebar').refresh();
            return true;
        } catch (e) {
            console.error("Scaffold Error:", e);
            return false;
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
                        <span style="font-weight:600; color:#fff;">🧠 Memro AI Project Scaffolder</span>
                        <span style="color:#888; font-size:28px; cursor:pointer;" onclick="this.closest('.page').hide()">&times;</span>
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

        const addMsg = (role, text) => {
            const m = document.createElement('div'); m.className = `memro-msg ${role}`; m.innerText = text;
            log.appendChild(m); log.scrollTop = log.scrollHeight;
            return m;
        };

        send.onclick = async () => {
            const val = input.value.trim(); if (!val) return;
            input.value = ''; input.style.height = 'auto';
            addMsg('user', val);
            const aiMsg = addMsg('ai', 'Thinking...');

            try {
                const response = await fetch(BACKEND_URL, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        messages: [
                            { role: 'system', content: SYSTEM_PROMPT },
                            { role: 'user', content: val }
                        ],
                        temperature: 0.2
                    })
                });
                const data = await response.json();
                const content = data.choices[0].message.content;
                aiMsg.innerText = content;

                // Check for JSON actions
                const jsonMatch = content.match(/\{[\s\S]*"action":\s*"scaffold"[\s\S]*\}/);
                if (jsonMatch) {
                    try {
                        const action = JSON.parse(jsonMatch[0]);
                        if (action.files) {
                            aiMsg.innerText += "\n\n[System] Scaffolding project...";
                            const success = await scaffoldFiles(action.files);
                            aiMsg.innerText += success ? "\n✅ Project created successfully!" : "\n❌ Error writing files.";
                        }
                    } catch(e) { console.error("Action Parse Error", e); }
                }
            } catch (err) {
                aiMsg.innerText = "Error: Could not connect to Memro Model Server. Ensure it is running on port 8000.";
            }
        };
        initialized = true;
    };

    const openChat = () => { if (pageRef) { buildUI(pageRef); pageRef.show(); } };

    // --- Sidebar + Commands ---
    if (typeof acode !== 'undefined') {
        acode.setPluginInit(PLUGIN_ID, (bu, $page) => {
            pageRef = $page;
            // Sidebar manual injection (sniper)
            setInterval(() => {
                if (document.getElementById('m-side-btn')) return;
                const pz = document.querySelector('.icon.extension') || document.querySelector('.puzzle');
                if (pz) {
                    const par = pz.closest('div, li, aside, nav');
                    if (par && par.parentElement) {
                        const btn = document.createElement('div'); btn.id = 'm-side-btn';
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
    }
})();
