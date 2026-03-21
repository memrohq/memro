use axum::{
    extract::{State, Path},
    Json,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use crate::adapters::api::AppState;
use crate::adapters::api::audit;
use crate::adapters::api::middleware::AuthenticatedAgent;
use memro_spec::{ReasoningResult, ResolutionResult, EvolutionResult, MemoryTimeline};
use uuid::Uuid;

#[derive(Deserialize)]
pub struct ReasonRequest {
    pub query: String,
}

#[derive(Deserialize)]
pub struct ResolveConflictRequest {
    pub memory_a_id: Uuid,
    pub memory_b_id: Uuid,
}

pub async fn reason_handler(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<AuthenticatedAgent>,
    Json(payload): Json<ReasonRequest>,
) -> Result<Json<ReasoningResult>, (axum::http::StatusCode, String)> {
    audit(&state.db_pool, &agent.id, &agent.tenant_id, "mee.reason", None, None).await;
    state.memory_engine.reason(&agent.id, &payload.query).await
        .map(Json)
        .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))
}

pub async fn resolve_conflict_handler(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<AuthenticatedAgent>,
    Json(payload): Json<ResolveConflictRequest>,
) -> Result<Json<ResolutionResult>, (axum::http::StatusCode, String)> {
    audit(&state.db_pool, &agent.id, &agent.tenant_id, "mee.resolve", None, None).await;
    state.memory_engine.resolve_conflict(&agent.id, payload.memory_a_id, payload.memory_b_id).await
        .map(Json)
        .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))
}

pub async fn evolve_handler(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<AuthenticatedAgent>,
) -> Result<Json<EvolutionResult>, (axum::http::StatusCode, String)> {
    audit(&state.db_pool, &agent.id, &agent.tenant_id, "mee.evolve", None, None).await;
    state.memory_engine.evolve(&agent.id).await
        .map(Json)
        .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))
}

pub async fn get_timeline_handler(
    State(state): State<Arc<AppState>>,
    _: axum::extract::Extension<AuthenticatedAgent>,
    Path(memory_id): Path<Uuid>,
) -> Result<Json<MemoryTimeline>, (axum::http::StatusCode, String)> {
    state.memory_engine.get_timeline(memory_id).await
        .map(Json)
        .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))
}
#[derive(Serialize)]
pub struct UsageResponse {
    pub reason_calls: i64,
    pub deep_reason_calls: i64,
    pub evolve_calls: i64,
    pub memory_nodes: i64,
    pub graph_edges: i64,
    pub warning: Option<String>,
}

pub async fn get_usage_handler(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<AuthenticatedAgent>,
) -> Result<Json<UsageResponse>, (axum::http::StatusCode, String)> {
    // 1. Query audit logs for call counts (today)
    let counts = sqlx::query(
        r#"
        SELECT action, COUNT(*) as count 
        FROM audit_log 
        WHERE agent_id = $1 
          AND created_at > CURRENT_DATE
        GROUP BY action
        "#
    )
    .bind(&agent.id)
    .fetch_all(&state.db_pool)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    // 2. Query memory counts
    let memory_count: i64 = sqlx::query_scalar(
        "SELECT COUNT(*) FROM memories WHERE agent_id = $1"
    )
    .bind(&agent.id)
    .fetch_one(&state.db_pool)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    // 3. Query edge counts
    let edge_count: i64 = sqlx::query_scalar(
        "SELECT COUNT(*) FROM memory_relations WHERE created_by = $1"
    )
    .bind(&agent.id)
    .fetch_one(&state.db_pool)
    .await
    .map_err(|e| (axum::http::StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    let mut usage = UsageResponse {
        reason_calls: 0,
        deep_reason_calls: 0,
        evolve_calls: 0,
        memory_nodes: memory_count,
        graph_edges: edge_count,
        warning: if memory_count > 800 { Some("Approaching memory soft cap (1000). Consider pruning.".to_string()) } else { None },
    };

    for row in counts {
        use sqlx::Row;
        let action: String = row.try_get("action").unwrap_or_default();
        let count: i64 = row.try_get("count").unwrap_or_default();
        match action.as_str() {
            "mee.reason" => usage.reason_calls = count,
            "mee.evolve" => usage.evolve_calls = count,
            _ => {}
        }
    }

    Ok(Json(usage))
}
