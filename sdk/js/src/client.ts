/**
 * Low-level HTTP client for the Memro API. One method per endpoint; most
 * code should use `Agent`, which keeps the credentials.
 */
import { generateKeypair, signRequest, solvePow } from './crypto.js';
import type {
  ConflictAnalysis, EvolutionResult, ExportData, FileInput, HealthStatus, Memory, MemoryChain,
  MemoryTimeline, ReasoningResult, ResolutionResult, SearchResponse, TemporalResult, TemporalStats,
  UploadedFile, Usage, WhoAmI,
} from './types.js';

export const DEFAULT_BASE_URL = 'https://api.memro.co';
export const API_KEY_PREFIX = 'memro_sk_';

export class MemroError extends Error {
  constructor(public readonly status: number, message: string) {
    super(`[${status}] ${message}`);
    this.name = 'MemroError';
  }
}

/** HTTP 429: slow down and retry later. */
export class RateLimitError extends MemroError {
  constructor(message: string) {
    super(429, message);
    this.name = 'RateLimitError';
  }
}

/** Credentials for one call: an agent's own key, when not using an API key. */
export interface Signer {
  agentId: string;
  privateKey: string;
}

export interface MemroClientOptions {
  /** Defaults to MEMRO_BASE_URL, then https://api.memro.co. */
  baseUrl?: string;
  /** memro_sk_... Defaults to MEMRO_API_KEY. */
  apiKey?: string;
  /** Per-request timeout in ms (default 30 000). */
  timeout?: number;
  /** Retries for rate limits and gateway errors on safe requests (default 2). */
  maxRetries?: number;
  /** Custom fetch, e.g. for testing or older runtimes. */
  fetch?: typeof fetch;
}

type Query = Record<string, string | number | boolean | undefined | null>;

interface RequestOptions {
  query?: Query;
  json?: unknown;
  form?: FormData;
  signer?: Signer;
  auth?: boolean;
}

const env = (name: string): string | undefined =>
  typeof process !== 'undefined' && process.env ? process.env[name] : undefined;

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

export class MemroClient {
  readonly baseUrl: string;
  readonly apiKey?: string;
  private readonly timeout: number;
  private readonly maxRetries: number;
  private readonly fetchImpl: typeof fetch;

  constructor(options: MemroClientOptions = {}) {
    this.baseUrl = (options.baseUrl ?? env('MEMRO_BASE_URL') ?? DEFAULT_BASE_URL).replace(/\/+$/, '');
    this.apiKey = options.apiKey ?? env('MEMRO_API_KEY');
    if (this.apiKey && !this.apiKey.startsWith(API_KEY_PREFIX)) {
      throw new Error('memro: API keys start with memro_sk_. Create one with `npx memro-cli connect` or in the dashboard.');
    }
    this.timeout = options.timeout ?? 30_000;
    this.maxRetries = options.maxRetries ?? 2;
    this.fetchImpl = options.fetch ?? globalThis.fetch.bind(globalThis);
  }

  /**
   * Sends a request and returns the parsed JSON (or undefined for 204).
   * The signature covers the exact path, query string and body sent.
   */
  async request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(opts.query ?? {})) {
      if (v !== undefined && v !== null) params.append(k, String(v));
    }
    const qs = params.toString();
    const pathAndQuery = path + (qs ? `?${qs}` : '');
    const url = this.baseUrl + pathAndQuery;

    let body: Uint8Array = new Uint8Array(0);
    const baseHeaders: Record<string, string> = { Accept: 'application/json' };
    if (opts.form) {
      // Serialize the multipart body once so the bytes signed are the bytes sent.
      const req = new Request(url, { method, body: opts.form });
      body = new Uint8Array(await req.arrayBuffer());
      baseHeaders['Content-Type'] = req.headers.get('Content-Type')!;
    } else if (opts.json !== undefined) {
      body = new TextEncoder().encode(JSON.stringify(opts.json));
      baseHeaders['Content-Type'] = 'application/json';
    }

    for (let attempt = 0; ; attempt++) {
      const headers = { ...baseHeaders, ...(opts.auth === false ? {} : this.authHeaders(method, pathAndQuery, body, opts.signer)) };
      const ctrl = new AbortController();
      const timer = setTimeout(() => ctrl.abort(), this.timeout);
      let resp: Response;
      try {
        resp = await this.fetchImpl(url, { method, headers, body: body.length ? (body as BodyInit) : undefined, signal: ctrl.signal });
      } finally {
        clearTimeout(timer);
      }
      const text = await resp.text();
      if (resp.ok) return (text ? JSON.parse(text) : undefined) as T;

      const canRetry = attempt < this.maxRetries;
      if (canRetry && resp.status === 401 && text.includes('already used') && 'X-Signature' in headers) {
        // Signatures are deterministic: re-sign an identical request in the next second.
        await sleep(1050 - (Date.now() % 1000));
        continue;
      }
      if (canRetry && (resp.status === 429 || ([502, 503, 504].includes(resp.status) && (method === 'GET' || method === 'DELETE')))) {
        const after = Number(resp.headers.get('Retry-After'));
        await sleep(Number.isFinite(after) && after > 0 ? Math.min(after, 30) * 1000 : 500 * 2 ** attempt);
        continue;
      }
      let message = text || resp.statusText;
      try {
        const j = JSON.parse(text);
        message = j.message ?? j.error ?? message;
      } catch { /* plain-text error */ }
      throw resp.status === 429 ? new RateLimitError(message) : new MemroError(resp.status, message);
    }
  }

  /** Authentication headers for one request; override to add headers. */
  protected authHeaders(method: string, pathAndQuery: string, body: Uint8Array, signer?: Signer): Record<string, string> {
    if (this.apiKey) return { Authorization: `Bearer ${this.apiKey}` };
    if (signer) return signRequest(signer.agentId, signer.privateKey, method, pathAndQuery, body);
    return {};
  }

  // ── Account ─────────────────────────────────────────────────────────────

  whoami(signer?: Signer) { return this.request<WhoAmI>('GET', '/auth/whoami', { signer }); }
  health() { return this.request<HealthStatus>('GET', '/health', { auth: false }); }

  // ── Identity ────────────────────────────────────────────────────────────

  /** Generates a key pair locally and registers only the public key. */
  async createIdentity(): Promise<{ agent_id: string; private_key: string }> {
    const kp = generateKeypair();
    const body: Record<string, unknown> = { public_key: kp.publicKey };
    const pow = await this.powParams();
    if (pow) body.pow = solvePow(kp.publicKey, pow.difficulty);
    await this.request('POST', '/identity', { json: body, auth: false });
    return { agent_id: kp.publicKey, private_key: kp.privateKey };
  }
  /** The server's registration proof-of-work settings, or null if it has none. */
  private async powParams(): Promise<{ difficulty: number; required: boolean } | null> {
    try {
      return await this.request<{ difficulty: number; required: boolean }>('GET', '/identity/pow', { auth: false });
    } catch (e) {
      if (e instanceof MemroError && e.status === 404) return null;
      throw e;
    }
  }
  getIdentity(agentId: string) {
    return this.request<{ id: string; created_at: string }>('GET', `/identity/${agentId}`, { auth: false });
  }
  /** Deletes the agent with all of its memories, files, keys and webhooks. */
  deleteIdentity(agentId: string, signer?: Signer) { return this.request<void>('DELETE', `/identity/${agentId}`, { signer }); }

  // ── Memories ────────────────────────────────────────────────────────────

  createMemory(body: Record<string, unknown>, signer?: Signer) { return this.request<Memory>('POST', '/memory', { json: body, signer }); }
  listMemories(agentId: string, query: Query, signer?: Signer) { return this.request<Memory[]>('GET', `/memory/agent/${agentId}`, { query, signer }); }
  async deleteMemory(memoryId: string, signer?: Signer): Promise<boolean> {
    try {
      await this.request('DELETE', `/memory/${memoryId}`, { signer });
      return true;
    } catch (e) {
      if (e instanceof MemroError && e.status === 404) return false;
      throw e;
    }
  }
  export(agentId: string, signer?: Signer) { return this.request<ExportData>('GET', `/export/${agentId}`, { signer }); }

  // ── Search ──────────────────────────────────────────────────────────────

  search(query: Query, signer?: Signer) { return this.request<SearchResponse>('GET', '/search', { query, signer }); }
  searchTemporal(query: Query, signer?: Signer) {
    return this.request<{ results: TemporalResult[]; total: number }>('GET', '/search/temporal', { query, signer });
  }

  // ── Memory engine ───────────────────────────────────────────────────────

  reason(query: string, signer?: Signer) { return this.request<ReasoningResult>('POST', '/mee/reason', { json: { query }, signer }); }
  resolveConflict(a: string, b: string, signer?: Signer) {
    return this.request<ResolutionResult>('POST', '/mee/resolve', { json: { memory_a_id: a, memory_b_id: b }, signer });
  }
  evolve(signer?: Signer) { return this.request<EvolutionResult>('POST', '/mee/evolve', { signer }); }
  timeline(memoryId: string, signer?: Signer) { return this.request<MemoryTimeline>('GET', `/mee/timeline/${memoryId}`, { signer }); }
  memoryChain(memoryId: string, signer?: Signer) { return this.request<MemoryChain>('GET', `/memory/chain/${memoryId}`, { signer }); }
  analyzeConflicts(signer?: Signer) { return this.request<ConflictAnalysis>('GET', '/analyze/conflicts', { signer }); }
  generateAtoms(text: string, signer?: Signer) {
    return this.request<{ atomic_memories: string[]; chunk_preview: string }>('POST', '/memory/generate-atoms', { json: { chunk: text }, signer });
  }
  stats(signer?: Signer) { return this.request<TemporalStats>('GET', '/stats/temporal', { signer }); }
  usage(signer?: Signer) { return this.request<Usage>('GET', '/usage', { signer }); }

  // ── Files ───────────────────────────────────────────────────────────────

  uploadFile(agentId: string, file: FileInput, signer?: Signer) {
    const form = formWith('file', file);
    form.append('agent_id', agentId);
    return this.request<UploadedFile>('POST', '/upload', { form, signer });
  }
  memoryFromFile(body: Record<string, unknown>, signer?: Signer) {
    return this.request<{ memory_id: string; file_id: string; content: string }>('POST', '/memory/from-file', { json: body, signer });
  }
  uploadImage(file: FileInput, signer?: Signer) { return this.request<Memory>('POST', '/upload/image', { form: formWith('image', file), signer }); }
  uploadDocument(file: FileInput, signer?: Signer) { return this.request<Memory[]>('POST', '/upload/document', { form: formWith('document', file), signer }); }

  // ── Webhooks ────────────────────────────────────────────────────────────

  registerWebhook(url: string, eventType = 'memory_created', signer?: Signer) {
    return this.request<void>('POST', '/hooks', { json: { event_type: eventType, url }, signer });
  }
  async deleteWebhook(url: string, eventType = 'memory_created', signer?: Signer): Promise<boolean> {
    try {
      await this.request('DELETE', '/hooks', { json: { event_type: eventType, url }, signer });
      return true;
    } catch (e) {
      if (e instanceof MemroError && e.status === 404) return false;
      throw e;
    }
  }
  listWebhooks(eventType = 'memory_created', signer?: Signer) { return this.request<string[]>('GET', `/hooks/${eventType}`, { signer }); }

  // ── Coordination ────────────────────────────────────────────────────────

  coordinationClaim(body: Record<string, unknown>, signer?: Signer) {
    return this.request<Record<string, unknown>>('POST', '/coordination/claim', { json: body, signer });
  }
  coordinationHeartbeat(body: Record<string, unknown>, signer?: Signer) {
    return this.request<Record<string, unknown>>('POST', '/coordination/heartbeat', { json: body, signer });
  }
  coordinationRelease(body: Record<string, unknown>, signer?: Signer) {
    return this.request<Record<string, unknown>>('POST', '/coordination/release', { json: body, signer });
  }
  coordinationHandoff(body: Record<string, unknown>, signer?: Signer) {
    return this.request<Record<string, unknown>>('POST', '/coordination/handoff', { json: body, signer });
  }
  coordinationCheck(body: Record<string, unknown>, signer?: Signer) {
    return this.request<Record<string, unknown>>('POST', '/coordination/check', { json: body, signer });
  }
  coordinationActive(query?: Record<string, string>, signer?: Signer) {
    return this.request<Record<string, unknown>>('GET', '/coordination/active', { query, signer });
  }
  coordinationIntent(body: Record<string, unknown>, signer?: Signer) {
    return this.request<Record<string, unknown>>('POST', '/coordination/intent', { json: body, signer });
  }
}

function formWith(field: string, file: FileInput): FormData {
  const form = new FormData();
  if (typeof Blob !== 'undefined' && file instanceof Blob) {
    form.append(field, file, (file as File).name ?? field);
  } else {
    const f = file as { name: string; data: Uint8Array | ArrayBuffer; type?: string };
    form.append(field, new Blob([f.data as BlobPart], { type: f.type ?? 'application/octet-stream' }), f.name);
  }
  return form;
}
