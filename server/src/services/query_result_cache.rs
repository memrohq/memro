// P6 Optimization: Full query result caching layer
// caches the final SearchResponse to avoid reconstructive costs for repeated queries

use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use std::time::{Instant, Duration};
use memro_spec::SearchResponse;

const CACHE_TTL: Duration = Duration::from_secs(300); // 5 minutes TTL
const MAX_CACHE_SIZE: usize = 1000; // Max 1000 results

#[derive(Clone, Debug)]
struct CachedResult {
    response: SearchResponse,
    created_at: Instant,
}

pub struct QueryResultCache {
    cache: Arc<RwLock<HashMap<String, CachedResult>>>,
}

impl QueryResultCache {
    pub fn new() -> Self {
        Self {
            cache: Arc::new(RwLock::new(HashMap::with_capacity(100))),
        }
    }

    pub async fn get(&self, key: &str) -> Option<SearchResponse> {
        let cache = self.cache.read().await;
        if let Some(cached) = cache.get(key) {
            if cached.created_at.elapsed() < CACHE_TTL {
                return Some(cached.response.clone());
            }
        }
        None
    }

    pub async fn set(&self, key: String, response: SearchResponse) {
        let mut cache = self.cache.write().await;
        if cache.len() >= MAX_CACHE_SIZE {
            if let Some((oldest_key, _)) = cache
                .iter()
                .min_by_key(|(_, v)| v.created_at)
                .map(|(k, v)| (k.clone(), v.created_at))
            {
                cache.remove(&oldest_key);
            }
        }
        cache.insert(key, CachedResult {
            response,
            created_at: Instant::now(),
        });
    }

    pub async fn clear(&self) {
        self.cache.write().await.clear();
    }
}

impl Default for QueryResultCache {
    fn default() -> Self {
        Self::new()
    }
}
