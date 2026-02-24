use axum::{
    extract::{Query, State},
    http::StatusCode,
    Json,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use super::AppState;

#[derive(Debug, Deserialize)]
pub struct SearchQuery {
    pub agent_id: String,
    pub query: String,
    #[serde(default = "default_limit")]
    pub limit: usize,
    pub memory_type: Option<String>,
}

fn default_limit() -> usize {
    10
}

#[derive(Debug, Serialize)]
pub struct SearchResult {
    pub memory_id: String,
    pub content: String,
    pub score: f32,
    pub memory_type: String,
    pub created_at: String,
}

#[derive(Debug, Serialize)]
pub struct SearchResponse {
    pub results: Vec<SearchResult>,
    pub latency_ms: u64,
    pub total: usize,
}

/// Semantic search endpoint
/// GET /search?agent_id=xxx&query=xxx&limit=10
pub async fn search_memories(
    State(state): State<Arc<AppState>>,
    Query(params): Query<SearchQuery>,
) -> Result<Json<SearchResponse>, (StatusCode, String)> {
    let start = std::time::Instant::now();

    // Get OpenAI API key
    let openai_key = std::env::var("OPENAI_API_KEY")
        .map_err(|_| (StatusCode::INTERNAL_SERVER_ERROR, "OPENAI_API_KEY not set".to_string()))?;
    
    let openai = crate::services::OpenAIService::new(openai_key);

    // Generate embedding for search query
    let query_embedding = openai.embed_text(&params.query).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Failed to generate embedding: {}", e)))?;

    // Search vector store
    let memory_ids = state.vector_store
        .search_similar(&params.agent_id, query_embedding, params.limit)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Vector search failed: {}", e)))?;

    // Fetch memory details from database
    let mut results = Vec::new();
    
    for (idx, memory_id) in memory_ids.iter().enumerate() {
        // Query memory from database
        let memory = sqlx::query!(
            r#"
            SELECT id, content, memory_type, created_at
            FROM memories
            WHERE id = $1 AND agent_id = $2
            "#,
            memory_id,
            params.agent_id
        )
        .fetch_optional(&state.db_pool)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Database error: {}", e)))?;

        if let Some(mem) = memory {
            // Calculate score based on position (higher is better)
            let score = 1.0 - (idx as f32 / params.limit as f32);
            
            results.push(SearchResult {
                memory_id: mem.id.to_string(),
                content: mem.content,
                score,
                memory_type: mem.memory_type,
                created_at: mem.created_at.to_rfc3339(),
            });
        }
    }

    let latency = start.elapsed();

    Ok(Json(SearchResponse {
        total: results.len(),
        results,
        latency_ms: latency.as_millis() as u64,
    }))
}
