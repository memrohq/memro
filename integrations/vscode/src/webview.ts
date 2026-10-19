import { randomBytes } from 'crypto';
import * as vscode from 'vscode';

export class MemoryWebviewProvider implements vscode.WebviewViewProvider {
    public static readonly viewType = 'memro.memoryExplorer';
    private _view?: vscode.WebviewView;

    constructor(
        private readonly _extensionUri: vscode.Uri,
        private readonly _getMcpClient: () => Promise<any>
    ) {}

    public resolveWebviewView(
        webviewView: vscode.WebviewView,
        _context: vscode.WebviewViewResolveContext,
        _token: vscode.CancellationToken,
    ) {
        this._view = webviewView;

        webviewView.webview.options = {
            enableScripts: true,
            localResourceRoots: [this._extensionUri]
        };

        webviewView.webview.html = this._getHtmlForWebview(webviewView.webview);

        webviewView.webview.onDidReceiveMessage(async (data) => {
            switch (data.type) {
                case 'search': await this._handleSearch(String(data.query ?? '')); break;
                case 'getRecent': await this._handleGetRecent(); break;
                case 'getFeed': await this._handleGetFeed(); break;
                case 'insert': await this._handleInsert(data.text); break;
                case 'evolve': await this._handleEvolve(); break;
                case 'reason': await this._handleReason(typeof data.query === 'string' ? data.query : undefined); break;
                case 'intent': await this._handleLogIntent(); break;
                case 'claim': await this._handleClaim(); break;
                case 'release': await this._handleRelease(); break;
                case 'handoff': await this._handleHandoff(); break;
                case 'setSession': await this._handleSetSession(typeof data.session === 'string' ? data.session : undefined); break;
                case 'openFile': if (typeof data.path === 'string') await vscode.commands.executeCommand('memro.openFile', data.path); break;
            }
        });
    }

    public updateActiveWork(claims: any[]) {
        if (this._view) {
            this._view.webview.postMessage({ type: 'activeWork', claims });
        }
    }

    private async _handleLogIntent() {
        const intent = await vscode.window.showInputBox({ prompt: 'Declare your intent (e.g. Refactoring auth module)' });
        if (!intent) return;
        try {
            const client = await this._getMcpClient();
            await client.callTool({ name: 'log_intent', arguments: { intent } });
            vscode.window.showInformationMessage('Memro: Intent logged.');
            await this._handleGetFeed();
        } catch (e) { vscode.window.showErrorMessage('Failed to log intent: ' + e); }
    }
    private async _handleSetSession(session?: string) {
        if (!session) {
            session = (await vscode.window.showInputBox({ prompt: 'Session ID', ignoreFocusOut: true }))?.trim();
            if (!session) return;
        }
        const config = vscode.workspace.getConfiguration('memro');
        await config.update('sessionId', session, vscode.ConfigurationTarget.Global);
        vscode.window.showInformationMessage(`Memro: Switched to session ${session}`);
        await this._handleGetFeed();
    }

    private async _handleClaim() {
        try {
            await vscode.commands.executeCommand('memro.claim');
            await this._handleGetFeed();
        } catch (e) { vscode.window.showErrorMessage('Claim failed: ' + e); }
    }


    private async _handleHandoff() {
        try {
            await vscode.commands.executeCommand('memro.handoff');
            await this._handleGetFeed();
        } catch (e) { vscode.window.showErrorMessage('Handoff failed: ' + e); }
    }

    private async _handleRelease() {
        try {
            await vscode.commands.executeCommand('memro.release');
            await this._handleGetFeed();
        } catch (e) { vscode.window.showErrorMessage('Release failed: ' + e); }
    }

    private async _handleGetFeed() {
        if (!this._view) return;
        try {
            const client = await this._getMcpClient();
            const editor = vscode.window.activeTextEditor;
            const resource = editor ? vscode.workspace.asRelativePath(editor.document.uri) : undefined;
            
            const [feedRes, contextRes, workRes] = await Promise.all([
                client.callTool({ name: 'get_recent_memories', arguments: { type: 'procedural', limit: 20 } }),
                resource ? client.callTool({ name: 'get_active_context', arguments: { resource } }) : Promise.resolve({ content: [{ text: '{}' }] }),
                client.callTool({ name: 'get_active_work', arguments: {} })
            ]);

            this._view.webview.postMessage({ 
                type: 'feed', 
                items: feedRes.content || [],
                context: contextRes.content?.[0] ? JSON.parse((contextRes.content as any[])[0].text) : { claims: [] },
                work: workRes.content?.[0] ? JSON.parse((workRes.content as any[])[0].text) : { claims: [] }
            });
        } catch (e) { this._view.webview.postMessage({ type: 'error', message: String(e) }); }
    }

    /** Inserts a memory at the cursor, after the user confirms: memory content
     * is untrusted and this writes into their source. */
    private async _handleInsert(text: unknown) {
        const editor = vscode.window.activeTextEditor;
        if (!editor || typeof text !== 'string' || !text) return;
        const preview = text.length > 200 ? text.slice(0, 200) + '…' : text;
        const ok = await vscode.window.showWarningMessage(
            `Insert this memory into ${vscode.workspace.asRelativePath(editor.document.uri)}?`,
            { modal: true, detail: preview }, 'Insert');
        if (ok === 'Insert') {
            await editor.edit(editBuilder => editBuilder.insert(editor.selection.active, text));
        }
    }

    private async _handleSearch(query: string) {
        if (!this._view) return;
        try {
            const client = await this._getMcpClient();
            const res = await client.callTool({ name: 'recall', arguments: { query, limit: 10 } });
            this._view.webview.postMessage({ type: 'results', results: res.content || [] });
        } catch (e) { this._view.webview.postMessage({ type: 'error', message: String(e) }); }
    }

    private async _handleGetRecent() {
        if (!this._view) return;
        try {
            const client = await this._getMcpClient();
            const res = await client.callTool({ name: 'get_recent_memories', arguments: { limit: 10 } });
            this._view.webview.postMessage({ type: 'results', results: res.content || [] });
        } catch (e) { this._view.webview.postMessage({ type: 'error', message: String(e) }); }
    }

    private async _handleEvolve() {
        try {
            const client = await this._getMcpClient();
            await client.callTool({ name: 'mee_evolve', arguments: {} });
            vscode.window.showInformationMessage('Memro: Cognition evolved!');
            await this._handleGetRecent();
        } catch (e) { vscode.window.showErrorMessage('Evolution failed: ' + e); }
    }

    private async _handleReason(query?: string) {
        if (!this._view) return;
        const q = query || await vscode.window.showInputBox({ prompt: 'Reason about your codebase...' });
        if (!q) return;
        try {
            const client = await this._getMcpClient();
            const res = await client.callTool({ name: 'mee_reason', arguments: { query: q } });
            const content = res.content as any[];
            const data = content?.[0]?.text || '{}';
            this._view.webview.postMessage({ type: 'reasoning', data });
        } catch (e) { vscode.window.showErrorMessage('Reasoning failed: ' + e); }
    }

    private _getHtmlForWebview(webview: vscode.Webview) {
        const nonce = randomBytes(16).toString('base64');
        return `<!DOCTYPE html>
            <html lang="en">
            <head>
                <meta charset="UTF-8">
                <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; img-src ${webview.cspSource}; script-src 'nonce-${nonce}';">
                <meta name="viewport" content="width=device-width, initial-scale=1.0">
                <title>Memro DevTools</title>
                <style>
                    :root {
                        --glass-bg: rgba(255, 255, 255, 0.03);
                        --glass-border: rgba(255, 255, 255, 0.1);
                        --accent-blue: #40a9ff;
                        --accent-purple: #9254de;
                        --accent-green: #73d13d;
                        --accent-red: #ff4d4f;
                        --text-dim: rgba(255, 255, 255, 0.45);
                        --card-bg: rgba(30, 30, 35, 0.7);
                    }
                    body { font-family: var(--vscode-font-family, sans-serif); padding: 0; color: #e6edf3; background: #0d1117; margin:0; height: 100vh; overflow:hidden; display:flex; flex-direction:column; }
                    .header { padding: 12px 16px; background: rgba(0,0,0,0.4); border-bottom: 1px solid var(--glass-border); display:flex; justify-content:space-between; align-items:center; }
                    .header h2 { margin:0; font-size: 11px; color: var(--accent-blue); text-transform:uppercase; letter-spacing:1px; }
                    .tabs { display:flex; background:rgba(0,0,0,0.2); border-bottom: 1px solid var(--glass-border); position: relative; z-index: 10; }
                    .tab { flex:1; padding:10px; text-align:center; font-size:11px; cursor:pointer; color:var(--text-dim); transition:0.2s; position: relative; }
                    .tab:hover { background: rgba(255,255,255,0.05); color: white; }
                    .tab.active { color:white; background:var(--glass-bg); box-shadow: inset 0 -2px 0 var(--accent-blue); }
                    .content { flex:1; overflow-y:auto; padding:12px; display:none; }
                    .content.active { display:block; }
                    .card { background: var(--card-bg); border: 1px solid var(--glass-border); border-radius:6px; padding:12px; margin-bottom:12px; }
                    .pill { font-size:9px; padding:2px 6px; border-radius:4px; margin-right:4px; background:rgba(255,255,255,0.05); }
                    .pill-type { color:var(--accent-purple); border:1px solid rgba(146,84,222,0.3); }
                    .pill-file { color:var(--accent-blue); border:1px solid rgba(64,169,255,0.3); cursor:pointer; }
                    .btn { padding:6px 12px; font-size:11px; background:var(--glass-bg); border:1px solid var(--glass-border); color:white; border-radius:4px; cursor:pointer; width:100%; transition:0.2s; }
                    .btn:hover { background:var(--glass-border); }
                    .btn-primary { background:var(--accent-blue); color:black; font-weight:700; border:none; }
                    input { width:100%; background:rgba(0,0,0,0.3); border:1px solid var(--glass-border); border-radius:4px; padding:8px; color:white; font-size:12px; margin-bottom:12px; box-sizing:border-box; }
                    .confidence-bar { height:4px; background:rgba(255,255,255,0.1); border-radius:2px; margin-top:8px; }
                    .confidence-fill { height:100%; background:var(--accent-blue); border-radius:2px; transition:width 0.5s; }
                    .feed-event { display:flex; gap:10px; align-items:flex-start; margin-bottom:12px; padding-bottom:12px; border-bottom:1px solid rgba(255,255,255,0.03); }
                    .active-work-item { border-left: 2px solid var(--accent-blue); padding-left: 8px; margin-bottom:10px; }
                </style>
            </head>
            <body>
                <div class="header">
                    <h2>Memro Coordination OS</h2>
                    <div style="display:flex; align-items:center; gap:4px;">
                        <span style="font-size:9px; opacity:0.5;">SESSION</span>
                        <select id="sessionSelect" style="background:rgba(255,255,255,0.05); border:1px solid rgba(255,255,255,0.1); color:white; font-size:10px; border-radius:3px; padding:2px;">
                            <option value="default">Default</option>
                            <option value="team_alpha">Alpha</option>
                            <option value="team_beta">Beta</option>
                            <option value="custom">+ New</option>
                        </select>
                    </div>
                </div>
                <div class="tabs">
                    <div class="tab active" data-tab="explorer">Explorer</div>
                    <div class="tab" data-tab="inspector">Inspector</div>
                    <div class="tab" data-tab="feed">Feed</div>
                </div>
                <div id="explorer" class="content active">
                    <input type="text" id="searchInput" placeholder="Filter memories...">
                    <div id="resultsList"></div>
                    <div style="display:flex; gap:8px;">
                        <button class="btn" id="refreshBtn">Refresh</button>
                        <button class="btn btn-primary" id="evolveBtn">Evolve</button>
                    </div>
                </div>
                <div id="inspector" class="content">
                    <div style="margin-bottom:15px;">
                        <h4 style="margin:0 0 10px 0; font-size:10px; opacity:0.5; letter-spacing:1px;">CONVERSATIONAL INSPECTOR</h4>
                        <input type="text" id="inspectorQuery" placeholder="Ask about memory (e.g. travel preferences?)">
                    </div>
                    <div id="reasoningContent">
                        <div id="activeWorkList" style="margin-bottom:20px;"></div>
                        <p style="opacity:0.3; text-align:center; margin-top:40px;">No active reasoning. Query memory to begin.</p>
                    </div>
                </div>
                <div id="feed" class="content">
                    <div id="activeContext" style="margin-bottom:20px;"></div>
                    <div id="feedList"></div>
                    <div style="display:flex; gap:8px; margin-top:15px;">
                        <button class="btn" data-action="intent">Intent</button>
                        <button class="btn" data-action="handoff">Handoff</button>
                        <button class="btn btn-primary" data-action="claim">Claim</button>
                    </div>
                </div>
                <script nonce="${nonce}">
                    (function() {
                        const vscode = acquireVsCodeApi();
                        const lists = { 
                            explorer: document.getElementById('resultsList'), 
                            inspector: document.getElementById('reasoningContent'), 
                            feed: document.getElementById('feedList'),
                            activeContext: document.getElementById('activeContext'),
                            activeWork: document.getElementById('activeWorkList')
                        };
                        // Memory content is untrusted (any agent or document can write it):
                        // everything interpolated into markup goes through esc().
                        const esc = (v) => String(v ?? '').replace(/[&<>"']/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
                        // The memories currently shown, so Copy/Insert refer to them by index.
                        let shown = [];

                        // Tab switching
                        document.querySelectorAll('.tab').forEach(t => {
                            t.addEventListener('click', () => {
                                const id = t.getAttribute('data-tab');
                                document.querySelectorAll('.tab').forEach(el => el.classList.remove('active'));
                                document.querySelectorAll('.content').forEach(el => el.classList.remove('active'));
                                t.classList.add('active');
                                const content = document.getElementById(id);
                                if (content) content.classList.add('active');
                                if (id === 'feed' || id === 'inspector') {
                                    vscode.postMessage({ type: 'getFeed' });
                                }
                            });
                        });

                        // One delegated handler instead of inline onclick (blocked by the CSP).
                        document.body.addEventListener('click', (e) => {
                            const el = e.target.closest('[data-action]');
                            if (!el) return;
                            const action = el.getAttribute('data-action');
                            if (action === 'open') vscode.postMessage({ type: 'openFile', path: el.getAttribute('data-path') });
                            if (action === 'copy') { const m = shown[+el.getAttribute('data-i')]; if (m) navigator.clipboard.writeText(m.content || ''); }
                            if (action === 'insert') { const m = shown[+el.getAttribute('data-i')]; if (m) vscode.postMessage({ type: 'insert', text: m.content || '' }); }
                            if (action === 'intent') vscode.postMessage({ type: 'intent' });
                            if (action === 'handoff') vscode.postMessage({ type: 'handoff' });
                            if (action === 'claim') vscode.postMessage({ type: 'claim' });
                        });

                        window.addEventListener('message', e => {
                            const d = e.data;
                            try {
                                if(d.type === 'results') renderResults(d.results);
                                if(d.type === 'reasoning') renderReasoning(d.data);
                                if(d.type === 'activeWork') renderActiveWork({ claims: d.claims || [] });
                                if(d.type === 'feed') {
                                    renderFeed(d.items);
                                    renderActiveContext(d.context);
                                    renderActiveWork(d.work);
                                }
                                if(d.type === 'error') lists.explorer.innerHTML = '<p style="color:red">Error: ' + esc(d.message) + '</p>';
                            } catch (err) {
                                console.error('Error handling message:', err);
                            }
                        });

                        function parseList(items) {
                            try {
                                const v = JSON.parse(items && items[0] && items[0].text || '[]');
                                return Array.isArray(v) ? v : [];
                            } catch { return []; }
                        }

                        const fileLink = (fileId, label) => fileId
                            ? \`<span class="pill pill-file" data-action="open" data-path="\${esc(fileId)}">\${esc(label)}</span>\`
                            : '';

                        function renderResults(items) {
                            shown = parseList(items);
                            lists.explorer.innerHTML = shown.map((m, i) => {
                                const fileId = m.metadata && m.metadata.file_id;
                                return \`
                                    <div class="card">
                                        <div style="display:flex; justify-content:space-between; align-items:center">
                                            <span class="pill pill-type">\${esc(m.type ? String(m.type).toUpperCase() : 'UNKNOWN')}</span>
                                            \${fileLink(fileId, fileId ? String(fileId).split('/').pop() : '')}
                                        </div>
                                        <div style="font-size:12px; margin-top:8px; white-space:pre-wrap;">\${esc(m.content)}</div>
                                        <div style="display:flex; gap:8px; margin-top:10px;">
                                            <button class="btn" style="padding:2px; flex:1" data-action="copy" data-i="\${i}">Copy</button>
                                            <button class="btn btn-primary" style="padding:2px; flex:1" data-action="insert" data-i="\${i}">Insert</button>
                                        </div>
                                    </div>
                                \`;
                            }).join('') || '<p style="text-align:center; opacity:0.3">No traces.</p>';
                        }

                        function renderReasoning(raw) {
                            let p;
                            try { p = JSON.parse(raw); } catch { p = { answer: raw }; }
                            const conf = Math.max(0, Math.min(100, Math.round((Number(p.confidence) || 0) * 100)));
                            const why = typeof p.why === 'string' ? p.why : (p.why ? JSON.stringify(p.why, null, 2) : '');
                            lists.inspector.innerHTML = \`
                                <div class="card">
                                    <div style="font-size:13px; line-height:1.5; white-space:pre-wrap;">\${esc(p.answer)}</div>
                                    <div class="confidence-bar"><div class="confidence-fill" style="width:\${conf}%"></div></div>
                                    \${why ? \`<p style="font-size:10px; opacity:0.5; margin-top:15px">WHY</p>
                                    <div style="font-size:11px; opacity:0.8; white-space:pre-wrap;">\${esc(why)}</div>\` : ''}
                                </div>
                            \`;
                        }

                        function renderFeed(items) {
                            const evs = parseList(items);
                            lists.feed.innerHTML = evs.reverse().map(ev => {
                                const action = String((ev.metadata && ev.metadata.action) || 'event');
                                const emoji = action === 'claim' ? '🔴' : action === 'release' ? '🟢' : action === 'handoff' ? '🤝' : '🎯';
                                return \`
                                    <div class="feed-event">
                                        <span>\${emoji}</span>
                                        <div style="flex:1">
                                            <b style="font-size:9px; opacity:0.6; letter-spacing:0.5px;">\${esc(action.toUpperCase())}</b>
                                            <div style="font-size:12px;">\${esc(ev.content)}</div>
                                        </div>
                                    </div>
                                \`;
                            }).join('') || '<p style="text-align:center; opacity:0.3">All quiet.</p>';
                        }

                        function renderActiveWork(ctx) {
                            if (!ctx || !Array.isArray(ctx.claims) || !ctx.claims.length) {
                                lists.activeWork.innerHTML = '<p style="font-size:10px; opacity:0.3; text-align:center;">No active work in session.</p>';
                                return;
                            }
                            lists.activeWork.innerHTML = \`
                                <h4 style="margin:0 0 10px 0; font-size:10px; color:var(--accent-blue); letter-spacing:1px;">ACTIVE WORK</h4>
                                \${ctx.claims.map(c => {
                                    const resName = c.resource ? String(c.resource).split('/').pop() : 'Workspace';
                                    return \`<div class="card active-work-item" style="margin-bottom:8px; padding:8px;">
                                            <div style="display:flex; justify-content:space-between; align-items:center;">
                                                <b style="font-size:11px; color:white;">\${esc(resName)}</b>
                                                <span style="font-size:9px; opacity:0.5;">\${esc(c.timestamp)}</span>
                                            </div>
                                            <div style="font-size:10px; margin-top:4px;">👤 \${esc(c.agent || 'Unknown')}</div>
                                            <div style="font-size:10px; opacity:0.7; margin-top:4px; font-style:italic;">\${esc(c.reason)}</div>
                                        </div>\`;
                                }).join('')}
                            \`;
                        }

                        function renderActiveContext(ctx) {
                            if (!ctx || !Array.isArray(ctx.claims) || !ctx.claims.length) {
                                 lists.activeContext.innerHTML = '';
                                 return;
                            }
                            lists.activeContext.innerHTML = \`
                                <div style="background:rgba(255,77,79,0.1); border:1px solid rgba(255,77,79,0.3); padding:8px; border-radius:4px; font-size:11px; margin-bottom:15px;">
                                    <span style="color:var(--accent-red)">🔴 FILE CLAIMED:</span> 
                                    <b>\${esc(ctx.claims[0].agent || 'Unknown')}</b> is working here.
                                </div>
                            \`;
                        }

                        document.getElementById('sessionSelect').addEventListener('change', (e) => {
                            // prompt() does not work in webviews; the extension asks for a custom ID.
                            vscode.postMessage({ type: 'setSession', session: e.target.value === 'custom' ? undefined : e.target.value });
                        });

                        document.getElementById('inspectorQuery').addEventListener('keypress', (e) => {
                            if (e.key === 'Enter') {
                                vscode.postMessage({ type: 'reason', query: e.target.value });
                                lists.inspector.innerHTML = '<p style="text-align:center; margin-top:40px; font-size:11px; opacity:0.5;">🧠 Thinking...</p>';
                            }
                        });

                        document.getElementById('refreshBtn').addEventListener('click', () => vscode.postMessage({ type: 'getRecent' }));
                        document.getElementById('evolveBtn').addEventListener('click', () => vscode.postMessage({ type: 'evolve' }));
                        
                        let sTo;
                        document.getElementById('searchInput').addEventListener('input', (e) => {
                            clearTimeout(sTo);
                            sTo = setTimeout(() => vscode.postMessage({ type: 'search', query: e.target.value }), 300);
                        });

                        // Initial load
                        vscode.postMessage({ type: 'getRecent' });
                        vscode.postMessage({ type: 'getFeed' });
                    })();
                </script>
            </body>
            </html>`;
    }
}
