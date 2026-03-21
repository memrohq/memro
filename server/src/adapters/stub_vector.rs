use memro_spec::VectorStore;
use anyhow::Result;
use async_trait::async_trait;
use uuid::Uuid;

pub struct StubVectorStore;

#[async_trait]
impl VectorStore for StubVectorStore {
    async fn index_memory(&self, _id: &Uuid, _agent_id: &str, _tenant_id: &Uuid, _vector: Vec<f32>) -> Result<()> {
        // No-op: semantic search unavailable
        Ok(())
    }

    async fn search_similar(&self, _agent_id: &str, _tenant_id: &Uuid, _vector: Vec<f32>, _limit: usize) -> Result<Vec<(Uuid, f32)>> {
        // Return empty results when Qdrant is unavailable
        Ok(vec![])
    }

    async fn search_similar_filtered(&self, _agent_id: &str, _tenant_id: &Uuid, _vector: Vec<f32>, _candidate_ids: &[Uuid], _limit: usize) -> Result<Vec<(Uuid, f32)>> {
        // Return empty results when Qdrant is unavailable
        Ok(vec![])
    }
}
