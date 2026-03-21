use anyhow::Result;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::{mpsc, Semaphore};
use uuid::Uuid;

use crate::adapters::api::metrics::EMBEDDING_QUEUE_DEPTH;
use crate::services::EmbeddingProvider;
use memro_spec::VectorStore;
use std::sync::atomic::{AtomicU64, Ordering};

/// P0 Optimization: Track embedding latency for monitoring
static EMBEDDING_BATCH_TIME_MS: AtomicU64 = AtomicU64::new(0);
static EMBEDDING_BATCH_COUNT: AtomicU64 = AtomicU64::new(0);

/// Background worker that processes embedding generation queue
pub struct EmbeddingPipeline {
    provider: Arc<EmbeddingProvider>,
    vector_store: Arc<dyn VectorStore>,
    receiver: mpsc::Receiver<EmbeddingTask>,
    semaphore: Arc<Semaphore>,
}

#[derive(Debug, Clone)]
pub struct EmbeddingTask {
    pub memory_id: Uuid,
    pub agent_id: String,
    pub tenant_id: Uuid,
    pub content: String,
}

impl EmbeddingPipeline {
    pub fn new(
        provider: Arc<EmbeddingProvider>,
        vector_store: Arc<dyn VectorStore>,
        receiver: mpsc::Receiver<EmbeddingTask>,
    ) -> Self {
        Self {
            provider,
            vector_store,
            receiver,
            semaphore: Arc::new(Semaphore::new(30)), // P0: Increased from 10 to 30 concurrent batches
        }
    }

    /// Start the background worker with concurrent batching
    pub async fn run(self) {
        let provider = self.provider;
        let vector_store = self.vector_store;
        let semaphore = self.semaphore;
        let mut receiver = self.receiver;

        tracing::info!("Embedding pipeline started (provider={})", provider.name());

        let mut batch = Vec::with_capacity(200); // P0: Increased from 100 to 200 for larger batches
        let mut interval = tokio::time::interval(Duration::from_millis(100)); // P0: Reduced from 500ms to 100ms for faster processing

        loop {
            tokio::select! {
                Some(task) = receiver.recv() => {
                    batch.push(task);
                    if batch.len() >= 200 { // P0: Process at 200 items instead of 100
                        let b = std::mem::take(&mut batch);
                        let p = Arc::clone(&provider);
                        let vs = Arc::clone(&vector_store);
                        let sem = Arc::clone(&semaphore);
                        tokio::spawn(async move {
                            if let Err(e) = process_batch_internal(b, p, vs, sem).await {
                                tracing::error!("Failed to process embedding batch: {}", e);
                            }
                        });
                    }
                }
                _ = interval.tick() => {
                    if !batch.is_empty() {
                        let b = std::mem::take(&mut batch);
                        let p = Arc::clone(&provider);
                        let vs = Arc::clone(&vector_store);
                        let sem = Arc::clone(&semaphore);
                        tokio::spawn(async move {
                            if let Err(e) = process_batch_internal(b, p, vs, sem).await {
                                tracing::error!("Failed to process embedding batch: {}", e);
                            }
                        });
                    }
                }
            }
        }
    }
}

/// Internal helper for processing a batch concurrently
async fn process_batch_internal(
    tasks: Vec<EmbeddingTask>,
    provider: Arc<EmbeddingProvider>,
    vector_store: Arc<dyn VectorStore>,
    semaphore: Arc<Semaphore>,
) -> Result<()> {
    if tasks.is_empty() { return Ok(()); }

    // Wait for a slot in the concurrency limit
    let _permit = semaphore.acquire().await?;

    let batch_start = std::time::Instant::now();
    let batch_size = tasks.len();
    
    tracing::info!("Processing embedding batch of {} tasks", batch_size);
    
    let contents: Vec<String> = tasks.iter().map(|t| t.content.clone()).collect();
    let embeddings = provider.embed_batch(contents).await?;

    // P0: Parallelize vector store indexing for better throughput
    let indexing_tasks = tasks.into_iter().zip(embeddings.into_iter())
        .map(|(task, embedding)| {
            let vs = Arc::clone(&vector_store);
            tokio::spawn(async move {
                if let Err(e) = vs.index_memory(&task.memory_id, &task.agent_id, &task.tenant_id, embedding).await {
                    tracing::error!("Failed to index memory {} in vector store: {}", task.memory_id, e);
                }
            })
        })
        .collect::<Vec<_>>();
    
    // Wait for all indexing operations to complete
    for handle in indexing_tasks {
        let _ = handle.await;
    }

    let batch_time_ms = batch_start.elapsed().as_millis() as u64;
    EMBEDDING_BATCH_TIME_MS.store(batch_time_ms, Ordering::Relaxed);
    EMBEDDING_BATCH_COUNT.fetch_add(1, Ordering::Relaxed);
    
    tracing::info!("Batch processed in {}ms (avg {:.2}ms/item)", batch_time_ms, batch_time_ms as f64 / batch_size as f64);
    EMBEDDING_QUEUE_DEPTH.sub(batch_size as i64);

    Ok(())
}

/// Embedding queue for sending tasks to the pipeline
#[derive(Clone)]
pub struct EmbeddingQueue {
    sender: mpsc::Sender<EmbeddingTask>,
}

impl EmbeddingQueue {
    pub fn new(sender: mpsc::Sender<EmbeddingTask>) -> Self {
        Self { sender }
    }

    /// Queue a memory for embedding generation
    pub async fn queue_memory(
        &self,
        memory_id: Uuid,
        agent_id: String,
        tenant_id: Uuid,
        content: String,
    ) -> Result<()> {
        let task = EmbeddingTask {
            memory_id,
            agent_id,
            tenant_id,
            content,
        };

        EMBEDDING_QUEUE_DEPTH.inc();
        self.sender.send(task).await?;
        Ok(())
    }

    /// Queue multiple memories for batch processing
    pub async fn queue_batch(&self, tasks: Vec<EmbeddingTask>) -> Result<()> {
        let len = tasks.len();
        for task in tasks {
            self.sender.send(task).await?;
        }
        EMBEDDING_QUEUE_DEPTH.add(len as i64);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_embedding_queue() {
        let (tx, mut rx) = mpsc::channel(100);
        let queue = EmbeddingQueue::new(tx);

        queue
            .queue_memory(
                Uuid::new_v4(),
                "test_agent".to_string(),
                Uuid::new_v4(),
                "test content".to_string(),
            )
            .await
            .unwrap();

        let task = rx.recv().await.unwrap();
        assert_eq!(task.agent_id, "test_agent");
        assert_eq!(task.content, "test content");
    }
}
