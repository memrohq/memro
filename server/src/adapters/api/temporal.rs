use axum::{
    extract::{Query, State},
    http::StatusCode,
    Json,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use crate::adapters::api::{AppState, middleware};

/// Temporal search query parameters
#[derive(Debug, Deserialize)]
pub struct TemporalSearchParams {
    pub agent_id: String,
    pub query: String,
    pub after_date: Option<String>, // ISO 8601 date for document_date filter
    pub before_date: Option<String>,
    pub after_event_date: Option<String>,
    pub before_event_date: Option<String>,
    pub session_id: Option<String>,
    pub atomic_only: Option<bool>,
    pub temporal_weight: Option<f32>, // 0.0-1.0, default 0.3
    #[serde(default = "default_limit")]
    pub limit: i64,
}

fn default_limit() -> i64 {
    10
}

#[derive(Debug, Serialize)]
pub struct TemporalSearchResponseItem {
    pub memory_id: String,
    pub content: String,
    pub document_date: String,
    pub event_date: Option<String>,
    pub is_atomic: bool,
    pub score: f32,
    pub temporal_context: String,
}

#[derive(Debug, Serialize)]
pub struct TemporalSearchResponse {
    pub results: Vec<TemporalSearchResponseItem>,
    pub total: usize,
}

/// Search with temporal reasoning
/// 
/// Implements Supermemory-style temporal search that considers:
/// - When the conversation happened (document_date)
/// - When the event described actually occurred (event_date)
/// - Multi-session synthesis
/// - Atomic vs chunk memories
pub async fn search_temporal(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    Query(params): Query<TemporalSearchParams>,
) -> Result<Json<TemporalSearchResponse>, (StatusCode, String)> {
    // 1. Parse date parameters
    let after_date = params.after_date.as_ref()
        .and_then(|d| chrono::DateTime::parse_from_rfc3339(d).ok())
        .map(|dt| dt.with_timezone(&chrono::Utc));
    let before_date = params.before_date.as_ref()
        .and_then(|d| chrono::DateTime::parse_from_rfc3339(d).ok())
        .map(|dt| dt.with_timezone(&chrono::Utc));
    let after_event = params.after_event_date.as_ref()
        .and_then(|d| chrono::DateTime::parse_from_rfc3339(d).ok())
        .map(|dt| dt.with_timezone(&chrono::Utc));
    let before_event = params.before_event_date.as_ref()
        .and_then(|d| chrono::DateTime::parse_from_rfc3339(d).ok())
        .map(|dt| dt.with_timezone(&chrono::Utc));
    let sid = params.session_id.as_ref().and_then(|id| uuid::Uuid::parse_str(id).ok());

    // 2. Call vector search first if query is provided
    let embedding = state.embedding_provider.embed_text(&params.query).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Embedding failed: {}", e)))?;
    
    // Search with 3x limit to allow for temporal filtering
    let search_limit = (params.limit * 3) as usize;
    let vector_results = state.vector_store.search_similar(
        &params.agent_id,
        &agent.tenant_id,
        embedding,
        search_limit
    ).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Vector search failed: {}", e)))?;

    // 3. Fetch full memory objects and apply temporal filters
    let mut filtered_results = Vec::new();
    for (memory_id, score) in vector_results {
        if let Some(memory) = state.memory_cache.get_memory(&memory_id) {
            // Apply filters
            if let Some(after) = after_date { if memory.document_date < after { continue; } }
            if let Some(before) = before_date { if memory.document_date > before { continue; } }
            if let Some(after) = after_event { 
                if let Some(ed) = memory.event_date { if ed < after { continue; } } else { continue; }
            }
            if let Some(before) = before_event {
                if let Some(ed) = memory.event_date { if ed > before { continue; } } else { continue; }
            }
            if let Some(s) = sid { if memory.session_id != Some(s) { continue; } }
            if let Some(atomic) = params.atomic_only { if memory.is_atomic != atomic { continue; } }

            // 4. Calculate temporal score boosting (Supermemory-style)
            let temporal_weight = params.temporal_weight.unwrap_or(0.3);
            let time_diff = (chrono::Utc::now() - memory.document_date).num_days() as f32;
            let time_score = 1.0 / (1.0 + time_diff / 30.0); // Simple decay over 30 days
            let boosted_score = score * (1.0 - temporal_weight) + time_score * temporal_weight;

            filtered_results.push(TemporalSearchResponseItem {
                memory_id: memory_id.to_string(),
                content: memory.content.clone(),
                document_date: memory.document_date.to_rfc3339(),
                event_date: memory.event_date.map(|d| d.to_rfc3339()),
                is_atomic: memory.is_atomic,
                score: boosted_score,
                temporal_context: format!("Observed: {}, Occurred: {}", 
                    memory.document_date.format("%Y-%m-%d"),
                    memory.event_date.map_or("N/A".to_string(), |d| d.format("%Y-%m-%d").to_string())),
            });
        }
    }

    // Sort by boosted score and limit
    filtered_results.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap());
    filtered_results.truncate(params.limit as usize);

    Ok(Json(TemporalSearchResponse {
        total: filtered_results.len(),
        results: filtered_results,
    }))
}

/// Get memory chain (knowledge graph)
/// Returns a single memory and its related memories (updates, extends, derives)
#[derive(Debug, Serialize)]
pub struct MemoryChainItem {
    pub memory_id: String,
    pub content: String,
    pub relation_type: Option<String>, // "updates", "extends", "derives"
    pub confidence: Option<f32>,
}

#[derive(Debug, Serialize)]
pub struct MemoryChainResponse {
    pub root_memory: MemoryChainItem,
    pub related_memories: Vec<MemoryChainItem>,
}

pub async fn get_memory_chain(
    State(_state): State<Arc<AppState>>,
    axum::extract::Extension(_agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
) -> Result<Json<MemoryChainResponse>, (StatusCode, String)> {
    Err((
        StatusCode::NOT_IMPLEMENTED,
        "Memory chain endpoint implementation in progress".to_string(),
    ))
}

/// Query temporal statistics
/// Returns insights about memory distribution over time
pub async fn get_temporal_stats(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
) -> Result<Json<TemporalStats>, (StatusCode, String)> {
    let now = chrono::Utc::now();
    let today = now - chrono::Duration::days(1);
    let week = now - chrono::Duration::weeks(1);
    let month = now - chrono::Duration::days(30);

    // This is a simplified implementation - in production, use dedicated SQL queries for efficiency
    let memories = state.memory_store.get_memories_by_agent(&agent.id, &agent.tenant_id).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Failed to fetch memories: {}", e)))?;

    let mut stats = TemporalStats {
        total_memories: memories.len() as i64,
        memories_today: 0,
        memories_this_week: 0,
        memories_this_month: 0,
        average_days_between_memories: 0.0,
        memory_types_distribution: std::collections::HashMap::new(),
    };

    for m in &memories {
        if m.document_date >= today { stats.memories_today += 1; }
        if m.document_date >= week { stats.memories_this_week += 1; }
        if m.document_date >= month { stats.memories_this_month += 1; }
        
        let type_key = format!("{:?}", m.memory_type);
        *stats.memory_types_distribution.entry(type_key).or_insert(0) += 1;
    }

    if memories.len() > 1 {
        let span = memories.first().unwrap().document_date - memories.last().unwrap().document_date;
        stats.average_days_between_memories = span.num_days() as f64 / memories.len() as f64;
    }

    Ok(Json(stats))
}

pub async fn analyze_conflicts(
    State(_state): State<Arc<AppState>>,
    axum::extract::Extension(_agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
) -> Result<Json<ConflictAnalysis>, (StatusCode, String)> {
    Err((
        StatusCode::NOT_IMPLEMENTED,
        "Conflict analysis endpoint implementation in progress".to_string(),
    ))
}

pub async fn generate_atomic_memories(
    State(_state): State<Arc<AppState>>,
    axum::extract::Extension(_agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    Json(payload): Json<GenerateAtomicMemoriesRequest>,
) -> Result<Json<GenerateAtomicMemoriesResponse>, (StatusCode, String)> {
    let atoms = crate::services::AtomicMemoryGenerator::generate_from_chunk(&payload.chunk, payload.context.as_deref())
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Extraction failed: {}", e)))?;

    let atomic_items = atoms.into_iter().map(|a| AtomicMemoryItem {
        fact: a.fact,
        confidence: a.confidence,
        entities: a.entities,
        relations: a.relations,
    }).collect();

    Ok(Json(GenerateAtomicMemoriesResponse {
        atomic_memories: atomic_items,
        chunk_preview: if payload.chunk.len() > 100 { format!("{}...", &payload.chunk[..100]) } else { payload.chunk.clone() },
    }))
}

#[derive(Debug, Serialize)]
pub struct TemporalStats {
    pub total_memories: i64,
    pub memories_today: i64,
    pub memories_this_week: i64,
    pub memories_this_month: i64,
    pub average_days_between_memories: f64,
    pub memory_types_distribution: std::collections::HashMap<String, i64>,
}

#[derive(Debug, Serialize)]
pub struct ConflictAnalysis {
    pub total_conflicts: i64,
    pub unresolved_conflicts: i64,
    pub avg_resolution_time_sec: f64,
}

#[derive(Debug, Deserialize)]
pub struct GenerateAtomicMemoriesRequest {
    pub chunk: String,
    pub context: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct AtomicMemoryItem {
    pub fact: String,
    pub confidence: f32,
    pub entities: Vec<String>,
    pub relations: Vec<String>,
}

#[derive(Debug, Serialize)]
pub struct GenerateAtomicMemoriesResponse {
    pub atomic_memories: Vec<AtomicMemoryItem>,
    pub chunk_preview: String,
}
