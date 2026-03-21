-- P3 Optimization: Add relationship types for pre-filtering
-- Stores denormalized relationship types to speed up filtered searches

-- Add relationship_types column to memories table
ALTER TABLE memories 
ADD COLUMN relationship_types TEXT[] DEFAULT '{}';

-- Create index for relationship-based pre-filtering
CREATE INDEX idx_memories_relationship_types_gin 
ON memories USING GIN (relationship_types);

-- Create composite index for agent + relationship filtering
CREATE INDEX idx_memories_agent_relationships 
ON memories (agent_id, created_at DESC) 
INCLUDE (relationship_types);

-- Update existing memories to extract relationships
-- This gets all related memory IDs for each memory
WITH memory_relationships AS (
    SELECT 
        mr.target_memory_id as memory_id,
        ARRAY_AGG(DISTINCT mr.relation_type) as relation_types
    FROM memory_relations mr
    GROUP BY mr.target_memory_id
)
UPDATE memories m
SET relationship_types = COALESCE(mr.relation_types, '{}')
FROM memory_relationships mr
WHERE m.id = mr.memory_id;
