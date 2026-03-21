-- Add Full-Text Search GIN index to memories content for hybrid retrieval
-- This allows for fast keyword-based lookups alongside vector search

-- Create a generated column for the search vector if it doesn't exist, 
-- or just index the content directly using to_tsvector
CREATE INDEX IF NOT EXISTS idx_memories_content_fts ON memories USING GIN (to_tsvector('english', content));
