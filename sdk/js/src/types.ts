export type MemoryType = 'episodic' | 'semantic' | 'procedural' | 'profile';
export type Visibility = 'private' | 'shared' | 'public';
export type SearchMode = 'hybrid' | 'semantic' | 'keyword';

export interface Memory {
  id: string;
  agent_id: string;
  content: string;
  memory_type: MemoryType;
  visibility: Visibility;
  created_at: string;
  document_date?: string;
  event_date?: string | null;
  session_id?: string | null;
  is_atomic?: boolean;
  entity_tags?: string[];
  confidence?: number;
  access_count?: number;
  file_id?: string | null;
  asset_url?: string | null;
  metadata?: Record<string, unknown> | null;
}

export interface ResultGraphInfo {
  seed_id: string;
  path: string[];
  relation_types: string[];
  hops: number;
}

export interface SearchResult {
  memory_id: string;
  content: string;
  score: number;
  memory_type: MemoryType;
  created_at: string;
  document_date?: string | null;
  event_date?: string | null;
  session_id?: string | null;
  asset_url?: string | null;
  is_superseded?: boolean;
  superseded_by?: string | null;
  graph?: ResultGraphInfo;
}

export interface GraphDiagnostics {
  applied: boolean;
  mode?: string;
  seeds?: number;
  edges_examined?: number;
  paths?: number;
  nodes_added?: number;
  truncated?: boolean;
  elapsed_ms?: number;
  reason?: string;
}

export interface SearchResponse {
  results: SearchResult[];
  mode: SearchMode;
  latency_ms: number;
  total: number;
  ledger?: string | null;
  graph?: GraphDiagnostics;
}

export interface TemporalResult {
  memory_id: string;
  content: string;
  document_date: string;
  event_date: string | null;
  is_atomic: boolean;
  score: number;
  temporal_context: string;
}

export interface ExportData {
  agent_id: string;
  exported_at: string;
  total: number;
  memories: Memory[];
}

export interface ReasoningResult {
  answer: string;
  why: string;
  based_on: string[];
  confidence: number;
  paths: string[][];
}

/** `{ Resolved: id }` or `{ Ambiguous: [a, b, expiresAt] }`. */
export type ResolutionResult = { Resolved: string } | { Ambiguous: [string, string, string | null] };

export interface EvolutionResult {
  memories_evolved: number;
  patterns_identified: string[];
  diff: unknown;
}

export interface MemoryTimeline {
  memory_id: string;
  events: { timestamp: string; action: string; details: string }[];
}

export interface ChainItem {
  memory_id: string;
  content: string;
  relation_type: string | null;
  confidence: number | null;
}

export interface MemoryChain {
  root_memory: ChainItem;
  related_memories: ChainItem[];
}

export interface TemporalStats {
  total_memories: number;
  memories_today: number;
  memories_this_week: number;
  memories_this_month: number;
  average_days_between_memories: number;
  memory_types_distribution: Record<string, number>;
}

export interface ConflictAnalysis {
  total_conflicts: number;
  unresolved_conflicts: number;
  avg_resolution_time_sec: number;
}

export interface Usage {
  reason_calls: number;
  evolve_calls: number;
  memory_nodes: number;
  graph_edges: number;
  warning: string | null;
}

export interface UploadedFile {
  file_id: string;
  file_name: string;
  file_type: string;
  file_size: number;
  extracted_text: string | null;
  /** Set when the upload was also stored as a memory. */
  memory_id?: string;
}

export interface WhoAmI {
  agent_id: string;
  tenant_id: string;
  user_id: string | null;
  auth: 'api_key' | 'session' | 'signature';
}

export interface HealthStatus {
  status: 'ok' | 'degraded';
  db: 'ok' | 'error';
  vector_store: 'ok' | 'unavailable';
  embeddings: 'enabled' | 'disabled';
}

export interface RememberOptions {
  /** Omit (or 'auto') to let the server classify the memory. */
  type?: MemoryType | 'auto';
  visibility?: Visibility;
  /** When it happened, RFC 3339. Defaults to a date found in the text. */
  eventDate?: string;
  /** UUID grouping memories from one conversation. */
  sessionId?: string;
  isAtomic?: boolean;
  metadata?: Record<string, unknown>;
}

export interface RecallOptions {
  limit?: number;
  type?: MemoryType;
  mode?: SearchMode;
  /** Drop results scoring below this (0..1). */
  minScore?: number;
  graph?: 'legacy' | 'bounded';
}

export interface TemporalOptions {
  limit?: number;
  afterDate?: string;
  beforeDate?: string;
  afterEventDate?: string;
  beforeEventDate?: string;
  sessionId?: string;
  atomicOnly?: boolean;
  /** How much recency counts, 0..1 (default 0.3). */
  temporalWeight?: number;
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

/** A file to upload: a Blob/File, or raw bytes with a name. */
export type FileInput = Blob | { name: string; data: Uint8Array | ArrayBuffer; type?: string };
