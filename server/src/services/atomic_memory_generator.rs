use anyhow::Result;
use serde::{Deserialize, Serialize};

/// AtomicMemory represents a single, indivisible piece of information
/// Atomic memories are generated from chunks to resolve ambiguity
/// 
/// Example:
/// Chunk: "She mentioned her favorite hobby is rock climbing and she does it every weekend"
/// Atomic Memories:
/// - "User's favorite hobby is rock climbing"
/// - "User climbs every weekend"
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AtomicMemory {
    pub fact: String,
    pub confidence: f32, // How confident are we this is correct (0.0-1.0)
    pub source_chunk_id: Option<String>, // Reference back to source chunk
    pub entities: Vec<String>, // Extracted entities (e.g., names, locations)
    pub relations: Vec<String>, // Extracted relations (e.g., "likes", "works_at")
}

/// Service for generating atomic memories from chunks
/// Uses LLM to extract singular facts and resolve ambiguities
pub struct AtomicMemoryGenerator;

impl AtomicMemoryGenerator {
    /// Generate atomic memories from a chunk of text
    /// This is a simplified version - in production, call an LLM with the prompt below
    pub fn generate_from_chunk(
        chunk: &str,
        context: Option<&str>, // Previous context to resolve references
    ) -> Result<Vec<AtomicMemory>> {
        // For MVP: Use simple heuristics to extract atomic facts
        // For production: Replace with LLM call
        
        let atomic_memories = Self::extract_sentences_as_atoms(chunk, context);
        Ok(atomic_memories)
    }

    /// Simple implementation: split chunk into sentences and treat each as atomic
    /// Production version would use LLM for smarter extraction
    fn extract_sentences_as_atoms(
        chunk: &str,
        _context: Option<&str>,
    ) -> Vec<AtomicMemory> {
        let mut atoms = Vec::new();
        
        // Split by common sentence delimiters
        for sentence in chunk.split(|c| c == '.' || c == '!' || c == '?') {
            let trimmed = sentence.trim();
            if trimmed.is_empty() {
                continue;
            }
            
            atoms.push(AtomicMemory {
                fact: trimmed.to_string(),
                confidence: 0.8, // Slightly lower for automatic extraction
                source_chunk_id: None,
                entities: Self::extract_entities(trimmed),
                relations: Self::extract_relations(trimmed),
            });
        }
        
        atoms
    }

    /// Extract named entities using simple patterns
    /// In production, use a proper NER model
    fn extract_entities(text: &str) -> Vec<String> {
        let mut entities = Vec::new();
        
        // Very basic: capitalize words that might be names/locations
        for word in text.split_whitespace() {
            if word.len() > 3 && word.chars().next().map(|c| c.is_uppercase()).unwrap_or(false) {
                entities.push(word.to_string());
            }
        }
        
        entities
    }

    /// Extract relations using keyword matching
    /// In production, use a proper relation extraction model
    fn extract_relations(text: &str) -> Vec<String> {
        let relations = vec![
            ("likes", "prefers"),
            ("loves", "enjoys"),
            ("hates", "dislikes"),
            ("works at", "employed_at"),
            ("lives in", "residence"),
            ("studies", "learning"),
        ];
        
        let mut found_relations = Vec::new();
        let text_lower = text.to_lowercase();
        
        for (relation, alias) in &relations {
            if text_lower.contains(relation) {
                found_relations.push(alias.to_string());
            }
        }
        
        found_relations
    }

    /// PROMPT FOR LLM-BASED EXTRACTION (use this for production version)
    /// 
    /// System Prompt:
    /// ```
    /// You are an expert at extracting atomic facts from text.
    /// Atomic facts are singular, indivisible pieces of information.
    /// 
    /// Guidelines:
    /// 1. Each atomic fact should be complete and understandable on its own
    /// 2. Resolve ambiguous references using the provided context
    /// 3. Extract exactly one fact per line
    /// 4. Include confidence score (0.0-1.0) for each fact
    /// 5. Identify entities mentioned (names, places, concepts)
    /// 6. Identify relations (likes, works_at, etc.)
    /// 
    /// Example Input:
    /// "John loves hiking in the mountains. He goes every weekend with his friend Alice."
    /// 
    /// Example Output:
    /// ```json
    /// [
    ///   {
    ///     "fact": "John loves hiking",
    ///     "confidence": 0.95,
    ///     "entities": ["John", "hiking"],
    ///     "relations": ["loves"]
    ///   },
    ///   {
    ///     "fact": "John hikes in mountains",
    ///     "confidence": 0.9,
    ///     "entities": ["John", "mountains"],
    ///     "relations": ["hikes_in"]
    ///   },
    ///   {
    ///     "fact": "John goes hiking every weekend",
    ///     "confidence": 0.85,
    ///     "entities": ["John"],
    ///     "relations": ["frequency_weekly"]
    ///   },
    ///   {
    ///     "fact": "John's friend is Alice",
    ///     "confidence": 0.9,
    ///     "entities": ["John", "Alice"],
    ///     "relations": ["friend"]
    ///   }
    /// ]
    /// ```
    /// 
    /// User Prompt:
    /// ```
    /// Extract atomic facts from this text:
    /// "{chunk}"
    /// 
    /// Context (previous conversation): "{context}"
    /// 
    /// Return as JSON array with structure:
    /// [
    ///   { "fact": string, "confidence": float, "entities": [string], "relations": [string] }
    /// ]
    /// ```
    pub const EXTRACTION_PROMPT_TEMPLATE: &'static str = r#"
You are an expert at extracting atomic facts from text for a memory system.

Atomic facts are singular, indivisible pieces of information that:
1. Can be understood on their own
2. Have resolved all ambiguous references (including pronouns)
3. Capture one concept or relationship
4. Are extracted precisely from the source text

Your task:
1. Read the provided text chunk
2. Extract all atomic facts, resolving pronouns using the context
3. For each fact, identify entities and relationships mentioned
4. Provide a confidence score (0.0-1.0) for each fact

Important:
- Resolve "she", "he", "it", "they" using context
- One fact per item in the array
- Be precise and faithful to the original text
- Score higher confidence for facts directly stated vs. inferred

Return ONLY valid JSON in this format:
[
  {
    "fact": "description of one atomic fact",
    "confidence": 0.95,
    "entities": ["entity1", "entity2"],
    "relations": ["relation1", "relation2"]
  }
]

Text to analyze:
{chunk}

Context (resolves pronouns):
{context}
"#;
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_extract_sentences() {
        let chunk = "I love hiking. I go every weekend. My favorite place is the mountains.";
        let atoms = AtomicMemoryGenerator::extract_sentences_as_atoms(chunk, None);
        assert_eq!(atoms.len(), 3);
    }

    #[test]
    fn test_extract_entities() {
        let text = "John loves hiking in the mountains";
        let entities = AtomicMemoryGenerator::extract_entities(text);
        assert!(entities.contains(&"John".to_string()));
    }

    #[test]
    fn test_extract_relations() {
        let text = "John loves hiking in the mountains";
        let relations = AtomicMemoryGenerator::extract_relations(text);
        assert!(!relations.is_empty());
    }
}
