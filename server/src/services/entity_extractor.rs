// P2: Entity Extraction for Pre-filtering
// Add this to backend/src/services/entity_extractor.rs

use regex::Regex;

#[derive(Clone, Debug)]
pub struct EntityExtractor;

impl EntityExtractor {
    /// Extract simple entities from text for pre-filtering
    /// Returns array of lowercase entity tags for GIN index lookup
    pub fn extract_entities(content: &str) -> Vec<String> {
        let mut entities = Vec::new();
        
        // Convert to lowercase for matching
        let content_lower = content.to_lowercase();
        
        // 1. Extract capitalized words (likely proper nouns/names)
        if let Ok(re) = Regex::new(r"\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)*\b") {
            for cap in re.captures_iter(content) {
                let entity = cap[0].to_lowercase();
                if entity.len() > 2 && entity.len() < 50 {
                    entities.push(entity);
                }
            }
        }
        
        // 2. Extract common keywords (actions, entities)
        let keywords = vec![
            ("project", "project"),
            ("work", "work"),
            ("meeting", "meeting"),
            ("call", "call"),
            ("email", "email"),
            ("team", "team"),
            ("manager", "manager"),
            ("colleague", "colleague"),
            ("travel", "travel"),
            ("vacation", "vacation"),
            ("trip", "trip"),
            ("conference", "conference"),
            ("event", "event"),
            ("presentation", "presentation"),
            ("talk", "talk"),
            ("python", "python"),
            ("rust", "rust"),
            ("javascript", "javascript"),
            ("react", "react"),
            ("typescript", "typescript"),
            ("database", "database"),
            ("api", "api"),
            ("server", "server"),
            ("client", "client"),
        ];
        
        for (keyword, tag) in keywords {
            if content_lower.contains(keyword) {
                entities.push(tag.to_string());
            }
        }
        
        // 3. Extract dates/times
        if let Ok(re) = Regex::new(r"\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec|Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday|yesterday|today|tomorrow)\b") {
            for cap in re.captures_iter(&content_lower) {
                entities.push(format!("time:{}", &cap[0]));
            }
        }
        
        // 4. Extract email-like patterns (person identifiers)
        if let Ok(re) = Regex::new(r"\b[A-Za-z]+@[A-Za-z]+\b") {
            for cap in re.captures_iter(content) {
                let email = cap[0].to_lowercase();
                if let Some(name) = email.split('@').next() {
                    entities.push(format!("person:{}", name));
                }
            }
        }
        
        // Deduplicate and sort for consistency
        entities.sort();
        entities.dedup();
        
        // Limit to top entities to keep array reasonable size
        entities.truncate(20);
        
        entities
    }
    
    /// Extract entities from a query for filtering
    pub fn extract_query_entities(query: &str) -> Vec<String> {
        let mut entities = Vec::new();
        let query_lower = query.to_lowercase();
        
        // Extract capitalized words
        if let Ok(re) = Regex::new(r"\b[A-Z][a-z]+\b") {
            for cap in re.captures_iter(query) {
                let entity = cap[0].to_lowercase();
                if entity.len() > 2 {
                    entities.push(entity);
                }
            }
        }
        
        // Extract keywords
        let keywords = vec![
            "project", "work", "meeting", "team", "travel", "vacation",
            "trip", "conference", "python", "rust", "javascript", "react",
            "typescript", "database", "api", "server", "client",
        ];
        
        for keyword in keywords {
            if query_lower.contains(keyword) {
                entities.push(keyword.to_string());
            }
        }
        
        entities.sort();
        entities.dedup();
        entities.truncate(10);
        
        entities
    }
    
    /// Pre-filter candidates from PostgreSQL before vector search
    /// This significantly reduces the search space
    pub fn generate_prefilter_sql(
        agent_id: &str,
        tenant_id: &str,
        entities: &[String],
        days_back: i32,
        limit: i64,
    ) -> (String, Vec<String>) {
        // Build dynamic query based on entities
        if entities.is_empty() {
            // No entities - just return recent memories
            let sql = format!(
                r#"
                SELECT id FROM memories
                WHERE agent_id = $1 
                  AND tenant_id = $2
                  AND created_at > now() - interval '{} days'
                ORDER BY created_at DESC
                LIMIT {}
                "#,
                days_back, limit
            );
            (sql, vec![agent_id.to_string(), tenant_id.to_string()])
        } else {
            // Filter by entities
            let sql = format!(
                r#"
                SELECT id FROM memories
                WHERE agent_id = $1 
                  AND tenant_id = $2
                  AND created_at > now() - interval '{} days'
                  AND (entity_tags && $3::TEXT[] OR memory_type = 'semantic')
                ORDER BY created_at DESC
                LIMIT {}
                "#,
                days_back, limit
            );
            let mut params = vec![agent_id.to_string(), tenant_id.to_string()];
            params.push(serde_json::to_string(entities).unwrap_or_default());
            (sql, params)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_extract_entities() {
        let content = "Completed project Alpha using React and TypeScript";
        let entities = EntityExtractor::extract_entities(content);
        assert!(entities.iter().any(|e| e == "project"));
        assert!(entities.iter().any(|e| e == "react"));
        assert!(entities.iter().any(|e| e == "typescript"));
    }
    
    #[test]
    fn test_extract_query_entities() {
        let query = "React and TypeScript projects";
        let entities = EntityExtractor::extract_query_entities(query);
        assert!(entities.iter().any(|e| e == "react"));
        assert!(entities.iter().any(|e| e == "typescript"));
    }
}
