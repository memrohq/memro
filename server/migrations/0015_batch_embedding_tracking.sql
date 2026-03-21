-- P1 Optimization: Batch vector indexing support
-- This migration adds infrastructure for efficient batch memory operations
-- and tracking of bulk embedding jobs

-- Track batch embedding operations for monitoring and recovery
CREATE TABLE IF NOT EXISTS embedding_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    batch_size INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, processing, completed, failed
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    FOREIGN KEY (agent_id, tenant_id) REFERENCES agent_identities(id, tenant_id)
);

-- Index for querying pending batches
CREATE INDEX IF NOT EXISTS idx_embedding_batches_status 
ON embedding_batches(status, agent_id) 
WHERE status != 'completed';

-- Index for finding batches by agent
CREATE INDEX IF NOT EXISTS idx_embedding_batches_agent 
ON embedding_batches(agent_id, created_at DESC);

-- Track individual items in batches for recovery
CREATE TABLE IF NOT EXISTS embedding_batch_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id UUID NOT NULL REFERENCES embedding_batches(id) ON DELETE CASCADE,
    memory_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, embedded, failed
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for querying batch items
CREATE INDEX IF NOT EXISTS idx_embedding_batch_items_batch 
ON embedding_batch_items(batch_id, status);

-- Index for finding failed items
CREATE INDEX IF NOT EXISTS idx_embedding_batch_items_failed 
ON embedding_batch_items(batch_id)
WHERE status = 'failed';

-- Add comment for optimization tracking
COMMENT ON TABLE embedding_batches IS 'P1 Optimization: Track batch embedding operations for monitoring and recovery';
COMMENT ON TABLE embedding_batch_items IS 'P1 Optimization: Track individual items in batches';
COMMENT ON COLUMN embedding_batches.status IS 'P1: pending, processing, completed, or failed';
COMMENT ON COLUMN embedding_batch_items.status IS 'P1: pending, embedded, or failed';
