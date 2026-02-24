use async_trait::async_trait;
use crate::domain::{AgentIdentity, Memory};
use anyhow::Result;
use uuid::Uuid;

#[async_trait]
pub trait IdentityStore: Send + Sync {
    async fn store_identity(&self, identity: &AgentIdentity) -> Result<()>;
    async fn get_identity(&self, id: &str) -> Result<Option<AgentIdentity>>;
    async fn delete_identity(&self, id: &str) -> Result<()>;
}

#[async_trait]
pub trait MemoryStore: Send + Sync {
    async fn store_memory(&self, memory: &Memory) -> Result<()>;
    async fn get_memories_by_agent(&self, agent_id: &str) -> Result<Vec<Memory>>;
    async fn delete_memory(&self, id: &Uuid, agent_id: &str) -> Result<bool>;
    async fn get_memories_filtered(
        &self,
        agent_id: &str,
        memory_type: Option<&str>,
        visibility: Option<&str>,
        limit: Option<i64>,
    ) -> Result<Vec<Memory>>;
}

#[async_trait]
pub trait VectorStore: Send + Sync {
    async fn index_memory(&self, id: &uuid::Uuid, agent_id: &str, vector: Vec<f32>) -> Result<()>;
    async fn search_similar(&self, agent_id: &str, vector: Vec<f32>, limit: usize) -> Result<Vec<uuid::Uuid>>;
}

#[async_trait]
pub trait EmbeddingService: Send + Sync {
    async fn generate_embedding(&self, text: &str) -> Result<Vec<f32>>;
}

pub trait CryptoService: Send + Sync {
    fn generate_keypair(&self) -> (String, String); // (public, private)
    fn verify_signature(&self, public_key: &str, message: &[u8], signature: &str) -> bool;
}


