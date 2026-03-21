use ax_memro::adapters::api::{routes, rate_limit::RateLimiter, metrics::init_metrics, AppState};
use ax_memro::adapters::postgres::PostgresStore;
use ax_memro::adapters::crypto::Ed25519CryptoService;
use ax_memro::adapters::qdrant::QdrantStore;
use uuid::Uuid;
use ax_memro::adapters::stub_vector::StubVectorStore;
use ax_memro::services::EmbeddingProvider;
use std::time::Duration;

use sqlx::postgres::PgPoolOptions;
use std::net::SocketAddr;
use std::sync::Arc;

use tower_http::cors::{AllowOrigin, CorsLayer};

use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};
use dotenvy::dotenv;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    dotenv().ok();

    // Initialize tracing
    tracing_subscriber::registry()
        .with(tracing_subscriber::EnvFilter::try_from_default_env()
            .unwrap_or_else(|_| "ax_memro=debug,tower_http=debug".into()))
        .with(tracing_subscriber::fmt::layer())
        .init();

    let database_url = std::env::var("DATABASE_URL")
        .expect("DATABASE_URL must be set");

    // P1 Optimization: Tuned connection pool for better performance
    let max_connections: u32 = std::env::var("DB_MAX_CONNECTIONS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(100);  // P1: Increased from 20 to 100 for better concurrency
    
    let min_connections: u32 = std::env::var("DB_MIN_CONNECTIONS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(10);  // P1: Added min pool size

    let pool = PgPoolOptions::new()
        .max_connections(max_connections)
        .min_connections(min_connections)  // P1: Pre-allocate min connections
        .acquire_timeout(Duration::from_secs(30))  // P1: Timeout for acquiring connection
        .idle_timeout(Duration::from_secs(600))  // P1: Keep idle connections alive for 10 min
        .max_lifetime(Duration::from_secs(1800))  // P1: Maximum connection lifetime (30 min)
        .connect(&database_url)
        .await?;

    tracing::info!("Database pool initialized (min={}, max={} connections)", min_connections, max_connections);

    // Run migrations (toggle via MIGRATE_ON_STARTUP)
    let migrate_on_startup = std::env::var("MIGRATE_ON_STARTUP").unwrap_or_else(|_| "true".to_string());
    if migrate_on_startup.to_ascii_lowercase() == "true" {
        tracing::debug!("Applying embedded migrations...");
        sqlx::migrate!("./migrations").run(&pool).await?;
        tracing::info!("Database migrations applied");
    } else {
        tracing::info!("Database migrations skipped on startup (MIGRATE_ON_STARTUP={})", migrate_on_startup);
    }

    // Initialise Prometheus metrics counters
    init_metrics();

    let store = Arc::new(PostgresStore::new(pool.clone()));
    let crypto_service = Arc::new(Ed25519CryptoService);

    let rate_limit_max: u64 = std::env::var("RATE_LIMIT_MAX")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(200);

    let rate_limit_window_secs: u64 = std::env::var("RATE_LIMIT_WINDOW_SECONDS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(60);

    // Rate limiter: default 200 requests per minute per IP
    let rate_limiter = Arc::new(RateLimiter::new(rate_limit_max, Duration::from_secs(rate_limit_window_secs)));

    // Initialise embedding provider from environment (OpenAI or Voyage AI / Anthropic)
    let embedding_provider = Arc::new(EmbeddingProvider::from_env());
    let embedding_dim = embedding_provider.dimension();
    tracing::info!(
        "Embedding provider: {} (dimension={})",
        embedding_provider.name(),
        embedding_dim
    );

    let qdrant_url = std::env::var("QDRANT_URL")
        .unwrap_or_else(|_| "http://qdrant:6334".to_string());

    let qdrant_api_key = std::env::var("QDRANT_API_KEY").ok();
    
    // Try to initialise Qdrant with the correct vector dimension for the chosen provider
    let vector_store = match QdrantStore::new(&qdrant_url, qdrant_api_key, embedding_dim).await {
        Ok(store) => {
            tracing::info!("Qdrant vector store initialised successfully");
            Arc::new(store) as Arc<dyn memro_spec::VectorStore>
        },
        Err(e) => {
            tracing::warn!("Qdrant unavailable, using stub (semantic search disabled): {}", e);
            Arc::new(StubVectorStore) as Arc<dyn ax_memro::ports::VectorStore>
        }
    };

    // Use the real embedding provider as the embedding service for the engine
    let embedding_service = embedding_provider.clone();

    // Create embedding pipeline
    let (embedding_tx, embedding_rx) = tokio::sync::mpsc::channel(100_000);
    let embedding_queue = ax_memro::services::EmbeddingQueue::new(embedding_tx);

    // Start background embedding worker
    let pipeline = ax_memro::services::EmbeddingPipeline::new(
        embedding_provider.clone(),
        vector_store.clone(),
        embedding_rx,
    );

    tokio::spawn(async move {
        pipeline.run().await;
    });

    // P3 Optimization: Start relationship indexer background job
    let relationship_indexer = ax_memro::services::RelationshipIndexer::new(pool.clone());
    relationship_indexer.start().await;
    tracing::info!("P3: Relationship indexer background job started");

    let storage_path = std::env::var("FILE_STORAGE_PATH")
        .unwrap_or_else(|_| "./data/files".to_string());
    
    // Create storage directory if it doesn't exist
    if let Err(e) = std::fs::create_dir_all(&storage_path) {
        tracing::error!("Failed to create storage directory {}: {}", storage_path, e);
    }

    let port: u16 = std::env::var("PORT")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(8081);

    let base_url = std::env::var("BASE_URL")
        .unwrap_or_else(|_| format!("http://localhost:{}", port));

    let email_host = std::env::var("EMAIL_HOST").unwrap_or_else(|_| "localhost".to_string()).trim().to_string();
    let email_port = std::env::var("EMAIL_PORT").unwrap_or_else(|_| "587".to_string()).trim().parse().unwrap_or(587);
    let email_user = std::env::var("EMAIL_HOST_USER").unwrap_or_default().trim().to_string();
    let email_pass = std::env::var("EMAIL_HOST_PASSWORD").unwrap_or_default().trim().to_string();

    let email_service = Arc::new(ax_memro::adapters::email::LettreEmailService::new(
        &email_host,
        email_port,
        &email_user,
        &email_pass,
    ));

    // P1 Optimization: Initialize in-memory LRU cache for memories
    let memory_cache_size = std::env::var("MEMORY_CACHE_SIZE")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(10_000);  // P1: Cache up to 10k memories
    let memory_cache = Arc::new(ax_memro::services::MemoryCache::new(memory_cache_size));
    tracing::info!("Memory cache initialized (capacity={})", memory_cache_size);

    // P4 Optimization: Initialize query embedding cache
    let query_embedding_cache = Arc::new(ax_memro::services::QueryEmbeddingCache::new());
    let query_cache_cleanup = query_embedding_cache.clone();
    query_cache_cleanup.start_cleanup_task();
    tracing::info!("P4: Query embedding cache initialized");
    
    // P6 Optimization: Initialize full query result cache
    let query_result_cache = Arc::new(ax_memro::services::QueryResultCache::new());
    tracing::info!("P6: Query result cache initialized");

    // Initialize Memory Engine (Optimized for performance)
    let memory_engine = Arc::new(memro_engine::OptimizedEngine::new(
        pool.clone(),
        vector_store.clone(),
        embedding_service.clone(),
    ));

    // Bootstrap: Ensure all existing memories are indexed in vector store
    let boot_state = pool.clone();
    let boot_queue = embedding_queue.clone();
    tokio::spawn(async move {
        if let Err(e) = bootstrap_reindex(boot_state, boot_queue).await {
            tracing::error!("Bootstrap reindex failed: {}", e);
        }
    });

    tracing::info!("Memory Engine (Optimized) initialized");

    let state = Arc::new(AppState {
        identity_store: store.clone(),
        memory_store: store.clone(),
        tenant_store: store.clone(),
        hook_store: store.clone(),
        vector_store,
        embedding_service,
        crypto_service,
        email_service,
        db_pool: pool.clone(),
        embedding_queue,
        embedding_provider,
        rate_limiter,
        storage_path,
        base_url,
        memory_cache,  // P1: Add cache to state
        query_embedding_cache,  // P4: Add query embedding cache to state
        query_result_cache,  // P6: Add query result cache to state
        memory_engine, // Memory Engine abstraction
    });


    // CORS: allow all origins for deployment flexibility
    let cors = CorsLayer::new()
        .allow_origin(AllowOrigin::any())
        .allow_methods(tower_http::cors::Any)
        .allow_headers(tower_http::cors::Any);

    let app = routes(state)
        .layer(cors);

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    tracing::info!("Listening on {}", addr);
    let listener = tokio::net::TcpListener::bind(addr).await?;

    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await?;

    tracing::info!("Server shut down gracefully");
    Ok(())
}


async fn bootstrap_reindex(pool: sqlx::PgPool, queue: ax_memro::services::EmbeddingQueue) -> Result<(), anyhow::Error> {
    tracing::info!("Starting bootstrap reindex of existing memories...");
    
    // Fetch all memories that might need indexing
    // In a more complex system, we'd check if they're already in Qdrant,
    // but for now we just queue everything (idempotent upsert).
    let rows = sqlx::query(
        "SELECT id, agent_id, tenant_id, content FROM memories"
    )
    .fetch_all(&pool)
    .await?;

    let count = rows.len();
    for row in rows {
        use sqlx::Row;
        let id: Uuid = row.get("id");
        let agent_id: String = row.get("agent_id");
        let tenant_id: Uuid = row.get("tenant_id");
        let content: String = row.get("content");
        
        queue.queue_memory(id, agent_id, tenant_id, content).await?;
    }

    tracing::info!("Bootstrap: Queued {} memories for reindexing", count);
    Ok(())
}

async fn shutdown_signal() {
    let ctrl_c = async {
        tokio::signal::ctrl_c()
            .await
            .expect("Failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .expect("Failed to install SIGTERM handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => tracing::info!("Received Ctrl+C, shutting down"),
        _ = terminate => tracing::info!("Received SIGTERM, shutting down"),
    }
}
