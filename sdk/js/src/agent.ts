/**
 * An AI agent with persistent memory.
 *
 *   const agent = await Agent.fromApiKey(process.env.MEMRO_API_KEY!);
 *   await agent.remember('User prefers TypeScript', { type: 'profile' });
 *   const results = await agent.recall('language preferences');
 */
import { MemroClient, type MemroClientOptions, type Signer } from './client.js';
import type {
  AgentIdentity, ExportData, FileInput, GetRecentOptions, Memory, MemoryChain, MemoryTimeline,
  ReasoningResult, RecallOptions, RememberOptions, ResolutionResult, SearchResult, TemporalOptions,
  TemporalResult, TemporalStats, UploadedFile, Usage, EvolutionResult, ConflictAnalysis, MemoryType,
} from './types.js';

export class Agent {
  /** The agent's ID (its ed25519 public key, hex). */
  readonly agentId: string;
  readonly client: MemroClient;
  private readonly signer?: Signer;

  /** Prefer the static constructors. privateKey is only for self-owned identities. */
  constructor(agentId: string, client: MemroClient, privateKey?: string) {
    this.agentId = agentId;
    this.client = client;
    if (privateKey) this.signer = { agentId, privateKey };
  }

  // ── Constructors ────────────────────────────────────────────────────────

  /** Connect with an API key (memro_sk_...) from `npx memro-cli connect` or the dashboard. */
  static async fromApiKey(apiKey: string, options: Omit<MemroClientOptions, 'apiKey'> = {}): Promise<Agent> {
    const client = new MemroClient({ ...options, apiKey });
    const who = await client.whoami();
    return new Agent(who.agent_id, client);
  }

  /** MEMRO_API_KEY, or MEMRO_AGENT_ID + MEMRO_PRIVATE_KEY (and optional MEMRO_BASE_URL). */
  static async fromEnv(options: Omit<MemroClientOptions, 'apiKey'> = {}): Promise<Agent> {
    const e = typeof process !== 'undefined' ? process.env : {};
    if (e.MEMRO_API_KEY) return Agent.fromApiKey(e.MEMRO_API_KEY, options);
    if (e.MEMRO_AGENT_ID && e.MEMRO_PRIVATE_KEY) {
      return new Agent(e.MEMRO_AGENT_ID, new MemroClient({ ...options, apiKey: undefined }), e.MEMRO_PRIVATE_KEY);
    }
    throw new Error('memro: set MEMRO_API_KEY, or MEMRO_AGENT_ID and MEMRO_PRIVATE_KEY');
  }

  /**
   * Create a self-owned identity: the key pair is generated here and only the
   * public key is sent. Save it with `save()` or `toJSON()`; it can't be recovered.
   */
  static async create(options: Omit<MemroClientOptions, 'apiKey'> = {}): Promise<Agent> {
    const client = new MemroClient({ ...options, apiKey: undefined });
    const id = await client.createIdentity();
    return new Agent(id.agent_id, client, id.private_key);
  }

  static fromIdentity(identity: AgentIdentity, options: Omit<MemroClientOptions, 'apiKey'> = {}): Agent {
    return new Agent(identity.agent_id, new MemroClient({ ...options, apiKey: undefined }), identity.private_key);
  }

  /** Node.js only. */
  static async fromFile(path: string, options: Omit<MemroClientOptions, 'apiKey'> = {}): Promise<Agent> {
    const { readFile } = await import('node:fs/promises');
    return Agent.fromIdentity(JSON.parse(await readFile(path, 'utf8')), options);
  }

  /** Node.js only. Writes the identity with owner-only permissions. Keep it secret. */
  async save(path: string): Promise<void> {
    const { writeFile } = await import('node:fs/promises');
    await writeFile(path, JSON.stringify(this.toJSON(), null, 2) + '\n', { mode: 0o600 });
  }

  toJSON(): AgentIdentity {
    if (!this.signer) throw new Error('memro: this agent uses an API key and has no private key to save');
    return { agent_id: this.agentId, private_key: this.signer.privateKey };
  }

  // ── Memories ────────────────────────────────────────────────────────────

  /** Store a memory (max 10,000 characters). */
  remember(content: string, options: RememberOptions = {}): Promise<Memory> {
    const body: Record<string, unknown> = { agent_id: this.agentId, content, visibility: options.visibility ?? 'private' };
    if (options.type) body.memory_type = options.type;
    if (options.eventDate) body.event_date = options.eventDate;
    if (options.sessionId) body.session_id = options.sessionId;
    if (options.isAtomic !== undefined) body.is_atomic = options.isAtomic;
    if (options.metadata) body.metadata = options.metadata;
    return this.client.createMemory(body, this.signer);
  }

  /** Search memories (hybrid by default). */
  async recall(query: string, options: RecallOptions = {}): Promise<SearchResult[]> {
    const res = await this.client.search({
      agent_id: this.agentId, query, limit: Math.min(options.limit ?? 10, 50),
      memory_type: options.type, mode: options.mode ?? 'hybrid',
    }, this.signer);
    return options.minScore === undefined ? res.results : res.results.filter((r) => r.score >= options.minScore!);
  }

  /** Search with date filters and a recency boost. */
  async recallTemporal(query: string, options: TemporalOptions = {}): Promise<TemporalResult[]> {
    const res = await this.client.searchTemporal({
      agent_id: this.agentId, query, limit: Math.min(options.limit ?? 10, 50),
      after_date: options.afterDate, before_date: options.beforeDate,
      after_event_date: options.afterEventDate, before_event_date: options.beforeEventDate,
      session_id: options.sessionId, atomic_only: options.atomicOnly, temporal_weight: options.temporalWeight,
    }, this.signer);
    return res.results;
  }

  /** Most recent memories, newest first (at most 100). */
  getRecent(options: GetRecentOptions = {}): Promise<Memory[]> {
    return this.client.listMemories(this.agentId, {
      limit: options.limit ?? 10, memory_type: options.type, visibility: options.visibility,
    }, this.signer);
  }

  /** Returns false if the memory does not exist. */
  deleteMemory(memoryId: string): Promise<boolean> { return this.client.deleteMemory(memoryId, this.signer); }
  export(): Promise<ExportData> { return this.client.export(this.agentId, this.signer); }

  // ── Memory engine ───────────────────────────────────────────────────────

  reason(query: string): Promise<ReasoningResult> { return this.client.reason(query, this.signer); }
  resolveConflict(memoryAId: string, memoryBId: string): Promise<ResolutionResult> {
    return this.client.resolveConflict(memoryAId, memoryBId, this.signer);
  }
  evolve(): Promise<EvolutionResult> { return this.client.evolve(this.signer); }
  timeline(memoryId: string): Promise<MemoryTimeline> { return this.client.timeline(memoryId, this.signer); }
  memoryChain(memoryId: string): Promise<MemoryChain> { return this.client.memoryChain(memoryId, this.signer); }
  analyzeConflicts(): Promise<ConflictAnalysis> { return this.client.analyzeConflicts(this.signer); }
  stats(): Promise<TemporalStats> { return this.client.stats(this.signer); }
  usage(): Promise<Usage> { return this.client.usage(this.signer); }

  // ── Files ───────────────────────────────────────────────────────────────

  /** Upload a file and, by default, store its extracted text as a memory. */
  async uploadFile(file: FileInput, options: { remember?: boolean; type?: MemoryType } = {}): Promise<UploadedFile> {
    const up = await this.client.uploadFile(this.agentId, file, this.signer);
    if (options.remember ?? true) {
      const res = await this.client.memoryFromFile({
        agent_id: this.agentId, file_id: up.file_id, memory_type: options.type ?? 'semantic',
      }, this.signer);
      up.memory_id = res.memory_id;
    }
    return up;
  }
  /** Describe an image with vision and store the description. */
  uploadImage(file: FileInput): Promise<Memory> { return this.client.uploadImage(file, this.signer); }
  /** Store a PDF, Word, PowerPoint, Excel or text document as ~1,500-character overlapping memories. */
  uploadDocument(file: FileInput): Promise<Memory[]> { return this.client.uploadDocument(file, this.signer); }

  // ── Webhooks ────────────────────────────────────────────────────────────

  /** POST each new memory of this agent to `url` (public http(s) only). */
  addWebhook(url: string, eventType = 'memory_created'): Promise<void> {
    return this.client.registerWebhook(url, eventType, this.signer);
  }
  /** Returns false if the webhook was not registered. */
  removeWebhook(url: string, eventType = 'memory_created'): Promise<boolean> {
    return this.client.deleteWebhook(url, eventType, this.signer);
  }
  webhooks(eventType = 'memory_created'): Promise<string[]> { return this.client.listWebhooks(eventType, this.signer); }

  /** Permanently delete this agent and everything it stored. Needs the agent's private key: API keys get 403. */
  deleteAgent(): Promise<void> { return this.client.deleteIdentity(this.agentId, this.signer); }

  // ── Coordination ────────────────────────────────────────────────────────

  claim(resource: string, options?: { intent?: string; sessionId?: string; ttlSeconds?: number }): Promise<Record<string, unknown>> {
    return this.client.coordinationClaim({
      resource,
      intent: options?.intent ?? '',
      session_id: options?.sessionId,
      ttl_seconds: options?.ttlSeconds,
    }, this.signer);
  }

  heartbeat(options: { resource?: string; leaseId?: string; ttlSeconds?: number }): Promise<Record<string, unknown>> {
    return this.client.coordinationHeartbeat({
      resource: options.resource,
      lease_id: options.leaseId,
      ttl_seconds: options.ttlSeconds,
    }, this.signer);
  }

  release(resource: string, status = 'completed'): Promise<Record<string, unknown>> {
    return this.client.coordinationRelease({ resource, status }, this.signer);
  }

  handoff(resource: string, summary: string, nextAgentId?: string): Promise<Record<string, unknown>> {
    return this.client.coordinationHandoff({ resource, summary, next_agent_id: nextAgentId }, this.signer);
  }

  check(resource: string, intent = ''): Promise<Record<string, unknown>> {
    return this.client.coordinationCheck({ resource, intent }, this.signer);
  }

  active(prefix?: string): Promise<Record<string, unknown>> {
    return this.client.coordinationActive(prefix ? { resource: prefix } : undefined, this.signer);
  }

  logIntent(resource: string, intent: string, ttlSeconds?: number): Promise<Record<string, unknown>> {
    return this.client.coordinationIntent({ resource, intent, ttl_seconds: ttlSeconds }, this.signer);
  }

  toString(): string { return `Agent(${this.agentId.slice(0, 16)}…)`; }
}
