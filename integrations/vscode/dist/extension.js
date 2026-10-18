"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.activate = activate;
exports.deactivate = deactivate;
const vscode = __importStar(require("vscode"));
const index_js_1 = require("@modelcontextprotocol/sdk/client/index.js");
const sse_js_1 = require("@modelcontextprotocol/sdk/client/sse.js");
const webview_js_1 = require("./webview.js");
let mcpClient = null;
let statusBarItem;
let currentSessionId = 'default';
function resetMcpClient() {
    if (mcpClient) {
        mcpClient.close();
        mcpClient = null;
    }
}
async function getMcpClient() {
    if (mcpClient)
        return mcpClient;
    const config = vscode.workspace.getConfiguration('memro');
    let agentId = config.get('agentId');
    let privateKey = config.get('privateKey');
    let mcpBaseUrl = config.get('mcpBaseUrl');
    currentSessionId = config.get('sessionId') || 'default';
    if (!agentId) {
        agentId = await vscode.window.showInputBox({
            prompt: 'Enter your Memro Agent ID',
            ignoreFocusOut: true
        });
        if (agentId) {
            await config.update('agentId', agentId, vscode.ConfigurationTarget.Global);
        }
    }
    if (!privateKey) {
        privateKey = await vscode.window.showInputBox({
            prompt: 'Enter your Memro Private Key',
            password: true,
            ignoreFocusOut: true
        });
        if (privateKey) {
            await config.update('privateKey', privateKey, vscode.ConfigurationTarget.Global);
        }
    }
    if (!mcpBaseUrl || mcpBaseUrl === 'http://localhost:8080/sse') {
        const newUrl = await vscode.window.showInputBox({
            prompt: 'Enter Memro MCP Server URL (SSE)',
            value: mcpBaseUrl || 'http://localhost:8080/sse',
            ignoreFocusOut: true
        });
        if (newUrl) {
            mcpBaseUrl = newUrl;
            await config.update('mcpBaseUrl', mcpBaseUrl, vscode.ConfigurationTarget.Global);
        }
    }
    if (!agentId || !privateKey || !mcpBaseUrl) {
        vscode.window.showErrorMessage('Memro Agent ID, Private Key, and Server URL are required.');
        throw new Error('Missing credentials');
    }
    const sseUrl = new URL(mcpBaseUrl);
    sseUrl.searchParams.set('agent_id', agentId);
    sseUrl.searchParams.set('token', privateKey);
    sseUrl.searchParams.set('session_id', currentSessionId);
    const transport = new sse_js_1.SSEClientTransport(sseUrl);
    mcpClient = new index_js_1.Client({ name: 'vscode-memro', version: '1.0.0' }, {});
    await mcpClient.connect(transport);
    return mcpClient;
}
function activate(context) {
    console.log('Memro Extension is now active!');
    const configListener = vscode.workspace.onDidChangeConfiguration(e => {
        if (e.affectsConfiguration('memro')) {
            resetMcpClient();
            console.log('Memro: Config changed, resetting MCP client.');
        }
    });
    context.subscriptions.push(configListener);
    statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
    statusBarItem.command = 'memro-explorer.focus';
    statusBarItem.text = '$(outline-view) Memro';
    statusBarItem.show();
    context.subscriptions.push(statusBarItem);
    const remember = vscode.commands.registerCommand('memro.remember', async () => {
        let content;
        const editor = vscode.window.activeTextEditor;
        let metadata = {};
        if (editor) {
            const fileName = vscode.workspace.asRelativePath(editor.document.uri);
            const language = editor.document.languageId;
            metadata.file_id = fileName;
            metadata.language = language;
            const gitExtension = vscode.extensions.getExtension('vscode.git')?.exports;
            if (gitExtension) {
                const repo = gitExtension.getAPI(1).repositories[0];
                if (repo) {
                    metadata.git_branch = repo.state.HEAD?.name || 'unknown';
                }
            }
            try {
                const symbols = await vscode.commands.executeCommand('vscode.executeDocumentSymbolProvider', editor.document.uri);
                if (symbols) {
                    const cursorPosition = editor.selection.active;
                    const findFunction = (s) => {
                        for (const sym of s) {
                            if (sym.range.contains(cursorPosition)) {
                                if (sym.kind === vscode.SymbolKind.Function || sym.kind === vscode.SymbolKind.Method || sym.kind === vscode.SymbolKind.Class) {
                                    return sym.name;
                                }
                                if (sym.children)
                                    return findFunction(sym.children);
                            }
                        }
                    };
                    metadata.function_name = findFunction(symbols);
                }
            }
            catch (symbolError) {
                console.warn('Memro: Could not capture function name (Language Server busy/unavailable)', symbolError);
            }
            if (!editor.selection.isEmpty) {
                content = editor.document.getText(editor.selection);
            }
        }
        if (!content) {
            content = await vscode.window.showInputBox({
                prompt: 'What would you like to remember?',
                placeHolder: 'Select text or type here...',
                ignoreFocusOut: true
            });
        }
        if (!content)
            return;
        try {
            const client = await getMcpClient();
            const res = await client.callTool({
                name: 'remember_with_context',
                arguments: {
                    content: content,
                    context_metadata: metadata,
                    type: 'episodic',
                    visibility: 'private',
                    session_id: currentSessionId
                }
            });
            const resContent = res.content;
            const resText = resContent?.[0]?.text || 'Memory stored successully';
            vscode.window.showInformationMessage(`Memro: ${resText}`);
        }
        catch (e) {
            vscode.window.showErrorMessage(`Failed to store memory: ${e instanceof Error ? e.message : String(e)}`);
        }
    });
    const recall = vscode.commands.registerCommand('memro.recall', async () => {
        const query = await vscode.window.showInputBox({
            prompt: 'Recall memory for query',
            ignoreFocusOut: true
        });
        if (!query)
            return;
        try {
            const client = await getMcpClient();
            const res = await client.callTool({
                name: 'recall',
                arguments: {
                    query,
                    limit: 5,
                    session_id: currentSessionId
                }
            });
            const resContent = res.content;
            const text = resContent?.[0]?.text ?? '';
            vscode.window.showInformationMessage(text || 'No memories found.');
        }
        catch (e) {
            vscode.window.showErrorMessage(`Recall failed: ${e instanceof Error ? e.message : String(e)}`);
        }
    });
    const getRecent = vscode.commands.registerCommand('memro.getRecent', async () => {
        try {
            const client = await getMcpClient();
            const res = await client.callTool({ name: 'get_recent_memories', arguments: { type: 'procedural', limit: 10 } });
            const resContent = res.content;
            vscode.window.showInformationMessage(resContent?.[0]?.text ?? 'No memories.');
        }
        catch (e) {
            vscode.window.showErrorMessage(`Get recent failed: ${e instanceof Error ? e.message : String(e)}`);
        }
    });
    const deleteMemory = vscode.commands.registerCommand('memro.deleteMemory', async () => {
        const memoryId = await vscode.window.showInputBox({
            prompt: 'Memory ID to delete',
            ignoreFocusOut: true
        });
        if (!memoryId)
            return;
        try {
            const client = await getMcpClient();
            const res = await client.callTool({ name: 'delete_memory', arguments: { memory_id: memoryId } });
            const resContent = res.content;
            vscode.window.showInformationMessage(resContent?.[0]?.text ?? 'Deleted.');
        }
        catch (e) {
            vscode.window.showErrorMessage(`Delete failed: ${e instanceof Error ? e.message : String(e)}`);
        }
    });
    const exportMemories = vscode.commands.registerCommand('memro.exportMemories', async () => {
        try {
            const client = await getMcpClient();
            const res = await client.callTool({ name: 'export_memories', arguments: {} });
            const resContent = res.content;
            vscode.window.showInformationMessage(resContent?.[0]?.text ?? 'Export complete.');
        }
        catch (e) {
            vscode.window.showErrorMessage(`Export failed: ${e instanceof Error ? e.message : String(e)}`);
        }
    });
    const queryGraph = vscode.commands.registerCommand('memro.queryGraph', async () => {
        const q = await vscode.window.showInputBox({
            prompt: 'Knowledge graph query',
            ignoreFocusOut: true
        });
        if (!q)
            return;
        try {
            const client = await getMcpClient();
            const res = await client.callTool({ name: 'query_graph', arguments: { query: q } });
            const resContent = res.content;
            vscode.window.showInformationMessage(resContent?.[0]?.text ?? 'No results.');
        }
        catch (e) {
            vscode.window.showErrorMessage(`Graph query failed: ${e instanceof Error ? e.message : String(e)}`);
        }
    });
    const openFile = vscode.commands.registerCommand('memro.openFile', async (filePath) => {
        const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
        if (!workspaceFolder)
            return;
        const uri = vscode.Uri.joinPath(workspaceFolder.uri, filePath);
        try {
            const doc = await vscode.workspace.openTextDocument(uri);
            await vscode.window.showTextDocument(doc);
        }
        catch (e) {
            vscode.window.showErrorMessage('Could not open file: ' + e);
        }
    });
    const claim = vscode.commands.registerCommand('memro.claim', async () => {
        const editor = vscode.window.activeTextEditor;
        const resource = editor ? vscode.workspace.asRelativePath(editor.document.uri) : 'Workspace';
        const intent = await vscode.window.showInputBox({ prompt: `What is your intent for ${resource}?` });
        if (!intent)
            return;
        try {
            const client = await getMcpClient();
            await client.callTool({
                name: 'claim_task',
                arguments: {
                    resource,
                    intent,
                    session_id: currentSessionId
                }
            });
            vscode.window.showInformationMessage(`Memro: ${resource} claimed in session ${currentSessionId}.`);
        }
        catch (e) {
            vscode.window.showErrorMessage('Claim failed: ' + e);
        }
    });
    const logIntent = vscode.commands.registerCommand('memro.logIntent', async () => {
        const editor = vscode.window.activeTextEditor;
        const resource = editor ? vscode.workspace.asRelativePath(editor.document.uri) : 'Workspace';
        const intent = await vscode.window.showInputBox({ prompt: 'Declare your intent (e.g. Refactoring auth module)' });
        if (!intent)
            return;
        try {
            const client = await getMcpClient();
            await client.callTool({
                name: 'log_intent',
                arguments: {
                    resource,
                    intent,
                    session_id: currentSessionId
                }
            });
            vscode.window.showInformationMessage('Memro: Intent logged.');
        }
        catch (e) {
            vscode.window.showErrorMessage('Failed to log intent: ' + e);
        }
    });
    const release = vscode.commands.registerCommand('memro.release', async () => {
        const editor = vscode.window.activeTextEditor;
        const resource = editor ? vscode.workspace.asRelativePath(editor.document.uri) : 'Workspace';
        try {
            const client = await getMcpClient();
            await client.callTool({
                name: 'release_task',
                arguments: {
                    resource,
                    status: 'completed',
                    session_id: currentSessionId
                }
            });
            vscode.window.showInformationMessage(`Memro: ${resource} released.`);
            updateStatusBar('allow', '');
        }
        catch (e) {
            vscode.window.showErrorMessage('Release failed: ' + e);
        }
    });
    const handoff = vscode.commands.registerCommand('memro.handoff', async () => {
        const editor = vscode.window.activeTextEditor;
        const resource = editor ? vscode.workspace.asRelativePath(editor.document.uri) : 'Workspace';
        const summary = await vscode.window.showInputBox({
            prompt: 'Summarize work done for handover (e.g. Refactored auth but tests need fix)',
            placeHolder: 'Work summary...'
        });
        if (!summary)
            return;
        const nextAgent = await vscode.window.showInputBox({
            prompt: 'Assign to specific agent? (Optional)',
            placeHolder: 'Agent ID or leave empty'
        });
        try {
            const client = await getMcpClient();
            await client.callTool({
                name: 'handoff_task',
                arguments: {
                    resource,
                    summary,
                    next_agent_id: nextAgent || undefined,
                    session_id: currentSessionId
                }
            });
            vscode.window.showInformationMessage(`Memro: ${resource} handed off.`);
            updateStatusBar('allow', '');
        }
        catch (e) {
            vscode.window.showErrorMessage('Handoff failed: ' + e);
        }
    });
    function updateStatusBar(decision, reason) {
        if (decision === 'block') {
            statusBarItem.text = `$(lock) Memro: Claimed`;
            statusBarItem.backgroundColor = new vscode.ThemeColor('statusBarItem.errorBackground');
            statusBarItem.tooltip = reason;
        }
        else if (decision === 'warn') {
            statusBarItem.text = `$(warning) Memro: Overlap`;
            statusBarItem.backgroundColor = new vscode.ThemeColor('statusBarItem.warningBackground');
            statusBarItem.tooltip = reason;
        }
        else {
            statusBarItem.text = `$(outline-view) Memro`;
            statusBarItem.backgroundColor = undefined;
            statusBarItem.tooltip = 'Memro: All clear';
        }
    }
    let poller;
    function startPoller() {
        if (poller)
            clearInterval(poller);
        poller = setInterval(async () => {
            try {
                const client = await getMcpClient();
                const res = await client.callTool({ name: 'get_active_work', arguments: { session_id: currentSessionId } });
                const data = JSON.parse(res.content[0].text);
                provider.updateActiveWork(data.claims);
                const editor = vscode.window.activeTextEditor;
                if (editor) {
                    const resource = vscode.workspace.asRelativePath(editor.document.uri);
                    const claim = data.claims.find((c) => c.resource === resource);
                    if (claim)
                        updateStatusBar('block', `Claimed by ${claim.agent}`);
                    else
                        updateStatusBar('allow', '');
                }
            }
            catch (e) { }
        }, 10000);
    }
    async function performPreActionCheck(document) {
        const resource = vscode.workspace.asRelativePath(document.uri);
        if (resource.startsWith('.') || resource.includes('node_modules'))
            return;
        try {
            const client = await getMcpClient();
            const res = await client.callTool({
                name: 'pre_action_check',
                arguments: {
                    resource,
                    intent: "Reading/Opening file",
                    session_id: currentSessionId
                }
            });
            const data = JSON.parse(res.content[0].text);
            updateStatusBar(data.decision, data.reason || '');
            if (data.decision === 'block') {
                vscode.window.showErrorMessage(`🔴 Conflict: ${resource} is claimed by ${data.details.agent}`, 'View Details')
                    .then(selection => {
                    if (selection === 'View Details')
                        vscode.commands.executeCommand('memro-explorer.focus');
                });
            }
        }
        catch (e) {
            console.error('Memro: Pre-action check failed', e);
        }
    }
    let intentTimeout;
    async function autoClaimOnEdit(document) {
        if (intentTimeout)
            clearTimeout(intentTimeout);
        intentTimeout = setTimeout(async () => {
            const resource = vscode.workspace.asRelativePath(document.uri);
            if (resource.startsWith('.') || resource.includes('node_modules'))
                return;
            try {
                const client = await getMcpClient();
                const statusRes = await client.callTool({ name: 'get_active_context', arguments: { resource } });
                const status = JSON.parse(statusRes.content[0].text);
                if (status.claims.length === 0) {
                    const claimRes = await client.callTool({
                        name: 'claim_task',
                        arguments: {
                            resource,
                            intent: `Editing ${resource}`,
                            session_id: currentSessionId
                        }
                    });
                    if (claimRes.isError) {
                        console.error('Memro: Auto-claim tool returned error', claimRes.content);
                        return;
                    }
                    vscode.window.showInformationMessage(`⬢ Memro: Auto-claimed ${resource} for editing.`);
                    vscode.window.setStatusBarMessage(`$(lock) Memro: Auto-claimed ${resource}`, 3000);
                }
            }
            catch (e) {
                console.error('Memro: Auto-claim failed', e);
            }
        }, 500);
    }
    vscode.workspace.onDidOpenTextDocument(performPreActionCheck);
    vscode.workspace.onDidChangeTextDocument(e => autoClaimOnEdit(e.document));
    startPoller();
    const provider = new webview_js_1.MemoryWebviewProvider(context.extensionUri, () => getMcpClient());
    const memoryExplorer = vscode.window.registerWebviewViewProvider(webview_js_1.MemoryWebviewProvider.viewType, provider);
    context.subscriptions.push(remember, recall, getRecent, deleteMemory, exportMemories, queryGraph, openFile, claim, logIntent, release, handoff, memoryExplorer);
}
function deactivate() {
    mcpClient = null;
}
//# sourceMappingURL=extension.js.map