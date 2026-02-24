use axum::{
    extract::{Query, State},
    http::{HeaderMap, StatusCode},
    Json,
    routing::{post, get, delete},
    Router,
};
use serde::{Deserialize, Serialize};
use crate::ports::{IdentityStore, MemoryStore, VectorStore, EmbeddingService, CryptoService};
use crate::domain::{AgentIdentity, Memory, MemoryType, Visibility};
use std::sync::Arc;
use chrono::Utc;
use uuid::Uuid;

pub mod upload;
pub mod search;

pub struct AppState {
    pub identity_store: Arc<dyn IdentityStore>,
    pub memory_store: Arc<dyn MemoryStore>,
    pub vector_store: Arc<dyn VectorStore>,
    pub embedding_service: Arc<dyn EmbeddingService>,
    pub crypto_service: Arc<dyn CryptoService>,
    pub db_pool: sqlx::PgPool,
    pub embedding_queue: crate::services::EmbeddingQueue,
}

// ---------------------------------------------------------------------------
// Signature verification helper
// ---------------------------------------------------------------------------

/// Verify ed25519 signature on a mutating request.
/// Required headers:
///   X-Agent-Id:  hex-encoded public key (= agent_id)
///   X-Signature: hex-encoded ed25519 signature over the raw request body bytes
///   X-Timestamp: Unix timestamp in seconds (rejected if older than ±300s)
fn verify_request(
    state: &AppState,
    headers: &HeaderMap,
    body: &[u8],
) -> Result<String, (StatusCode, String)> {
    let agent_id = headers
        .get("x-agent-id")
        .and_then(|v| v.to_str().ok())
        .ok_or((StatusCode::UNAUTHORIZED, "Missing X-Agent-Id header".to_string()))?;

    let signature = headers
        .get("x-signature")
        .and_then(|v| v.to_str().ok())
        .ok_or((StatusCode::UNAUTHORIZED, "Missing X-Signature header".to_string()))?;

    let timestamp_str = headers
        .get("x-timestamp")
        .and_then(|v| v.to_str().ok())
        .ok_or((StatusCode::UNAUTHORIZED, "Missing X-Timestamp header".to_string()))?;

    // Replay protection: reject requests older than 5 minutes
    let timestamp: i64 = timestamp_str
        .parse()
        .map_err(|_| (StatusCode::UNAUTHORIZED, "Invalid X-Timestamp".to_string()))?;

    let now = Utc::now().timestamp();
    if (now - timestamp).abs() > 300 {
        return Err((StatusCode::UNAUTHORIZED, "Request timestamp expired".to_string()));
    }

    if !state.crypto_service.verify_signature(agent_id, body, signature) {
        return Err((StatusCode::UNAUTHORIZED, "Invalid signature".to_string()));
    }

    Ok(agent_id.to_string())
}

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

#[derive(Serialize)]
pub struct CreateIdentityResponse {
    pub agent_id: String,
    pub private_key: String,
}

pub fn routes() -> Router<Arc<AppState>> {
    Router::new()
        .route("/health", get(health_check))
        .route("/identity", post(create_identity))
        .route("/identity/:id", get(get_identity))
        .route("/identity/:id", delete(delete_identity))
        .route("/memory", post(create_memory))
        .route("/memory/:agent_id", get(get_memories))
        .route("/memory/:id", delete(delete_memory_handler))
        .route("/export/:agent_id", get(export_agent_data))
        .route("/upload", post(upload::upload_file))
        .route("/memory/from-file", post(upload::create_memory_from_file))
        .route("/search", get(search::search_memories))
}

// ---------------------------------------------------------------------------
// Health check — probes DB connectivity
// ---------------------------------------------------------------------------

async fn health_check(
    State(state): State<Arc<AppState>>,
) -> Json<serde_json::Value> {
    let db_status = match sqlx::query("SELECT 1").execute(&state.db_pool).await {
        Ok(_) => "ok",
        Err(e) => {
            tracing::error!("Health check DB probe failed: {}", e);
            "error"
        }
    };

    // StubVectorStore returns Ok([]) so this works for both real Qdrant and stub
    let qdrant_status = match state.vector_store
        .search_similar("__health__", vec![0.0_f32; 1536], 1)
        .await
    {
        Ok(_) => "ok",
        Err(_) => "unavailable",
    };

    let overall = if db_status == "ok" { "ok" } else { "degraded" };

    Json(serde_json::json!({
        "status": overall,
        "db": db_status,
        "vector_store": qdrant_status,
    }))
}

// ---------------------------------------------------------------------------
// Identity endpoints
// ---------------------------------------------------------------------------

async fn create_identity(
    State(state): State<Arc<AppState>>,
) -> Result<Json<CreateIdentityResponse>, (StatusCode, String)> {
    let (public_key, private_key) = state.crypto_service.generate_keypair();

    let identity = AgentIdentity {
        id: public_key.clone(),
        created_at: Utc::now(),
    };

    state.identity_store.store_identity(&identity).await
        .map_err(|e| {
            tracing::error!("Failed to store identity: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Failed to create identity".to_string())
        })?;

    Ok(Json(CreateIdentityResponse {
        agent_id: public_key,
        private_key,
    }))
}

async fn get_identity(
    State(state): State<Arc<AppState>>,
    axum::extract::Path(id): axum::extract::Path<String>,
) -> Result<Json<AgentIdentity>, (StatusCode, String)> {
    match state.identity_store.get_identity(&id).await {
        Ok(Some(identity)) => Ok(Json(identity)),
        Ok(None) => Err((StatusCode::NOT_FOUND, "Identity not found".to_string())),
        Err(e) => {
            tracing::error!("Failed to get identity: {}", e);
            Err((StatusCode::INTERNAL_SERVER_ERROR, "Internal error".to_string()))
        },
    }
}

async fn delete_identity(
    State(state): State<Arc<AppState>>,
    axum::extract::Path(id): axum::extract::Path<String>,
    headers: HeaderMap,
) -> Result<StatusCode, (StatusCode, String)> {
    verify_request(&state, &headers, id.as_bytes())?;

    match state.identity_store.delete_identity(&id).await {
        Ok(_) => Ok(StatusCode::NO_CONTENT),
        Err(e) => {
            tracing::error!("Failed to delete identity: {}", e);
            Err((StatusCode::INTERNAL_SERVER_ERROR, "Failed to delete identity".to_string()))
        },
    }
}

// ---------------------------------------------------------------------------
// Memory endpoints
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
pub struct CreateMemoryRequest {
    pub agent_id: String,
    pub content: String,
    pub memory_type: String,
    pub visibility: String,
}

async fn create_memory(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    body: axum::body::Bytes,
) -> Result<Json<Memory>, (StatusCode, String)> {
    verify_request(&state, &headers, &body)?;

    let payload: CreateMemoryRequest = serde_json::from_slice(&body)
        .map_err(|e| (StatusCode::BAD_REQUEST, format!("Invalid JSON: {}", e)))?;

    if payload.content.is_empty() {
        return Err((StatusCode::BAD_REQUEST, "content cannot be empty".to_string()));
    }
    if payload.content.len() > 10_000 {
        return Err((StatusCode::BAD_REQUEST, "content exceeds 10,000 character limit".to_string()));
    }
    if payload.agent_id.is_empty() {
        return Err((StatusCode::BAD_REQUEST, "agent_id is required".to_string()));
    }

    let memory_type = match payload.memory_type.as_str() {
        "episodic" => MemoryType::Episodic,
        "semantic" => MemoryType::Semantic,
        "procedural" => MemoryType::Procedural,
        "profile" => MemoryType::Profile,
        other => return Err((StatusCode::BAD_REQUEST, format!("Invalid memory_type: '{}'", other))),
    };

    let visibility = match payload.visibility.as_str() {
        "private" => Visibility::Private,
        "shared" => Visibility::Shared,
        "public" => Visibility::Public,
        other => return Err((StatusCode::BAD_REQUEST, format!("Invalid visibility: '{}'", other))),
    };

    let memory = Memory {
        id: Uuid::new_v4(),
        agent_id: payload.agent_id,
        content: payload.content,
        memory_type,
        visibility,
        created_at: Utc::now(),
    };

    state.memory_store.store_memory(&memory).await
        .map_err(|e| {
            tracing::error!("Failed to store memory: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Failed to store memory".to_string())
        })?;

    if let Err(e) = state.embedding_queue.queue_memory(
        memory.id,
        memory.agent_id.clone(),
        memory.content.clone(),
    ).await {
        tracing::error!("Failed to queue memory for embedding: {}", e);
    }

    Ok(Json(memory))
}

#[derive(Deserialize)]
pub struct GetMemoriesQuery {
    pub memory_type: Option<String>,
    pub visibility: Option<String>,
    pub limit: Option<i64>,
}

async fn get_memories(
    State(state): State<Arc<AppState>>,
    axum::extract::Path(agent_id): axum::extract::Path<String>,
    Query(params): Query<GetMemoriesQuery>,
) -> Result<Json<Vec<Memory>>, (StatusCode, String)> {
    let limit = params.limit.map(|l| l.min(100));

    match state.memory_store.get_memories_filtered(
        &agent_id,
        params.memory_type.as_deref(),
        params.visibility.as_deref(),
        limit,
    ).await {
        Ok(memories) => Ok(Json(memories)),
        Err(e) => {
            tracing::error!("Failed to get memories: {}", e);
            Err((StatusCode::INTERNAL_SERVER_ERROR, "Failed to retrieve memories".to_string()))
        }
    }
}

async fn delete_memory_handler(
    State(state): State<Arc<AppState>>,
    axum::extract::Path(id): axum::extract::Path<String>,
    headers: HeaderMap,
) -> Result<StatusCode, (StatusCode, String)> {
    verify_request(&state, &headers, id.as_bytes())?;

    let agent_id = headers
        .get("x-agent-id")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");

    let memory_uuid = Uuid::parse_str(&id)
        .map_err(|_| (StatusCode::BAD_REQUEST, "Invalid memory ID format".to_string()))?;

    match state.memory_store.delete_memory(&memory_uuid, agent_id).await {
        Ok(true) => Ok(StatusCode::NO_CONTENT),
        Ok(false) => Err((StatusCode::NOT_FOUND, "Memory not found".to_string())),
        Err(e) => {
            tracing::error!("Failed to delete memory: {}", e);
            Err((StatusCode::INTERNAL_SERVER_ERROR, "Failed to delete memory".to_string()))
        }
    }
}

// ---------------------------------------------------------------------------
// Export endpoint — GET /export/:agent_id
// ---------------------------------------------------------------------------

#[derive(Serialize)]
struct ExportResponse {
    agent_id: String,
    exported_at: String,
    total: usize,
    memories: Vec<Memory>,
}

async fn export_agent_data(
    State(state): State<Arc<AppState>>,
    axum::extract::Path(agent_id): axum::extract::Path<String>,
) -> Result<Json<ExportResponse>, (StatusCode, String)> {
    let memories = state.memory_store.get_memories_by_agent(&agent_id).await
        .map_err(|e| {
            tracing::error!("Failed to export agent data: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Failed to export data".to_string())
        })?;

    let total = memories.len();
    Ok(Json(ExportResponse {
        agent_id,
        exported_at: Utc::now().to_rfc3339(),
        total,
        memories,
    }))
}
