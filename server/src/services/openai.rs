use anyhow::Result;
use serde::{Deserialize, Serialize};
use reqwest::Client;

#[derive(Debug, Clone)]
pub struct OpenAIService {
    api_key: String,
    client: Client,
    model: String,
}

#[derive(Debug, Serialize)]
struct EmbeddingRequest {
    model: String,
    input: Vec<String>,
}

#[derive(Debug, Deserialize)]
struct EmbeddingResponse {
    data: Vec<EmbeddingData>,
}

#[derive(Debug, Deserialize)]
struct EmbeddingData {
    embedding: Vec<f32>,
}

#[derive(Debug, Serialize)]
struct TranscriptionRequest {
    model: String,
    file: String,
}

impl OpenAIService {
    pub fn new(api_key: String) -> Self {
        let model = std::env::var("EMBEDDING_MODEL")
            .unwrap_or_else(|_| "text-embedding-3-small".to_string());
        
        Self {
            api_key,
            client: Client::new(),
            model,
        }
    }

    /// Generate embedding for a single text
    pub async fn embed_text(&self, text: &str) -> Result<Vec<f32>> {
        let embeddings = self.embed_batch(vec![text.to_string()]).await?;
        Ok(embeddings.into_iter().next().unwrap())
    }

    /// Generate embeddings for multiple texts (batch)
    pub async fn embed_batch(&self, texts: Vec<String>) -> Result<Vec<Vec<f32>>> {
        let request = EmbeddingRequest {
            model: self.model.clone(),
            input: texts,
        };

        let response = self.client
            .post("https://api.openai.com/v1/embeddings")
            .header("Authorization", format!("Bearer {}", self.api_key))
            .header("Content-Type", "application/json")
            .json(&request)
            .send()
            .await?;

        if !response.status().is_success() {
            let error_text = response.text().await?;
            anyhow::bail!("OpenAI API error: {}", error_text);
        }

        let data: EmbeddingResponse = response.json().await?;
        Ok(data.data.into_iter().map(|d| d.embedding).collect())
    }

    /// Transcribe audio file using Whisper API
    pub async fn transcribe_audio(&self, audio_path: &str) -> Result<String> {
        // Read audio file
        let audio_data = tokio::fs::read(audio_path).await?;
        
        // Create multipart form
        let form = reqwest::multipart::Form::new()
            .text("model", "whisper-1")
            .part(
                "file",
                reqwest::multipart::Part::bytes(audio_data)
                    .file_name("audio.mp3")
                    .mime_str("audio/mpeg")?
            );

        let response = self.client
            .post("https://api.openai.com/v1/audio/transcriptions")
            .header("Authorization", format!("Bearer {}", self.api_key))
            .multipart(form)
            .send()
            .await?;

        if !response.status().is_success() {
            let error_text = response.text().await?;
            anyhow::bail!("Whisper API error: {}", error_text);
        }

        #[derive(Deserialize)]
        struct TranscriptionResponse {
            text: String,
        }

        let data: TranscriptionResponse = response.json().await?;
        Ok(data.text)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_embed_text() {
        let api_key = std::env::var("OPENAI_API_KEY").unwrap();
        let service = OpenAIService::new(api_key);
        
        let embedding = service.embed_text("Hello, world!").await.unwrap();
        assert_eq!(embedding.len(), 1536); // text-embedding-3-small dimension
    }

    #[tokio::test]
    async fn test_embed_batch() {
        let api_key = std::env::var("OPENAI_API_KEY").unwrap();
        let service = OpenAIService::new(api_key);
        
        let texts = vec![
            "First text".to_string(),
            "Second text".to_string(),
        ];
        
        let embeddings = service.embed_batch(texts).await.unwrap();
        assert_eq!(embeddings.len(), 2);
        assert_eq!(embeddings[0].len(), 1536);
    }
}
