-- P2 Optimization: Add entity tags for pre-filtering
-- Stores denormalized entity tags to speed up filtered searches

-- Add entity_tags column to memories table if it doesn't exist
ALTER TABLE memories 
ADD COLUMN IF NOT EXISTS entity_tags TEXT[] DEFAULT '{}';

-- Update existing NULL entity_tags to empty array to prevent panics
UPDATE memories SET entity_tags = '{}' WHERE entity_tags IS NULL;

-- Create index for entity-based pre-filtering if it doesn't exist
CREATE INDEX IF NOT EXISTS idx_memories_entity_tags_gin 
ON memories USING GIN (entity_tags);

-- Create composite index for agent + entity filtering if it doesn't exist
CREATE INDEX IF NOT EXISTS idx_memories_agent_entity_tags 
ON memories (agent_id, entity_tags);
