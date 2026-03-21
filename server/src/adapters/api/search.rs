use axum::{
    extract::{Query, State},
    http::{StatusCode, HeaderMap},
    Json,
    Extension,
};
use std::sync::Arc;
use crate::adapters::api::AppState;
use memro_spec::{SearchQuery, SearchResponse};

/// Hybrid search endpoint (optimized for parallel execution)
/// GET /search?agent_id=xxx&query=xxx&limit=10&mode=hybrid
/// 
/// Optimization: Runs semantic + keyword search concurrently, merges results with deduplication
pub async fn search_memories(
    State(state): State<Arc<AppState>>,
    Extension(agent): Extension<super::middleware::AuthenticatedAgent>,
    headers: HeaderMap,
    Query(mut params): Query<SearchQuery>,
) -> Result<Json<SearchResponse>, (StatusCode, String)> {
    let start = std::time::Instant::now();
    let ip = super::rate_limit::client_ip(&headers);
    if !state.rate_limiter.check_and_increment(&ip) {
        super::metrics::RATE_LIMIT_HITS.inc();
        return Err((StatusCode::TOO_MANY_REQUESTS, "Rate limit exceeded".to_string()));
    }

    // Input validation
    if params.query.is_empty() {
        return Err((StatusCode::BAD_REQUEST, "query cannot be empty".to_string()));
    }
    if params.query.len() > 500 {
        return Err((StatusCode::BAD_REQUEST, "query exceeds 500 character limit".to_string()));
    }
    if params.agent_id.is_empty() {
        return Err((StatusCode::BAD_REQUEST, "agent_id is required".to_string()));
    }
    params.limit = params.limit.min(50);

    // P6 Optimization: Full query result caching layer
    let cache_key = format!("query_res:{}:{}:{}:{:?}:{}", 
        params.agent_id, params.query, params.limit, params.memory_type, params.mode);
    if let Some(cached_response) = state.query_result_cache.get(&cache_key).await {
        super::metrics::SEARCH_CACHE_HITS.inc();
        return Ok(Json(cached_response));
    }
    super::metrics::SEARCH_CACHE_MISSES.inc();

    // Delegate to the Memory Engine (Separation of Core vs Engine)
    let engine_response = state.memory_engine
        .search(&params, &agent.tenant_id)
        .await
        .map_err(|e| {
            tracing::error!("Memory Engine search failed: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Search operation failed".to_string())
        })?;

    let latency = start.elapsed();
    super::metrics::SEARCHES_TOTAL.inc();
    super::audit(&state.db_pool, &agent.id, &agent.tenant_id, "memory.search", None, Some(&ip)).await;

    // Build the final response (preserving existing structure)
    let response = SearchResponse {
        total: engine_response.total,
        results: engine_response.results,
        mode: params.mode,
        latency_ms: latency.as_millis() as u64,
    };

    // P6: Cache the full response
    state.query_result_cache.set(cache_key, response.clone()).await;

    Ok(Json(response))
}
