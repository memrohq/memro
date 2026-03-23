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
                padding: 15px 20px 25px 20px; background: #1a1a1a;
                display: flex; gap: 10px; align-items: center; border-top: 1px solid #222;
            }
            .memro-textarea {
                flex: 1; background: #2a2a2a; color: #fff; border: 1px solid #333;
                padding: 12px 16px; border-radius: 25px; outline: none; resize: none;
                font-size: 14px; max-height: 100px;
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
                    <div class="memro-input-area">
                        <textarea id="memro-in" class="memro-textarea" placeholder="Message Memro..." rows="1"></textarea>
                        <button id="memro-send" style="background:#3b82f6; color:#fff; border:none; width:44px; height:44px; border-radius:50%; display:flex; align-items:center; justify-content:center;">➤</button>
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
