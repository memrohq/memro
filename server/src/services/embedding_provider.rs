//! Pluggable embedding provider with an in-process query cache.
//!
//! Set `EMBEDDING_PROVIDER` to choose a provider:
//!   - `openai`   (default) — OpenAI embeddings + Whisper transcription
//!   - `voyageai` / `voyage` / `anthropic` — Voyage AI embeddings (Anthropic's partner)
//!
//! Keys:
//!   - OpenAI:   `OPENAI_API_KEY`
//!   - Voyage:   `VOYAGE_API_KEY`  (also falls back to `ANTHROPIC_API_KEY`)
//!
//! Model env var: `EMBEDDING_MODEL`
//!   - OpenAI defaults:   `text-embedding-3-small` (1536-dim)
//!   - Voyage defaults:   `voyage-3` (1024-dim)
//!
//! Cache env var: `EMBEDDING_CACHE_SIZE` (default 2048 entries)

use anyhow::Result;
use dashmap::DashMap;
use reqwest::Client;
use serde::Deserialize;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

// ── Shared response types (OpenAI-compatible, Voyage uses same format) ──────

#[derive(Debug, Deserialize)]
struct EmbeddingResponse {
    data: Vec<EmbeddingData>,
}

#[derive(Debug, Deserialize)]
struct EmbeddingData {
    embedding: Vec<f32>,
}

// ── Internal backend enum ─────────────────────────────────────────────────────

#[derive(Debug)]
enum Backend {
    OpenAI { api_key: String, model: String, client: Client },
    VoyageAI { api_key: String, model: String, client: Client },
}

// ── Public provider (backend + cache) ────────────────────────────────────────

/// Thread-safe embedding provider with an in-process LRU-style cache.
/// Share via `Arc<EmbeddingProvider>`.
#[derive(Debug)]
pub struct EmbeddingProvider {
    backend: Backend,
    cache: DashMap<String, Vec<f32>>,
    max_cache_entries: usize,
    // counters for observability
    pub cache_hits: AtomicU64,
    pub cache_misses: AtomicU64,
}

impl EmbeddingProvider {
    /// Initialise from environment variables.
    pub fn from_env() -> Self {
        let provider_name = std::env::var("EMBEDDING_PROVIDER")
            .unwrap_or_else(|_| "openai".to_string())
            .to_lowercase();

        let max_cache_entries: usize = std::env::var("EMBEDDING_CACHE_SIZE")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(2048);

        let client = Client::builder()
            .timeout(Duration::from_secs(30))
            .build()
            .unwrap_or_else(|_| Client::new());

        let backend = match provider_name.as_str() {
            "voyageai" | "voyage" | "anthropic" => {
                let api_key = std::env::var("VOYAGE_API_KEY")
                    .or_else(|_| std::env::var("ANTHROPIC_API_KEY"))
                    .unwrap_or_default();
                let model = std::env::var("EMBEDDING_MODEL")
                    .unwrap_or_else(|_| "voyage-3".to_string());
                tracing::info!("Embedding provider: Voyage AI (model={})", model);
                Backend::VoyageAI { api_key, model, client }
            }
            _ => {
                let api_key = std::env::var("OPENAI_API_KEY").unwrap_or_default();
                let model = std::env::var("EMBEDDING_MODEL")
                    .unwrap_or_else(|_| "text-embedding-3-small".to_string());
                tracing::info!("Embedding provider: OpenAI (model={})", model);
                Backend::OpenAI { api_key, model, client }
            }
        };

        Self {
            backend,
            cache: DashMap::new(),
            max_cache_entries,
            cache_hits: AtomicU64::new(0),
            cache_misses: AtomicU64::new(0),
        }
    }

    /// Human-readable provider name for logging.
    pub fn name(&self) -> &str {
        match &self.backend {
            Backend::OpenAI { .. } => "openai",
            Backend::VoyageAI { .. } => "voyageai",
        }
    }

    /// Expected vector dimension for the configured model.
    /// Used to create the Qdrant collection with the correct size.
    pub fn dimension(&self) -> u64 {
        match &self.backend {
            Backend::OpenAI { model, .. } => match model.as_str() {
                "text-embedding-3-large" => 3072,
                _ => 1536,
            },
            Backend::VoyageAI { model, .. } => match model.as_str() {
                "voyage-3-lite" => 512,
                _ => 1024,
            },
        }
    }

    /// Embed a single text string, returning from cache when available.
    pub async fn embed_text(&self, text: &str) -> Result<Vec<f32>> {
        // Check cache first
        if let Some(cached) = self.cache.get(text) {
            self.cache_hits.fetch_add(1, Ordering::Relaxed);
            return Ok(cached.clone());
        }

        self.cache_misses.fetch_add(1, Ordering::Relaxed);
        let mut batch = self.embed_batch_uncached(vec![text.to_string()]).await?;
        let vector = batch.pop()
            .ok_or_else(|| anyhow::anyhow!("Provider returned empty embedding batch"))?;

        // Evict a random entry when at capacity to bound memory usage
        if self.cache.len() >= self.max_cache_entries {
            if let Some(entry) = self.cache.iter().next().map(|e| e.key().clone()) {
                self.cache.remove(&entry);
            }
        }

        self.cache.insert(text.to_string(), vector.clone());
        tracing::debug!("Generated embedding of length {} for text: {}", vector.len(), text);
        Ok(vector)
    }

    /// Embed multiple texts in one API call (bypasses cache — used for memory indexing).
    pub async fn embed_batch(&self, texts: Vec<String>) -> Result<Vec<Vec<f32>>> {
        self.embed_batch_uncached(texts).await
    }

    async fn embed_batch_uncached(&self, texts: Vec<String>) -> Result<Vec<Vec<f32>>> {
        match &self.backend {
            Backend::OpenAI { api_key, model, client } => {
                let body = serde_json::json!({ "model": model, "input": texts });
                let resp = client
                    .post("https://api.openai.com/v1/embeddings")
                    .header("Authorization", format!("Bearer {}", api_key))
                    .json(&body)
                    .send()
                    .await?;
                if !resp.status().is_success() {
                    anyhow::bail!("OpenAI embeddings error: {}", resp.text().await?);
                }
                let data: EmbeddingResponse = resp.json().await?;
                Ok(data.data.into_iter().map(|d| d.embedding).collect())
            }
            Backend::VoyageAI { api_key, model, client } => {
                let body = serde_json::json!({ "model": model, "input": texts });
                let resp = client
                    .post("https://api.voyageai.com/v1/embeddings")
                    .header("Authorization", format!("Bearer {}", api_key))
                    .json(&body)
                    .send()
                    .await?;
                if !resp.status().is_success() {
                    anyhow::bail!("Voyage AI embeddings error: {}", resp.text().await?);
                }
                let data: EmbeddingResponse = resp.json().await?;
                Ok(data.data.into_iter().map(|d| d.embedding).collect())
            }
        }
    }

    /// Transcribe audio using Whisper (OpenAI only).
    pub async fn transcribe_audio(&self, audio_path: &str) -> Result<String> {
        match &self.backend {
            Backend::OpenAI { api_key, client, .. } => {
                let audio_data = tokio::fs::read(audio_path).await?;
                let form = reqwest::multipart::Form::new()
                    .text("model", "whisper-1")
                    .part(
                        "file",
                        reqwest::multipart::Part::bytes(audio_data)
                            .file_name("audio.mp3")
                            .mime_str("audio/mpeg")?,
                    );
                let resp = client
                    .post("https://api.openai.com/v1/audio/transcriptions")
                    .header("Authorization", format!("Bearer {}", api_key))
                    .multipart(form)
                    .send()
                    .await?;
                if !resp.status().is_success() {
                    anyhow::bail!("Whisper transcription error: {}", resp.text().await?);
                }
                #[derive(Deserialize)]
                struct TranscriptionResponse { text: String }
                let data: TranscriptionResponse = resp.json().await?;
                Ok(data.text)
            }
            Backend::VoyageAI { .. } => {
                anyhow::bail!(
                    "Audio transcription requires EMBEDDING_PROVIDER=openai (Voyage AI does not support Whisper)"
                )
            }
        }
    }

    /// Describe an image using GPT-4o Vision (OpenAI only).
    pub async fn describe_image(&self, base64_image: &str, mime_type: &str) -> Result<String> {
        match &self.backend {
            Backend::OpenAI { api_key, client, .. } => {
                let body = serde_json::json!({
                    "model": "gpt-4o",
                    "messages": [
                        {
                            "role": "user",
                            "content": [
                                {
                                    "type": "text",
                                    "text": "Describe this image in 1-2 concise sentences. Focus on key objects and context for a semantic memory system."
                                },
                                {
                                    "type": "image_url",
                                    "image_url": {
                                        "url": format!("data:{};base64,{}", mime_type, base64_image)
                                    }
                                }
                            ]
                        }
                    ],
                    "max_tokens": 300
                });

                let resp = client
                    .post("https://api.openai.com/v1/chat/completions")
                    .header("Authorization", format!("Bearer {}", api_key))
                    .json(&body)
                    .send()
                    .await?;

                if !resp.status().is_success() {
                    anyhow::bail!("OpenAI Vision error: {}", resp.text().await?);
                }

                #[derive(Deserialize)]
                struct ChatResponse { choices: Vec<Choice> }
                #[derive(Deserialize)]
                struct Choice { message: Message }
                #[derive(Deserialize)]
                struct Message { content: String }

                let data: ChatResponse = resp.json().await?;
                data.choices.first()
                    .map(|c| c.message.content.clone())
                    .ok_or_else(|| anyhow::anyhow!("No vision description returned"))
            }
            Backend::VoyageAI { .. } => {
                anyhow::bail!(
                    "Vision analysis requires EMBEDDING_PROVIDER=openai (Voyage AI does not support GPT-4o Vision)"
                )
            }
        }
    }
}
#[async_trait::async_trait]
impl memro_spec::EmbeddingService for EmbeddingProvider {
    async fn generate_embedding(&self, text: &str) -> Result<Vec<f32>> {
        self.embed_text(text).await
    }
}
