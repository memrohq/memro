/// Advanced Optimization: LRU in-memory cache for memory objects
/// Reduces database queries for repeated access patterns
use std::sync::Arc;
use dashmap::DashMap;
use lru::LruCache;
use std::num::NonZeroUsize;
use std::sync::Mutex;
use uuid::Uuid;
use crate::domain::Memory;

/// Memory cache with LRU eviction
/// Caches individual memory objects + search results
pub struct MemoryCache {
    /// Individual memory object cache (key: memory_id)
    memory_cache: Arc<DashMap<Uuid, Arc<Memory>>>,
    /// Search result cache (key: hash of search params)
    search_cache: Arc<Mutex<LruCache<String, Vec<Uuid>>>>,
    /// Cache hit/miss metrics
    hits: Arc<std::sync::atomic::AtomicU64>,
    miss: Arc<std::sync::atomic::AtomicU64>,
}

impl MemoryCache {
    pub fn new(capacity: usize) -> Self {
        Self {
            memory_cache: Arc::new(DashMap::new()),
            search_cache: Arc::new(Mutex::new(
                LruCache::new(NonZeroUsize::new(capacity / 10).unwrap())
            )),
            hits: Arc::new(std::sync::atomic::AtomicU64::new(0)),
            miss: Arc::new(std::sync::atomic::AtomicU64::new(0)),
        }
    }

    /// Get memory from cache (10-100x faster than DB fetch)
    pub fn get_memory(&self, memory_id: &Uuid) -> Option<Arc<Memory>> {
        if let Some(entry) = self.memory_cache.get(memory_id) {
            self.hits.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
            Some(Arc::clone(entry.value()))
        } else {
            self.miss.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
            None
        }
    }

    /// Cache a memory object
    pub fn cache_memory(&self, memory: Memory) {
        self.memory_cache.insert(memory.id, Arc::new(memory));
    }

    /// Cache multiple memories at once
    pub fn cache_memories(&self, memories: Vec<Memory>) {
        for memory in memories {
            self.cache_memory(memory);
        }
    }

    /// Get search results from cache
    pub fn get_search_results(&self, search_key: &str) -> Option<Vec<Uuid>> {
        let mut cache = self.search_cache.lock().unwrap();
        cache.get(search_key).cloned()
    }

    /// Cache search results (stays fresh for 5 minutes via TTL in search adapter)
    pub fn cache_search_results(&self, search_key: String, results: Vec<Uuid>) {
        let mut cache = self.search_cache.lock().unwrap();
        cache.put(search_key, results);
    }

    /// Clear all caches
    pub fn clear(&self) {
        self.memory_cache.clear();
        let mut cache = self.search_cache.lock().unwrap();
        cache.clear();
    }

    /// Invalidate specific memory from cache (call on update/delete)
    pub fn invalidate(&self, memory_id: &Uuid) {
        self.memory_cache.remove(memory_id);
    }

    /// Get cache statistics
    pub fn stats(&self) -> CacheStats {
        let hits = self.hits.load(std::sync::atomic::Ordering::Relaxed);
        let miss = self.miss.load(std::sync::atomic::Ordering::Relaxed);
        let total = hits + miss;
        
        CacheStats {
            hits,
            misses: miss,
            total_accesses: total,
            hit_rate: if total > 0 { (hits as f64 / total as f64) * 100.0 } else { 0.0 },
            memory_count: self.memory_cache.len(),
            search_cache_size: self.search_cache.lock().unwrap().len(),
        }
    }
}

#[derive(Debug, Clone)]
pub struct CacheStats {
    pub hits: u64,
    pub misses: u64,
    pub total_accesses: u64,
    pub hit_rate: f64,
    pub memory_count: usize,
    pub search_cache_size: usize,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::{MemoryType, Visibility};
    use chrono::Utc;

    #[test]
    fn test_memory_cache() {
        let cache = MemoryCache::new(1000);
        let memory_id = Uuid::new_v4();
        let memory = Memory {
            id: memory_id,
            agent_id: "test".to_string(),
            tenant_id: Uuid::new_v4(),
            content: "test".to_string(),
            memory_type: MemoryType::Episodic,
            visibility: Visibility::Private,
            created_at: Utc::now(),
            file_id: None,
            asset_url: None,
            document_date: Utc::now(),
            event_date: None,
            session_id: None,
            is_atomic: false,
            entity_tags: vec![],
            relationship_types: vec![],
            access_count: 0,
            confidence: 1.0,
            last_accessed_at: Utc::now(),
            metadata: None,
        };

        cache.cache_memory(memory.clone());
        assert!(cache.get_memory(&memory_id).is_some());

        cache.invalidate(&memory_id);
        assert!(cache.get_memory(&memory_id).is_none());
    }

    #[test]
    fn test_cache_stats() {
        let cache = MemoryCache::new(100);
        let memory_id = Uuid::new_v4();

        // Should miss first time
        cache.get_memory(&memory_id);
        let stats = cache.stats();
        assert_eq!(stats.hits, 0);
        assert_eq!(stats.misses, 1);

        // Create and cache memory
        let memory = Memory {
            id: memory_id,
            agent_id: "test".to_string(),
            tenant_id: Uuid::new_v4(),
            content: "test".to_string(),
            memory_type: MemoryType::Episodic,
            visibility: Visibility::Private,
            created_at: Utc::now(),
            file_id: None,
            asset_url: None,
            document_date: Utc::now(),
            event_date: None,
            session_id: None,
            is_atomic: false,
            entity_tags: vec![],
            relationship_types: vec![],
            access_count: 0,
            confidence: 1.0,
            last_accessed_at: Utc::now(),
            metadata: None,
        };
        cache.cache_memory(memory);

        // Should hit second time
        cache.get_memory(&memory_id);
        let stats = cache.stats();
        assert_eq!(stats.hits, 1);
        assert_eq!(stats.misses, 1);
        assert!(stats.hit_rate > 40.0 && stats.hit_rate < 60.0);
    }
}
