/**
 * memro — AI agent memory infrastructure SDK for JavaScript/TypeScript.
 *
 * Quickstart:
 *   import { Agent } from '@memro/sdk';
 *
 *   const agent = await Agent.create();
 *   await agent.remember('User prefers TypeScript', { type: 'profile' });
 *   const results = await agent.recall('user preferences');
 *   agent.save('identity.json');
 */
export { Agent } from './agent';
export { MemroClient, MemroError } from './client';
export { generateKeypair, signBody } from './crypto';
export type {
  Memory,
  SearchResult,
  ExportData,
  HealthStatus,
  MemoryType,
  Visibility,
  RememberOptions,
  RecallOptions,
  GetRecentOptions,
  AgentIdentity,
} from './types';
