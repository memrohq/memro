/**
 * High-level Agent interface for the memro protocol.
 */
import * as fs from 'fs';
import { MemroClient } from './client';
import type {
  Memory,
  SearchResult,
  ExportData,
  RememberOptions,
  RecallOptions,
  GetRecentOptions,
  AgentIdentity,
} from './types';

export interface AgentOptions {
  client?: MemroClient;
}

export class Agent {
  readonly agentId: string;
  private readonly _privateKey: string;
  readonly client: MemroClient;

  constructor(agentId: string, privateKey: string, client?: MemroClient) {
    this.agentId = agentId;
    this._privateKey = privateKey;
    this.client = client ?? new MemroClient();
  }

  // ── Constructors ──────────────────────────────────────────────────────────

  /** Create a new agent identity on the memro server. */
  static async create(options: AgentOptions = {}): Promise<Agent> {
    const client = options.client ?? new MemroClient();
    const { agent_id, private_key } = await client.createIdentity();
    return new Agent(agent_id, private_key, client);
  }

  /** Load agent identity from a JSON file. */
  static fromFile(path: string, options: AgentOptions = {}): Agent {
    const raw = fs.readFileSync(path, 'utf-8');
    const data: AgentIdentity = JSON.parse(raw);
    return new Agent(data.agent_id, data.private_key, options.client);
  }

  /**
   * Load agent from environment variables.
   * Requires: MEMRO_AGENT_ID, MEMRO_PRIVATE_KEY
   */
  static fromEnv(options: AgentOptions = {}): Agent {
    const agentId = process.env.MEMRO_AGENT_ID;
    const privateKey = process.env.MEMRO_PRIVATE_KEY;
    if (!agentId || !privateKey) {
      throw new Error('MEMRO_AGENT_ID and MEMRO_PRIVATE_KEY must be set');
    }
    return new Agent(agentId, privateKey, options.client);
  }

  /** Load agent from an identity object. */
  static fromIdentity(identity: AgentIdentity, options: AgentOptions = {}): Agent {
    return new Agent(identity.agent_id, identity.private_key, options.client);
  }

  // ── Persistence ───────────────────────────────────────────────────────────

  /** Save agent identity to a JSON file. Keep this file secret. */
  save(path: string): void {
    fs.writeFileSync(
      path,
      JSON.stringify({ agent_id: this.agentId, private_key: this._privateKey }, null, 2)
    );
  }

  toJSON(): AgentIdentity {
    return { agent_id: this.agentId, private_key: this._privateKey };
  }

  // ── Memory operations ─────────────────────────────────────────────────────

  /**
   * Store a new memory.
   * @param content  What to remember (max 10,000 chars)
   * @param options  type, visibility
   */
  async remember(content: string, options: RememberOptions = {}): Promise<Memory> {
    return this.client.createMemory(
      this.agentId,
      this._privateKey,
      content,
      options.type ?? 'episodic',
      options.visibility ?? 'private'
    );
  }

  /**
   * Search memories semantically.
   * @param query  Natural language search query
   * @param options limit, type, minScore
   */
  async recall(query: string, options: RecallOptions = {}): Promise<SearchResult[]> {
    const results = await this.client.search(
      this.agentId,
      query,
      options.limit ?? 10,
      options.type
    );
    if (options.minScore != null) {
      return results.filter((r) => r.score >= options.minScore!);
    }
    return results;
  }

  /** Get most recent memories without semantic search. */
  async getRecent(options: GetRecentOptions = {}): Promise<Memory[]> {
    return this.client.getMemories(this.agentId, {
      memoryType: options.type,
      visibility: options.visibility,
      limit: options.limit,
    });
  }

  /** Delete a specific memory by ID. Returns true if deleted. */
  async deleteMemory(memoryId: string): Promise<boolean> {
    return this.client.deleteMemory(this.agentId, this._privateKey, memoryId);
  }

  /** Export all memories for this agent. */
  async export(): Promise<ExportData> {
    return this.client.export(this.agentId);
  }

  /** Permanently delete this agent and all its memories. */
  async deleteAgent(): Promise<void> {
    return this.client.deleteIdentity(this.agentId, this._privateKey);
  }

  toString(): string {
    return `Agent(id=${this.agentId.slice(0, 16)}...)`;
  }
}
