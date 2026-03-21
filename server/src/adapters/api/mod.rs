use axum::{
    extract::{Query, State},
    http::{HeaderMap, StatusCode},
    Json,
    routing::{post, get, delete},
    Router,
};
use serde::{Deserialize, Serialize};
use crate::ports::{IdentityStore, MemoryStore, VectorStore, EmbeddingService, CryptoService, TenantStore, HookStore};
use crate::domain::{AgentIdentity, Memory, MemoryType, Visibility};
use crate::services::EmbeddingProvider;
use std::sync::Arc;
use chrono::Utc;
use uuid::Uuid;


pub mod upload;
pub mod search;
pub mod metrics;
pub mod rate_limit;
pub mod multimodal;
pub mod middleware;
pub mod auth;
pub mod temporal;
pub mod mee;

use rate_limit::RateLimiter;
use crate::services::MemoryCache;
use crate::services::QueryEmbeddingCache;

pub struct AppState {
    pub identity_store: Arc<dyn IdentityStore>,
    pub memory_store: Arc<dyn MemoryStore>,
    pub tenant_store: Arc<dyn TenantStore>,
    pub hook_store: Arc<dyn HookStore>,
    pub vector_store: Arc<dyn VectorStore>,
    pub embedding_service: Arc<dyn EmbeddingService>,
    pub crypto_service: Arc<dyn CryptoService>,
    pub email_service: Arc<dyn crate::ports::email::EmailService>,
    pub db_pool: sqlx::PgPool,
    pub embedding_queue: crate::services::EmbeddingQueue,
    pub embedding_provider: Arc<EmbeddingProvider>,
    pub rate_limiter: Arc<RateLimiter>,
    pub storage_path: String,
    pub base_url: String,
    pub memory_cache: Arc<MemoryCache>,  // P1: In-memory LRU cache for memories
    pub query_embedding_cache: Arc<QueryEmbeddingCache>,  // P4: Query embedding cache
    pub query_result_cache: Arc<crate::services::QueryResultCache>, // P6: Full query result cache
    pub memory_engine: Arc<dyn memro_spec::MemoryEngine>, // Memory Engine abstraction
}


// ---------------------------------------------------------------------------
// Audit logging helper
// ---------------------------------------------------------------------------

/// Fire-and-forget audit log write. Errors are logged but never propagated.
pub async fn audit(
    pool: &sqlx::PgPool,
    agent_id: &str,
    tenant_id: &Uuid,
    action: &str,
    resource_id: Option<&str>,
    ip: Option<&str>,
) {
    if let Err(e) = sqlx::query(
        "INSERT INTO audit_log (agent_id, tenant_id, action, resource_id, ip_addr) VALUES ($1, $2, $3, $4, $5)"
    )
    .bind(agent_id)
    .bind(tenant_id)
    .bind(action)
    .bind(resource_id)
    .bind(ip)
    .execute(pool)
    .await
    {
        tracing::error!("Audit log write failed: {}", e);
    }
}

// ---------------------------------------------------------------------------
// Signature verification helper
// ---------------------------------------------------------------------------

// verify_request removed: Now handled globally by middleware::AuthMiddlewareLayer

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
pub struct RegisterIdentityRequest {
    pub public_key: String,
}

#[derive(Serialize)]
pub struct RegisterIdentityResponse {
    pub agent_id: String,
}

pub fn routes(state: Arc<AppState>) -> Router {
    let protected: Router<Arc<AppState>> = Router::new()
        .route("/identity/:id", delete(delete_identity))
        .route("/memory", post(create_memory))
        .route("/memory/agent/:agent_id", get(get_memories))
        .route("/memory/:id", delete(delete_memory_handler))
        .route("/export/:agent_id", get(export_agent_data))
        .route("/upload", post(upload::upload_file))
        .route("/memory/from-file", post(upload::create_memory_from_file))
        .route("/search", get(search::search_memories))
        .route("/upload/image", post(multimodal::upload_image))
        .route("/upload/document", post(multimodal::upload_document))
        .route("/tenant", post(create_tenant))
        .route("/tenant/:id", get(get_tenant))
        .route("/tenant/:id/usage", get(get_tenant_usage))
        // Temporal/Supermemory endpoints
        .route("/search/temporal", get(temporal::search_temporal))
        .route("/memory/chain/:id", get(temporal::get_memory_chain))
        .route("/stats/temporal", get(temporal::get_temporal_stats))
        .route("/analyze/conflicts", get(temporal::analyze_conflicts))
        .route("/memory/generate-atoms", post(temporal::generate_atomic_memories))
        // MEE (Cognitive Engine) endpoints
        .route("/mee/reason", post(mee::reason_handler))
        .route("/mee/resolve", post(mee::resolve_conflict_handler))
        .route("/mee/evolve", post(mee::evolve_handler))
        .route("/mee/timeline/:id", get(mee::get_timeline_handler))
        .route("/usage", get(mee::get_usage_handler))
        .layer(axum::middleware::from_fn_with_state(state.clone(), middleware::rate_limit_middleware))
        // Hook Management
        .route("/hooks", post(register_hook))
        .route("/hooks/:event_type", get(get_hooks_handler))

        .route_layer(axum::middleware::from_fn_with_state(state.clone(), middleware::auth_middleware));

    Router::new()
        .route("/health", get(health_check))
        .route("/metrics", get(metrics::metrics_handler))
        .route("/stats/cache", get(get_cache_stats))  // P4: Expose cache statistics
        .route("/identity", post(create_identity))
        .route("/identity/:id", get(get_identity)) // Note: fetching public identity info can be public
        .route("/assets/:id", get(multimodal::get_asset)) // typically assets are public or signed URL
        .route("/auth/login", post(auth::login_handler))
        .route("/auth/verify", post(auth::verify_handler))
        .route("/auth/logout", delete(auth::logout_handler))
        .merge(protected)
        .with_state(state)
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
    // Using default tenant for health check
    let health_tenant = uuid::Uuid::parse_str("00000000-0000-0000-0000-000000000000").unwrap();
    let qdrant_status = match state.vector_store
        .search_similar("__health__", &health_tenant, vec![0.0_f32; 1536], 1)
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

// P4: Cache statistics endpoint
async fn get_cache_stats(
    State(state): State<Arc<AppState>>,
) -> Json<serde_json::Value> {
    let stats = state.query_embedding_cache.stats().await;
    
    Json(serde_json::json!({
        "query_embedding_cache": {
            "size": stats.size,
            "max_size": stats.max_size,
            "hits": stats.hits,
            "misses": stats.misses,
            "hit_rate_percent": format!("{:.2}%", stats.hit_rate),
            "ttl_seconds": stats.ttl_seconds,
        }
    }))
}

// ---------------------------------------------------------------------------
// Identity endpoints
// ---------------------------------------------------------------------------

async fn create_identity(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    Json(payload): Json<RegisterIdentityRequest>,
) -> Result<Json<RegisterIdentityResponse>, (StatusCode, String)> {
    let ip = rate_limit::client_ip(&headers);
    if !state.rate_limiter.check_and_increment(&ip) {
        metrics::RATE_LIMIT_HITS.inc();
        return Err((StatusCode::TOO_MANY_REQUESTS, "Rate limit exceeded".to_string()));
    }

    // Validate the public key is a valid 32-byte hex-encoded ed25519 key
    let pub_bytes = hex::decode(&payload.public_key)
        .map_err(|_| (StatusCode::BAD_REQUEST, "public_key must be hex-encoded".to_string()))?;
    if pub_bytes.len() != 32 {
        return Err((StatusCode::BAD_REQUEST, "public_key must be 32 bytes (64 hex chars)".to_string()));
    }

    // Verify it's actually a valid ed25519 public key
    if !state.crypto_service.validate_public_key(&payload.public_key) {
        return Err((StatusCode::BAD_REQUEST, "Invalid ed25519 public key".to_string()));
    }

    let identity = AgentIdentity {
        id: payload.public_key.clone(),
        user_id: None,
        tenant_id: uuid::Uuid::parse_str("00000000-0000-0000-0000-000000000000").unwrap(), // Default tenant for now
        created_at: Utc::now(),
    };

    state.identity_store.store_identity(&identity).await
        .map_err(|e| {
            tracing::error!("Failed to store identity: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Failed to create identity".to_string())
        })?;

    metrics::IDENTITIES_CREATED.inc();
    audit(&state.db_pool, &payload.public_key, &identity.tenant_id, "identity.create", None, Some(&ip)).await;

    Ok(Json(RegisterIdentityResponse {
        agent_id: payload.public_key,
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
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    axum::extract::Path(id): axum::extract::Path<String>,
    headers: HeaderMap,
) -> Result<StatusCode, (StatusCode, String)> {
    let ip = rate_limit::client_ip(&headers);
    if agent.id != id {
        return Err((StatusCode::FORBIDDEN, "Agent ID mismatch".to_string()));
    }

    match state.identity_store.delete_identity(&id).await {
        Ok(_) => {
            audit(&state.db_pool, &id, &agent.tenant_id, "identity.delete", Some(&id), Some(&ip)).await;
            Ok(StatusCode::NO_CONTENT)
        },
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
    pub memory_type: Option<String>,
    pub visibility: String,
    pub event_date: Option<String>, // ISO 8601 date or natural language, will be extracted if not provided
    pub session_id: Option<String>,
    pub is_atomic: Option<bool>, // Whether this is an atomic fact (true) or raw chunk (false)
    pub metadata: Option<serde_json::Value>,
}

async fn create_memory(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    headers: HeaderMap,
    body: axum::body::Bytes,
) -> Result<Json<Memory>, (StatusCode, String)> {
    let ip = rate_limit::client_ip(&headers);
    if !state.rate_limiter.check_and_increment(&ip) {
        metrics::RATE_LIMIT_HITS.inc();
        return Err((StatusCode::TOO_MANY_REQUESTS, "Rate limit exceeded".to_string()));
    }

    let payload: CreateMemoryRequest = serde_json::from_slice(&body)
        .map_err(|e| (StatusCode::BAD_REQUEST, format!("Invalid JSON: {}", e)))?;

    if payload.content.is_empty() {
        return Err((StatusCode::BAD_REQUEST, "content cannot be empty".to_string()));
    }
    if payload.content.chars().count() > 10_000 {
        return Err((StatusCode::BAD_REQUEST, "content exceeds 10,000 character limit".to_string()));
    }
    if payload.agent_id.is_empty() {
        return Err((StatusCode::BAD_REQUEST, "agent_id is required".to_string()));
    }

    let memory_type_str = match payload.memory_type.as_deref() {
        Some("auto") | Some("unknown") | None => {
            let classifications = state.memory_engine.classify(&payload.content).await
                .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;
            // Take the highest confidence classification
            classifications.first().map(|(t, _)| t.clone()).unwrap_or_else(|| "semantic".to_string())
        }
        Some(t) => t.to_string(),
    };

    let memory_type = match memory_type_str.as_str() {
        "episodic" => MemoryType::Episodic,
        "semantic" => MemoryType::Semantic,
        "procedural" => MemoryType::Procedural,
        "profile" => MemoryType::Profile,
        other => return Err((StatusCode::BAD_REQUEST, format!("Invalid memory_type: '{}'", other))),
    };

    // --- Soft Cap Check ---
    let count: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM memories WHERE agent_id = $1")
        .bind(&payload.agent_id)
        .fetch_one(&state.db_pool)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    if count >= 1000 {
        return Err((StatusCode::FORBIDDEN, "Memory soft cap reached (1000/agent). Please prune old memories to continue in Developer Preview.".to_string()));
    }

    let visibility = match payload.visibility.as_str() {
        "private" => Visibility::Private,
        "shared" => Visibility::Shared,
        "public" => Visibility::Public,
        other => return Err((StatusCode::BAD_REQUEST, format!("Invalid visibility: '{}'", other))),
    };

    let now = Utc::now();
    
    // Extract or parse event_date
    let event_date = if let Some(date_str) = &payload.event_date {
        // Try to parse ISO 8601 first
        chrono::DateTime::parse_from_rfc3339(date_str)
            .ok()
            .map(|dt| dt.with_timezone(&Utc))
    } else {
        // Auto-extract from content
        crate::services::TemporalExtractor::extract_event_date(&payload.content)
    };

    let session_id = payload.session_id.as_ref().and_then(|id| Uuid::parse_str(id).ok());
    let _is_atomic = payload.is_atomic.unwrap_or(false);

    let memory = Memory {
        id: Uuid::new_v4(),
        agent_id: payload.agent_id,
        tenant_id: agent.tenant_id,
        content: payload.content,
        memory_type,
        visibility,
        created_at: Utc::now(),
        asset_url: None,
        file_id: None,
        document_date: now,
        event_date,
        session_id,
        is_atomic: false,
        entity_tags: vec![],
        relationship_types: vec![],
        access_count: 0,
        confidence: 1.0,
        last_accessed_at: Utc::now(),
        metadata: payload.metadata,
    };

    state.memory_store.store_memory(&memory, &agent.tenant_id).await
        .map_err(|e| {
            tracing::error!("Failed to store memory: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Failed to store memory".to_string())
        })?;

    if let Err(e) = state.embedding_queue.queue_memory(
        memory.id,
        memory.agent_id.clone(),
        agent.tenant_id,
        memory.content.clone(),
    ).await {
        tracing::error!("Failed to queue memory for embedding: {}", e);
    }

    metrics::MEMORIES_CREATED.inc();
    audit(&state.db_pool, &memory.agent_id, &agent.tenant_id, "memory.create", Some(&memory.id.to_string()), Some(&ip)).await;

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
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    axum::extract::Path(agent_id): axum::extract::Path<String>,
    Query(params): Query<GetMemoriesQuery>,
    _headers: HeaderMap,
) -> Result<Json<Vec<Memory>>, (StatusCode, String)> {
    let limit = params.limit.map(|l| l.min(100));

    // Protected route: agent must match the requested subset
    if agent.id != agent_id {
        return Err((StatusCode::FORBIDDEN, "Agent ID mismatch".to_string()));
    }
    let effective_visibility = params.visibility.as_deref();

    match state.memory_store.get_memories_filtered(
        &agent_id,
        &agent.tenant_id,
        params.memory_type.as_deref(),
        effective_visibility,
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
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    axum::extract::Path(id): axum::extract::Path<String>,
    headers: HeaderMap,
) -> Result<StatusCode, (StatusCode, String)> {
    let ip = rate_limit::client_ip(&headers);
    let agent_id = &agent.id;

    let memory_uuid = Uuid::parse_str(&id)
        .map_err(|_| (StatusCode::BAD_REQUEST, "Invalid memory ID format".to_string()))?;

    match state.memory_store.delete_memory(&memory_uuid, agent_id, &agent.tenant_id).await {
        Ok(true) => {
            metrics::MEMORIES_DELETED.inc();
            audit(&state.db_pool, agent_id, &agent.tenant_id, "memory.delete", Some(&id), Some(&ip)).await;
            Ok(StatusCode::NO_CONTENT)
        },
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

// ---------------------------------------------------------------------------
// Tenant Management Endpoints
// ---------------------------------------------------------------------------

#[derive(Serialize, Deserialize)]
pub struct CreateTenantRequest {
    pub name: String,
    pub domain: Option<String>,
}

#[derive(Serialize)]
pub struct TenantResponse {
    pub id: String,
    pub name: String,
    pub domain: Option<String>,
    pub created_at: String,
}

#[derive(Serialize)]
pub struct TenantUsageResponse {
    pub tenant_id: String,
    pub metric: String,
    pub value: i64,
    pub period_hours: i64,
}

async fn create_tenant(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    Json(req): Json<CreateTenantRequest>,
) -> Result<Json<TenantResponse>, (StatusCode, String)> {
    // Only allow creating tenants from default tenant
    if agent.tenant_id != Uuid::parse_str("00000000-0000-0000-0000-000000000000").unwrap() {
        return Err((StatusCode::FORBIDDEN, "Only default tenant can create new tenants".to_string()));
    }

    let tenant = crate::domain::Tenant {
        id: Uuid::new_v4(),
        name: req.name,
        domain: req.domain,
        created_at: Utc::now(),
        updated_at: Utc::now(),
    };

    state.tenant_store.create_tenant(&tenant).await
        .map_err(|e| {
            tracing::error!("Failed to create tenant: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Failed to create tenant".to_string())
        })?;

    metrics::TENANTS_CREATED.inc();
    Ok(Json(TenantResponse {
        id: tenant.id.to_string(),
        name: tenant.name,
        domain: tenant.domain,
        created_at: tenant.created_at.to_rfc3339(),
    }))
}

async fn get_tenant(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    axum::extract::Path(tenant_id): axum::extract::Path<String>,
) -> Result<Json<TenantResponse>, (StatusCode, String)> {
    let tid = Uuid::parse_str(&tenant_id)
        .map_err(|_| (StatusCode::BAD_REQUEST, "Invalid tenant ID format".to_string()))?;

    // Can only fetch own tenant or default
    if agent.tenant_id != tid && agent.tenant_id != Uuid::parse_str("00000000-0000-0000-0000-000000000000").unwrap() {
        return Err((StatusCode::FORBIDDEN, "Cannot access other tenant".to_string()));
    }

    match state.tenant_store.get_tenant(&tid).await {
        Ok(Some(tenant)) => Ok(Json(TenantResponse {
            id: tenant.id.to_string(),
            name: tenant.name,
            domain: tenant.domain,
            created_at: tenant.created_at.to_rfc3339(),
        })),
        Ok(None) => Err((StatusCode::NOT_FOUND, "Tenant not found".to_string())),
        Err(e) => {
            tracing::error!("Failed to get tenant: {}", e);
            Err((StatusCode::INTERNAL_SERVER_ERROR, "Failed to get tenant".to_string()))
        }
    }
}

async fn get_tenant_usage(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    axum::extract::Path(tenant_id): axum::extract::Path<String>,
    Query(params): Query<std::collections::HashMap<String, String>>,
) -> Result<Json<Vec<TenantUsageResponse>>, (StatusCode, String)> {
    let tid = Uuid::parse_str(&tenant_id)
        .map_err(|_| (StatusCode::BAD_REQUEST, "Invalid tenant ID format".to_string()))?;

    // Can only fetch own tenant usage
    if agent.tenant_id != tid {
        return Err((StatusCode::FORBIDDEN, "Cannot access other tenant usage".to_string()));
    }

    let hours = params.get("hours")
        .and_then(|h| h.parse::<i64>().ok())
        .unwrap_or(24);

    let since = Utc::now() - chrono::Duration::hours(hours);

    // Get common metrics
    let metrics_to_check = vec!["memories_stored", "memories_searched", "api_calls"];
    let mut results = Vec::new();

    for metric in metrics_to_check {
        match state.tenant_store.get_tenant_usage(&tid, metric, since).await {
            Ok(value) => {
                results.push(TenantUsageResponse {
                    tenant_id: tid.to_string(),
                    metric: metric.to_string(),
                    value,
                    period_hours: hours,
                });
            }
            Err(_) => {
                // Metric not found, add with 0
                results.push(TenantUsageResponse {
                    tenant_id: tid.to_string(),
                    metric: metric.to_string(),
                    value: 0,
                    period_hours: hours,
                });
            }
        }
    }

    Ok(Json(results))
}

async fn export_agent_data(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    axum::extract::Path(agent_id): axum::extract::Path<String>,
    headers: HeaderMap,
) -> Result<Json<ExportResponse>, (StatusCode, String)> {
    let ip = rate_limit::client_ip(&headers);
    if agent.id != agent_id {
        return Err((StatusCode::FORBIDDEN, "Agent ID mismatch".to_string()));
    }

    let memories = state.memory_store.get_memories_by_agent(&agent_id, &agent.tenant_id).await
        .map_err(|e| {
            tracing::error!("Failed to export agent data: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Failed to export data".to_string())
        })?;

    let total = memories.len();
    audit(&state.db_pool, &agent_id, &agent.tenant_id, "data.export", None, Some(&ip)).await;
    Ok(Json(ExportResponse {
        agent_id,
        exported_at: Utc::now().to_rfc3339(),
        total,
        memories,
    }))
}

// ---------------------------------------------------------------------------
// Hook endpoints
// ---------------------------------------------------------------------------

#[derive(Deserialize)]
pub struct RegisterHookRequest {
    pub event_type: String,
    pub url: String,
}

async fn register_hook(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    Json(req): Json<RegisterHookRequest>,
) -> Result<StatusCode, (StatusCode, String)> {
    state.hook_store.register_hook(&agent.tenant_id, &req.event_type, &req.url).await
        .map_err(|e| {
            tracing::error!("Failed to register hook: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Failed to register hook".to_string())
        })?;

    Ok(StatusCode::CREATED)
}

async fn get_hooks_handler(
    State(state): State<Arc<AppState>>,
    axum::extract::Extension(agent): axum::extract::Extension<middleware::AuthenticatedAgent>,
    axum::extract::Path(event_type): axum::extract::Path<String>,
) -> Result<Json<Vec<String>>, (StatusCode, String)> {
    match state.hook_store.get_hooks(&agent.tenant_id, &event_type).await {
        Ok(hooks) => Ok(Json(hooks)),
        Err(e) => {
            tracing::error!("Failed to get hooks: {}", e);
            Err((StatusCode::INTERNAL_SERVER_ERROR, "Failed to retrieve hooks".to_string()))
        }
    }
}
