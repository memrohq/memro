-- Add files table for multimodal support
CREATE TABLE IF NOT EXISTS files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_type TEXT NOT NULL, -- pdf, image, audio, video, text
    file_path TEXT NOT NULL,
    file_size BIGINT NOT NULL,
    mime_type TEXT NOT NULL,
    extracted_text TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Add index for agent_id lookups
CREATE INDEX IF NOT EXISTS idx_files_agent_id ON files(agent_id);

-- Add file_id column to memories table
ALTER TABLE memories ADD COLUMN IF NOT EXISTS file_id UUID REFERENCES files(id);

-- Add index for file_id lookups
CREATE INDEX IF NOT EXISTS idx_memories_file_id ON memories(file_id);

-- Add embeddings table for vector storage metadata
CREATE TABLE IF NOT EXISTS embeddings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    vector_dimension INTEGER NOT NULL DEFAULT 1536,
    embedding_model TEXT NOT NULL DEFAULT 'text-embedding-3-small',
    indexed_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(memory_id)
);

-- Add index for memory_id lookups
CREATE INDEX IF NOT EXISTS idx_embeddings_memory_id ON embeddings(memory_id);
