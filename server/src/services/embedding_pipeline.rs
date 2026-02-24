use anyhow::Result;
use std::sync::Arc;
use tokio::sync::mpsc;
use uuid::Uuid;

use crate::services::OpenAIService;
use crate::ports::VectorStore;

/// Background worker that processes embedding generation queue
pub struct EmbeddingPipeline {
    openai: Arc<OpenAIService>,
    vector_store: Arc<dyn VectorStore>,
    receiver: mpsc::Receiver<EmbeddingTask>,
}

#[derive(Debug, Clone)]
pub struct EmbeddingTask {
    pub memory_id: Uuid,
    pub agent_id: String,
    pub content: String,
}

impl EmbeddingPipeline {
    pub fn new(
        openai: Arc<OpenAIService>,
        vector_store: Arc<dyn VectorStore>,
        receiver: mpsc::Receiver<EmbeddingTask>,
    ) -> Self {
        Self {
            openai,
            vector_store,
            receiver,
        }
    }

    /// Start the background worker
    pub async fn run(mut self) {
        tracing::info!("Embedding pipeline started");

        while let Some(task) = self.receiver.recv().await {
            if let Err(e) = self.process_task(task).await {
                tracing::error!("Failed to process embedding task: {}", e);
            }
        }

        tracing::warn!("Embedding pipeline stopped");
    }

    /// Process a single embedding task
    async fn process_task(&self, task: EmbeddingTask) -> Result<()> {
        tracing::debug!(
            "Generating embedding for memory {} (agent: {})",
            task.memory_id,
            task.agent_id
        );

        // Generate embedding using OpenAI
        let embedding = self.openai.embed_text(&task.content).await?;

        // Store in vector database
        self.vector_store
            .index_memory(&task.memory_id, &task.agent_id, embedding)
            .await?;

        tracing::info!(
            "Successfully indexed memory {} in vector store",
            task.memory_id
        );

        Ok(())
    }
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
        content: String,
    ) -> Result<()> {
        let task = EmbeddingTask {
            memory_id,
            agent_id,
            content,
        };

        self.sender.send(task).await?;
        Ok(())
    }

    /// Queue multiple memories for batch processing
    pub async fn queue_batch(&self, tasks: Vec<EmbeddingTask>) -> Result<()> {
        for task in tasks {
            self.sender.send(task).await?;
        }
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
                "test content".to_string(),
            )
            .await
            .unwrap();

        let task = rx.recv().await.unwrap();
        assert_eq!(task.agent_id, "test_agent");
        assert_eq!(task.content, "test content");
    }
}
