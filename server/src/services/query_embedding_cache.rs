// P4 Optimization: Query-time vector embedding cache
// Caches embeddings for queries to avoid re-computing embeddings for repeated queries

use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use std::time::{Instant, Duration};

const CACHE_TTL: Duration = Duration::from_secs(3600); // 1 hour TTL
const MAX_CACHE_SIZE: usize = 10_000; // Max 10k queries cached

#[derive(Clone, Debug)]
struct CachedEmbedding {
    vector: Vec<f32>,
    created_at: Instant,
}

/// P4: Query embedding cache for repeated query optimization
/// Caches embeddings for frequently searched queries
pub struct QueryEmbeddingCache {
    cache: Arc<RwLock<HashMap<String, CachedEmbedding>>>,
    hits: Arc<RwLock<u64>>,
    misses: Arc<RwLock<u64>>,
}

impl QueryEmbeddingCache {
    pub fn new() -> Self {
        Self {
            cache: Arc::new(RwLock::new(HashMap::with_capacity(1000))),
            hits: Arc::new(RwLock::new(0)),
            misses: Arc::new(RwLock::new(0)),
        }
    }

    /// Get cached embedding for a query, returns None if not found or expired
    pub async fn get(&self, query: &str) -> Option<Vec<f32>> {
        let cache = self.cache.read().await;
        
        if let Some(cached) = cache.get(query) {
            // Check if not expired
            if cached.created_at.elapsed() < CACHE_TTL {
                *self.hits.write().await += 1;
                return Some(cached.vector.clone());
            }
        }
        
        *self.misses.write().await += 1;
        None
    }

    /// Store embedding in cache
    pub async fn set(&self, query: String, vector: Vec<f32>) {
        let mut cache = self.cache.write().await;
        
        // Evict old entries if cache is full (simple FIFO eviction)
        if cache.len() >= MAX_CACHE_SIZE {
            // Remove oldest entry by finding earliest created_at
            if let Some((oldest_key, _)) = cache
                .iter()
                .min_by_key(|(_, v)| v.created_at)
                .map(|(k, v)| (k.clone(), v.created_at))
            {
                cache.remove(&oldest_key);
            }
        }
        
        cache.insert(query, CachedEmbedding {
            vector,
            created_at: Instant::now(),
        });
    }

    /// Get cache statistics
    pub async fn stats(&self) -> CacheStats {
        let cache = self.cache.read().await;
        let hits = *self.hits.read().await;
        let misses = *self.misses.read().await;
        let total = hits + misses;
        
        CacheStats {
            size: cache.len(),
            max_size: MAX_CACHE_SIZE,
            hits,
            misses,
            hit_rate: if total > 0 { (hits as f32 / total as f32) * 100.0 } else { 0.0 },
            ttl_seconds: CACHE_TTL.as_secs(),
        }
    }

    /// Clear cache (useful for testing)
    pub async fn clear(&self) {
        self.cache.write().await.clear();
        *self.hits.write().await = 0;
        *self.misses.write().await = 0;
    }

    /// Start periodic cleanup task to remove expired entries
    pub fn start_cleanup_task(self: Arc<Self>) {
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(600)); // Every 10 minutes
            loop {
                interval.tick().await;
                let mut cache = self.cache.write().await;
                let before = cache.len();
                cache.retain(|_, v| v.created_at.elapsed() < CACHE_TTL);
                let after = cache.len();
                
                if after < before {
                    tracing::info!("P4: Cleaned up {} expired query embeddings", before - after);
                }
            }
        });
    }
}

impl Default for QueryEmbeddingCache {
    fn default() -> Self {
        Self::new()
    }
}

#[derive(Clone, Debug)]
pub struct CacheStats {
    pub size: usize,
    pub max_size: usize,
    pub hits: u64,
    pub misses: u64,
    pub hit_rate: f32,
    pub ttl_seconds: u64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_cache_get_set() {
        let cache = QueryEmbeddingCache::new();
        let embedding = vec![0.1, 0.2, 0.3];
        
        // Cache miss
        assert!(cache.get("test query").await.is_none());
        
        // Set and get
        cache.set("test query".to_string(), embedding.clone()).await;
        let result = cache.get("test query").await;
        assert_eq!(result, Some(embedding));
    }

    #[tokio::test]
    async fn test_cache_hits_misses() {
        let cache = QueryEmbeddingCache::new();
        let embedding = vec![0.1, 0.2, 0.3];
        
        cache.set("query1".to_string(), embedding.clone()).await;
        
        // Miss
        let _ = cache.get("query2").await;
        // Hit
        let _ = cache.get("query1").await;
        
        let stats = cache.stats().await;
        assert_eq!(stats.hits, 1);
        assert_eq!(stats.misses, 1);
        assert_eq!(stats.hit_rate, 50.0);
    }
}
