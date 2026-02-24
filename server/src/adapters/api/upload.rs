use axum::{
    extract::State,
    http::StatusCode,
    Json,
};
use axum::extract::multipart::Multipart;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use uuid::Uuid;
use sqlx::Row;

use crate::services::{ContentExtractor, FileStorage, OpenAIService};
use super::AppState;

#[derive(Debug, Serialize)]
pub struct UploadResponse {
    pub file_id: String,
    pub file_name: String,
    pub file_type: String,
    pub file_size: i64,
    pub extracted_text: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct CreateMemoryWithFileRequest {
    pub agent_id: String,
    pub file_id: String,
    pub memory_type: String,
    pub visibility: String,
}

/// Upload file endpoint
/// POST /upload
/// Accepts multipart/form-data with file
pub async fn upload_file(
    State(state): State<Arc<AppState>>,
    mut multipart: Multipart,
) -> Result<Json<UploadResponse>, (StatusCode, String)> {
    // Get OpenAI API key from environment
    let openai_key = std::env::var("OPENAI_API_KEY")
        .map_err(|_| (StatusCode::INTERNAL_SERVER_ERROR, "OPENAI_API_KEY not set".to_string()))?;
    
    let openai = OpenAIService::new(openai_key);
    let storage = FileStorage::new()
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Storage init failed: {}", e)))?;
    let extractor = ContentExtractor::new();

    let mut file_name = String::new();
    let mut file_data = Vec::new();
    let mut agent_id = String::new();

    // Parse multipart form
    while let Some(field) = multipart.next_field().await
        .map_err(|e| (StatusCode::BAD_REQUEST, format!("Multipart error: {}", e)))? 
    {
        let name = field.name().unwrap_or("").to_string();
        
        match name.as_str() {
            "file" => {
                file_name = field.file_name().unwrap_or("unknown").to_string();
                file_data = field.bytes().await
                    .map_err(|e| (StatusCode::BAD_REQUEST, format!("Failed to read file: {}", e)))?
                    .to_vec();
            }
            "agent_id" => {
                agent_id = field.text().await
                    .map_err(|e| (StatusCode::BAD_REQUEST, format!("Failed to read agent_id: {}", e)))?;
            }
            _ => {}
        }
    }

    if file_name.is_empty() || file_data.is_empty() {
        return Err((StatusCode::BAD_REQUEST, "No file provided".to_string()));
    }

    if agent_id.is_empty() {
        return Err((StatusCode::BAD_REQUEST, "agent_id is required".to_string()));
    }

    // Detect file type
    let file_type = ContentExtractor::detect_file_type(&file_name);
    let file_size = file_data.len() as i64;

    // Store file
    let file_path = storage.store(&file_name, file_data).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Storage failed: {}", e)))?;

    // Extract text content
    let extracted_text = match file_type.as_str() {
        "audio" => {
            // Use Whisper for audio transcription
            openai.transcribe_audio(&file_path).await.ok()
        }
        _ => {
            // Use content extractor for other types
            extractor.extract(&file_path, &file_type).await.ok()
        }
    };

    // Store file metadata in database
    let file_id = Uuid::new_v4();
    let mime_type = mime_guess::from_path(&file_name)
        .first_or_octet_stream()
        .to_string();

    sqlx::query(
        r#"
        INSERT INTO files (id, agent_id, file_name, file_type, file_path, file_size, mime_type, extracted_text)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        "#
    )
    .bind(&file_id)
    .bind(&agent_id)
    .bind(&file_name)
    .bind(&file_type)
    .bind(&file_path)
    .bind(file_size)
    .bind(&mime_type)
    .bind(extracted_text.as_deref())
    .execute(&state.db_pool)
    .await
    .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Database error: {}", e)))?;

    Ok(Json(UploadResponse {
        file_id: file_id.to_string(),
        file_name,
        file_type,
        file_size,
        extracted_text,
    }))
}

/// Create memory from uploaded file
/// POST /memory/from-file
pub async fn create_memory_from_file(
    State(state): State<Arc<AppState>>,
    Json(req): Json<CreateMemoryWithFileRequest>,
) -> Result<Json<serde_json::Value>, (StatusCode, String)> {
    // Get file metadata
    let file_id_uuid = Uuid::parse_str(&req.file_id)
        .map_err(|e| (StatusCode::BAD_REQUEST, format!("Invalid file_id: {}", e)))?;
    
    let file = sqlx::query(
        r#"
        SELECT file_name, file_type, extracted_text
        FROM files
        WHERE id = $1 AND agent_id = $2
        "#
    )
    .bind(&file_id_uuid)
    .bind(&req.agent_id)
    .fetch_one(&state.db_pool)
    .await
    .map_err(|e| (StatusCode::NOT_FOUND, format!("File not found: {}", e)))?;

    let file_name: String = file.try_get("file_name")
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Database error: {}", e)))?;
    let extracted_text: Option<String> = file.try_get("extracted_text").ok();

    let content = extracted_text
        .unwrap_or_else(|| format!("[File: {}]", file_name));

    // Create memory with file reference
    let memory_id = Uuid::new_v4();
    
    sqlx::query(
        r#"
        INSERT INTO memories (id, agent_id, content, memory_type, visibility, file_id)
        VALUES ($1, $2, $3, $4, $5, $6)
        "#
    )
    .bind(&memory_id)
    .bind(&req.agent_id)
    .bind(&content)
    .bind(&req.memory_type)
    .bind(&req.visibility)
    .bind(&file_id_uuid)
    .execute(&state.db_pool)
    .await
    .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Database error: {}", e)))?;

    // Queue for automatic embedding generation
    if let Err(e) = state.embedding_queue.queue_memory(
        memory_id,
        req.agent_id.clone(),
        content.clone(),
    ).await {
        tracing::error!("Failed to queue file memory for embedding: {}", e);
    }

    Ok(Json(serde_json::json!({
        "memory_id": memory_id.to_string(),
        "file_id": req.file_id,
        "content": content,
    })))
}
