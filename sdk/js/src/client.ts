/**
 * Low-level HTTP client for the memro protocol API.
 */
import axios, { AxiosInstance, AxiosError } from 'axios';
import { signBody } from './crypto';
import type {
  Memory,
  SearchResult,
  ExportData,
  HealthStatus,
  MemoryType,
  Visibility,
} from './types';

export class MemroError extends Error {
  constructor(public statusCode: number, message: string) {
    super(`[${statusCode}] ${message}`);
    this.name = 'MemroError';
  }
}

function handleAxiosError(err: unknown): never {
  if (axios.isAxiosError(err)) {
    const e = err as AxiosError;
    const status = e.response?.status ?? 0;
    const msg = (e.response?.data as string) ?? e.message;
    throw new MemroError(status, msg);
  }
  throw err;
}

export interface MemroClientOptions {
  baseUrl?: string;
}

export class MemroClient {
  private http: AxiosInstance;

  constructor(options: MemroClientOptions = {}) {
    const baseURL =
      options.baseUrl ??
      (typeof process !== 'undefined' ? process.env.MEMRO_BASE_URL : undefined) ??
      'http://localhost:8081';

    this.http = axios.create({ baseURL });
  }

  private authHeaders(agentId: string, privateKey: string, body: string) {
    const encoder = new TextEncoder();
    return signBody(agentId, privateKey, encoder.encode(body));
  }

  // ── Identity ──────────────────────────────────────────────────────────────

  async createIdentity(): Promise<{ agent_id: string; private_key: string }> {
    try {
      const { data } = await this.http.post('/identity');
      return data;
    } catch (e) { handleAxiosError(e); }
  }

  async getIdentity(agentId: string): Promise<{ id: string; created_at: string }> {
    try {
      const { data } = await this.http.get(`/identity/${agentId}`);
      return data;
    } catch (e) { handleAxiosError(e); }
  }

  async deleteIdentity(agentId: string, privateKey: string): Promise<void> {
    const headers = signBody(agentId, privateKey, new Uint8Array(0));
    try {
      await this.http.delete(`/identity/${agentId}`, { headers });
    } catch (e) { handleAxiosError(e); }
  }

  // ── Memory ────────────────────────────────────────────────────────────────

  async createMemory(
    agentId: string,
    privateKey: string,
    content: string,
    memoryType: MemoryType = 'episodic',
    visibility: Visibility = 'private'
  ): Promise<Memory> {
    const bodyStr = JSON.stringify({
      agent_id: agentId,
      content,
      memory_type: memoryType,
      visibility,
    });
    const headers = this.authHeaders(agentId, privateKey, bodyStr);
    try {
      const { data } = await this.http.post('/memory', bodyStr, { headers });
      return data;
    } catch (e) { handleAxiosError(e); }
  }

  async getMemories(
    agentId: string,
    options: { memoryType?: MemoryType; visibility?: Visibility; limit?: number } = {}
  ): Promise<Memory[]> {
    const params: Record<string, string | number> = {};
    if (options.memoryType) params.memory_type = options.memoryType;
    if (options.visibility) params.visibility = options.visibility;
    if (options.limit != null) params.limit = Math.min(options.limit, 100);
    try {
      const { data } = await this.http.get(`/memory/${agentId}`, { params });
      return data;
    } catch (e) { handleAxiosError(e); }
  }

  async deleteMemory(agentId: string, privateKey: string, memoryId: string): Promise<boolean> {
    const headers = signBody(agentId, privateKey, new Uint8Array(0));
    try {
      const resp = await this.http.delete(`/memory/${memoryId}`, { headers });
      return resp.status === 204;
    } catch (e) {
      if (axios.isAxiosError(e) && e.response?.status === 404) return false;
      handleAxiosError(e);
    }
  }

  // ── Search ────────────────────────────────────────────────────────────────

  async search(
    agentId: string,
    query: string,
    limit = 10,
    memoryType?: MemoryType
  ): Promise<SearchResult[]> {
    const params: Record<string, string | number> = {
      agent_id: agentId,
      query,
      limit: Math.min(limit, 100),
    };
    if (memoryType) params.memory_type = memoryType;
    try {
      const { data } = await this.http.get('/search', { params });
      return data.results ?? [];
    } catch (e) { handleAxiosError(e); }
  }

  // ── Export ────────────────────────────────────────────────────────────────

  async export(agentId: string): Promise<ExportData> {
    try {
      const { data } = await this.http.get(`/export/${agentId}`);
      return data;
    } catch (e) { handleAxiosError(e); }
  }

  // ── Health ────────────────────────────────────────────────────────────────

  async health(): Promise<HealthStatus> {
    try {
      const { data } = await this.http.get('/health');
      return data;
    } catch (e) { handleAxiosError(e); }
  }
}
