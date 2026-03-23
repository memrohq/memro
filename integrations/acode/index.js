/**
 * Memro AI - Ultimate Stable Edition
 * Native Bottom-Sheet + Surgical Sidebar Icon.
 */

(function() {
    const PLUGIN_ID = 'memro-mcp';
    let pageRef = null;
    let initialized = false;

    // --- Modern Styles ---
    const injectStyles = () => {
        if (document.getElementById('memro-styles')) return;
        const style = document.createElement('style');
        style.id = 'memro-styles';
        style.innerHTML = `
            .memro-page-container {
                height: 100%; display: flex; flex-direction: column;
                justify-content: flex-end; background: rgba(0,0,0,0.2);
            }
            .memro-backdrop { flex: 1; width: 100%; }
            .memro-bottom-sheet {
                background: #1a1a1a; border-top: 1px solid #333;
                border-radius: 20px 20px 0 0; height: 65%;
                display: flex; flex-direction: column;
                box-shadow: 0 -10px 40px rgba(0,0,0,0.5);
                animation: memroSlideUp 0.3s ease-out;
            }
            @keyframes memroSlideUp {
                from { transform: translateY(100%); }
                to { transform: translateY(0); }
            }
            .memro-chat-log {
                flex: 1; overflow-y: auto; padding: 10px 20px;
                display: flex; flex-direction: column; gap: 12px; background: #121212;
            }
            .memro-input-area {
                padding: 15px 18px 25px 18px; 
                background: #1a1a1a !important;
                display: flex !important; 
                gap: 10px !important; 
                align-items: flex-end !important; 
                border-top: 1px solid #222 !important;
                width: 100% !important;
                box-sizing: border-box !important;
            }
            textarea.memro-textarea {
                flex: 1 !important; 
                width: 100% !important;
                min-width: 0 !important;
                background-color: #2a2a2a !important; 
                color: #ffffff !important; 
                border: 1px solid #444 !important;
                padding: 12px 16px !important; 
                border-radius: 25px !important; 
                outline: none !important; 
                resize: none !important;
                font-size: 14px !important; 
                max-height: 100px !important;
                appearance: none !important; 
                -webkit-appearance: none !important;
                box-sizing: border-box !important;
            }
            textarea.memro-textarea::placeholder {
                color: #888 !important;
                background: transparent !important;
            }
            .memro-msg { padding: 10px 14px; border-radius: 18px; font-size: 14px; max-width: 85%; }
            .memro-msg.user { background: #2563eb; color: white; align-self: flex-end; }
            .memro-msg.ai { background: #2a2a2a; color: #eee; align-self: flex-start; border: 1px solid #333; }
        `;
        document.head.appendChild(style);
    };

    const buildUI = (page) => {
        if (initialized) return;
        injectStyles();
        page.innerHTML = `
            <div class="memro-page-container">
                <div class="memro-backdrop" id="memro-close-bg"></div>
                <div class="memro-bottom-sheet">
                    <div style="height:24px; display:flex; justify-content:center; align-items:center;">
                        <div style="width:40px; height:4px; background:#444; border-radius:2px;"></div>
                    </div>
                    <div style="padding:0 20px 10px; display:flex; justify-content:space-between; align-items:center;">
                        <span style="font-weight:600; color:#fff;">🧠 Memro AI</span>
                        <span id="memro-x" style="color:#888; font-size:28px; cursor:pointer;">&times;</span>
                    </div>
                    <div id="memro-chat-log" class="memro-chat-log"></div>
                    <div class="memro-input-area" style="padding:15px 18px 25px 18px !important; background:#1a1a1a !important; display:flex !important; gap:10px !important; align-items:flex-end !important; border-top:1px solid #222 !important; width:100% !important; box-sizing:border-box !important;">
                        <textarea id="memro-in" class="memro-textarea" placeholder="Message Memro..." rows="1" style="flex:1 !important; width:100% !important; min-width:0 !important; background:#2a2a2a !important; color:#ffffff !important; border:1px solid #444 !important; padding:12px 16px !important; border-radius:25px !important; outline:none !important; resize:none !important; font-size:14px !important; max-height:100px !important; appearance:none !important; -webkit-appearance:none !important; box-sizing:border-box !important;"></textarea>
                        <button id="memro-send" style="background:#3b82f6 !important; color:#fff !important; border:none !important; width:44px !important; height:44px !important; border-radius:50% !important; display:flex !important; align-items:center !important; justify-content:center !important; flex-shrink:0 !important;">➤</button>
                    </div>
                </div>
            </div>
        `;
        const log = page.querySelector('#memro-chat-log');
        const input = page.querySelector('#memro-in');
        const send = page.querySelector('#memro-send');
        
        input.oninput = function() { this.style.height='auto'; this.style.height=(this.scrollHeight)+'px'; };
        const addMsg = (role, text) => {
            const m = document.createElement('div'); m.className = `memro-msg ${role}`; m.innerText = text;
            log.appendChild(m); log.scrollTop = log.scrollHeight;
        };

        send.onclick = async () => {
            const q = input.value.trim(); if (!q) return;
            input.value = ''; input.style.height = 'auto'; addMsg('user', q);
            addMsg('ai', "Thinking...");
        };
        page.querySelector('#memro-close-bg').onclick = () => page.hide();
        page.querySelector('#memro-x').onclick = () => page.hide();
        initialized = true;
    };

    const openChat = () => {
        if (!pageRef) return;
        buildUI(pageRef);
        pageRef.show();
        setTimeout(() => pageRef.querySelector('#memro-chat-in')?.focus(), 300);
    };

    // --- Surgical Sidebar Injection ---
    const injectIcon = () => {
        const id = 'memro-side-btn';
        if (document.getElementById(id)) return;

        const sniper = setInterval(() => {
            if (document.getElementById(id)) { clearInterval(sniper); return; }
            const pz = document.querySelector('.icon.extension') || document.querySelector('.puzzle');
            if (pz) {
                const par = pz.closest('div, li, aside, nav');
                if (par && par.parentElement) {
                    const btn = document.createElement('div');
                    btn.id = id;
                    btn.style.cssText = 'width:100%;height:45px;display:flex;align-items:center;justify-content:center;cursor:pointer;';
                    btn.innerHTML = `<span style="font-size:22px;">🧠</span>`;
                    btn.onclick = (e) => { e.stopPropagation(); openChat(); };
                    par.parentElement.prepend(btn);
                    clearInterval(sniper);
                }
            }
        }, 1000);
    };

    // --- Init ---
    if (typeof acode !== 'undefined') {
        acode.setPluginInit(PLUGIN_ID, (bu, $page) => {
            pageRef = $page;
            injectIcon(); // Force the icon into the sidebar
            
            // Register Command Fallback
            try {
                const cmd = acode.require("commands");
                if (cmd) cmd.addCommand({ name: "memro:open", description: "Open Memro AI", exec: openChat });
            } catch(e) {}
        });
        acode.setPluginUnmount(PLUGIN_ID, () => { if (pageRef) pageRef.hide(); });
    }
})();
