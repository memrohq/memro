-- Add frequency, recency and confidence tracking fields to memories
ALTER TABLE memories ADD COLUMN access_count INTEGER DEFAULT 0;
ALTER TABLE memories ADD COLUMN last_accessed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW();
ALTER TABLE memories ADD COLUMN confidence FLOAT4 DEFAULT 1.0;

-- Index for performance on ranking signals
CREATE INDEX idx_memories_access_count ON memories(access_count DESC);
CREATE INDEX idx_memories_last_accessed ON memories(last_accessed_at DESC);
CREATE INDEX idx_memories_confidence ON memories(confidence DESC);
