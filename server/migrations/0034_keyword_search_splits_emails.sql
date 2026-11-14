-- Keyword search finds the parts of email addresses and domains: Postgres
-- keeps "jane@stripe.com" as one token, so "stripe" or "jane" never matched.
-- memro_tsv indexes the original words plus the text with @ and . between
-- word characters turned into spaces. Queries must use the same expression to
-- hit the index.
CREATE OR REPLACE FUNCTION memro_tsv(content TEXT) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT to_tsvector('english', content)
        || to_tsvector('english', regexp_replace(content, '(\w)[@.](?=\w)', '\1 ', 'g'))
$$;

-- 0014 duplicated 0009's index. 0009's stays until a later release, so a
-- rollback to code without memro_tsv keeps an index for its keyword search.
DROP INDEX IF EXISTS idx_memories_fts_agent;
CREATE INDEX IF NOT EXISTS idx_memories_tsv ON memories USING GIN (memro_tsv(content));
