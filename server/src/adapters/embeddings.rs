use crate::ports::EmbeddingService;
use anyhow::Result;
use async_trait::async_trait;
use rand::Rng;

pub struct MockEmbeddingService;

#[async_trait]
impl EmbeddingService for MockEmbeddingService {
    async fn generate_embedding(&self, _text: &str) -> Result<Vec<f32>> {
        // Generate a random 1536-dimensional vector (typical OpenAI size)
        let mut rng = rand::thread_rng();
        let vector: Vec<f32> = (0..1536).map(|_| rng.gen_range(-1.0..1.0)).collect();
        Ok(vector)
    }
}
