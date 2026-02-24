use ax_memro::adapters::api::{routes, AppState};
use ax_memro::adapters::postgres::PostgresStore;
use ax_memro::adapters::crypto::Ed25519CryptoService;
use ax_memro::adapters::qdrant::QdrantStore;
use ax_memro::adapters::embeddings::MockEmbeddingService;
use ax_memro::adapters::stub_vector::StubVectorStore;

use sqlx::postgres::PgPoolOptions;
use std::net::SocketAddr;
use std::sync::Arc;

use tower_http::cors::{AllowOrigin, CorsLayer};
use http::HeaderValue;

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

    let max_connections: u32 = std::env::var("DB_MAX_CONNECTIONS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(20);

    let pool = PgPoolOptions::new()
        .max_connections(max_connections)
        .connect(&database_url)
        .await?;

    tracing::info!("Database pool initialized (max_connections={})", max_connections);

    // Run migrations
    sqlx::migrate!("./migrations").run(&pool).await?;
    tracing::info!("Database migrations applied");

    let store = Arc::new(PostgresStore::new(pool.clone()));
    let crypto_service = Arc::new(Ed25519CryptoService);

    let qdrant_url = std::env::var("QDRANT_URL")
        .unwrap_or_else(|_| "http://qdrant:6333".to_string());

    // Try to initialize Qdrant, but don't fail if it's unavailable
    let vector_store = match QdrantStore::new(&qdrant_url).await {
        Ok(store) => {
            tracing::info!("Qdrant vector store initialized successfully");
            Arc::new(store) as Arc<dyn ax_memro::ports::VectorStore>
        },
        Err(e) => {
            tracing::warn!("Qdrant unavailable, using stub (semantic search disabled): {}", e);
            Arc::new(StubVectorStore) as Arc<dyn ax_memro::ports::VectorStore>
        }
    };

    let embedding_service = Arc::new(MockEmbeddingService);

    // Validate OpenAI API key
    let openai_key = std::env::var("OPENAI_API_KEY").unwrap_or_default();
    if openai_key.is_empty() {
        tracing::warn!("OPENAI_API_KEY not set — semantic search and audio transcription will not work");
    } else {
        tracing::info!("OpenAI API key configured");
    }

    let openai_service = Arc::new(ax_memro::services::OpenAIService::new(openai_key));

    // Create embedding pipeline
    let (embedding_tx, embedding_rx) = tokio::sync::mpsc::channel(1000);
    let embedding_queue = ax_memro::services::EmbeddingQueue::new(embedding_tx);

    // Start background embedding worker
    let pipeline = ax_memro::services::EmbeddingPipeline::new(
        openai_service.clone(),
        vector_store.clone(),
        embedding_rx,
    );

    tokio::spawn(async move {
        pipeline.run().await;
    });

    let state = Arc::new(AppState {
        identity_store: store.clone(),
        memory_store: store,
        vector_store,
        embedding_service,
        crypto_service,
        db_pool: pool.clone(),
        embedding_queue,
    });

    // CORS: restrict to configured origins, fall back to dev defaults
    let allowed_origins_str = std::env::var("ALLOWED_ORIGINS")
        .unwrap_or_else(|_| "http://localhost:5174,http://localhost:3000,http://localhost:5173".to_string());

    let allowed_origins: Vec<HeaderValue> = allowed_origins_str
        .split(',')
        .filter_map(|s| s.trim().parse().ok())
        .collect();

    let cors = CorsLayer::new()
        .allow_origin(AllowOrigin::list(allowed_origins))
        .allow_methods([
            http::Method::GET,
            http::Method::POST,
            http::Method::DELETE,
            http::Method::OPTIONS,
        ])
        .allow_headers([
            http::header::CONTENT_TYPE,
            http::header::AUTHORIZATION,
            "x-agent-id".parse().unwrap(),
            "x-signature".parse().unwrap(),
            "x-timestamp".parse().unwrap(),
        ]);

    let app = routes()
        .layer(cors)
        .with_state(state);

    let port: u16 = std::env::var("PORT")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(8080);

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    tracing::info!("Listening on {}", addr);
    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, app).await?;

    Ok(())
}
