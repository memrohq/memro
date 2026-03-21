use axum::{
    http::{header, StatusCode},
    response::IntoResponse,
};
use once_cell::sync::Lazy;
use prometheus::{register_counter, register_counter_vec, Counter, CounterVec, Encoder, TextEncoder};

// ── Global metrics ─────────────────────────────────────────────────────────

pub static MEMORIES_CREATED: Lazy<Counter> = Lazy::new(|| {
    register_counter!(
        "memro_memories_created_total",
        "Total number of memories created"
    )
    .unwrap()
});

pub static MEMORIES_DELETED: Lazy<Counter> = Lazy::new(|| {
    register_counter!(
        "memro_memories_deleted_total",
        "Total number of memories deleted"
    )
    .unwrap()
});

pub static SEARCHES_TOTAL: Lazy<Counter> = Lazy::new(|| {
    register_counter!(
        "memro_searches_total",
        "Total number of semantic searches performed"
    )
    .unwrap()
});

pub static IDENTITIES_CREATED: Lazy<Counter> = Lazy::new(|| {
    register_counter!(
        "memro_identities_created_total",
        "Total number of agent identities created"
    )
    .unwrap()
});

pub static TENANTS_CREATED: Lazy<Counter> = Lazy::new(|| {
    register_counter!(
        "memro_tenants_created_total",
        "Total number of tenants created"
    )
    .unwrap()
});

pub static HTTP_ERRORS: Lazy<CounterVec> = Lazy::new(|| {
    register_counter_vec!(
        "memro_http_errors_total",
        "Total HTTP errors by status code",
        &["status"]
    )
    .unwrap()
});

pub static SEARCH_CACHE_HITS: Lazy<Counter> = Lazy::new(|| {
    register_counter!(
        "memro_search_cache_hits_total",
        "Total number of search cache hits"
    )
    .unwrap()
});

pub static SEARCH_CACHE_MISSES: Lazy<Counter> = Lazy::new(|| {
    register_counter!(
        "memro_search_cache_misses_total",
        "Total number of search cache misses"
    )
    .unwrap()
});

pub static RATE_LIMIT_HITS: Lazy<Counter> = Lazy::new(|| {
    register_counter!(
        "memro_rate_limit_hits_total",
        "Total number of requests rejected by rate limiter"
    )
    .unwrap()
});

pub static EMBEDDING_QUEUE_DEPTH: Lazy<prometheus::IntGauge> = Lazy::new(|| {
    prometheus::register_int_gauge!(
        "memro_embedding_queue_depth",
        "Current number of memories waiting for embedding"
    )
    .unwrap()
});

/// Force initialise all lazy statics so they appear in metrics even before
/// the first request of each type.
pub fn init_metrics() {
    Lazy::force(&MEMORIES_CREATED);
    Lazy::force(&MEMORIES_DELETED);
    Lazy::force(&SEARCHES_TOTAL);
    Lazy::force(&IDENTITIES_CREATED);
    Lazy::force(&TENANTS_CREATED);
    Lazy::force(&HTTP_ERRORS);
    Lazy::force(&SEARCH_CACHE_HITS);
    Lazy::force(&SEARCH_CACHE_MISSES);
    Lazy::force(&RATE_LIMIT_HITS);
    Lazy::force(&EMBEDDING_QUEUE_DEPTH);
}

// ── Handler ────────────────────────────────────────────────────────────────

/// GET /metrics — Prometheus text format (scraped by Prometheus / Grafana Agent)
pub async fn metrics_handler() -> impl IntoResponse {
    let encoder = TextEncoder::new();
    let metric_families = prometheus::gather();
    let mut buffer = Vec::new();

    if let Err(e) = encoder.encode(&metric_families, &mut buffer) {
        return (
            StatusCode::INTERNAL_SERVER_ERROR,
            [(header::CONTENT_TYPE, "text/plain")],
            format!("metrics encoding error: {e}").into_bytes(),
        );
    }

    (
        StatusCode::OK,
        [(header::CONTENT_TYPE, "text/plain; version=0.0.4; charset=utf-8")],
        buffer,
    )
}
