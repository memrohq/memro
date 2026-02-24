export type MemoryType = 'episodic' | 'semantic' | 'procedural' | 'profile';
export type Visibility = 'private' | 'shared' | 'public';

export interface Memory {
  id: string;
  agent_id: string;
  content: string;
  memory_type: MemoryType;
  visibility: Visibility;
  created_at: string;
}

export interface SearchResult {
  memory_id: string;
  content: string;
  score: number;
  memory_type: MemoryType;
  created_at: string;
}

export interface ExportData {
  agent_id: string;
  exported_at: string;
  total: number;
  memories: Memory[];
}

export interface HealthStatus {
  status: 'ok' | 'degraded';
  db: 'ok' | 'error';
  vector_store: 'ok' | 'unavailable';
}

export interface RememberOptions {
  type?: MemoryType;
  visibility?: Visibility;
}

export interface RecallOptions {
  limit?: number;
  type?: MemoryType;
  minScore?: number;
}

export interface GetRecentOptions {
  limit?: number;
  type?: MemoryType;
  visibility?: Visibility;
}

export interface AgentIdentity {
  agent_id: string;
  private_key: string;
}
