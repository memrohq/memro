use axum::{
    extract::{Multipart, State, Path},
    http::{HeaderMap, StatusCode, header},
    Json,
    Extension,
};
use tokio::fs;
use std::path::PathBuf;
use std::sync::Arc;
use uuid::Uuid;
use chrono::Utc;
use crate::adapters::api::{AppState, audit};
use crate::domain::{Memory, MemoryType, Visibility};

/// Upload an image and generate a semantic description via OpenAI Vision
pub async fn upload_image(
    State(state): State<Arc<AppState>>,
    Extension(agent): Extension<super::middleware::AuthenticatedAgent>,
    mut multipart: Multipart,
) -> Result<Json<Memory>, (StatusCode, String)> {
    let agent_id = &agent.id;

    let mut image_data = None;
    let mut mime_type = None;

    while let Some(field) = multipart.next_field().await.map_err(|e| (StatusCode::BAD_REQUEST, e.to_string()))? {
        let name = field.name().unwrap_or_default().to_string();
        if name == "image" {
            mime_type = field.content_type().map(|s| s.to_string());
            image_data = Some(field.bytes().await.map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?);
        }
    }

    let image_bytes = image_data.ok_or((StatusCode::BAD_REQUEST, "No image found in part".to_string()))?;
    let mime = mime_type.unwrap_or_else(|| "image/jpeg".to_string());
    
    // 1. Describe image using OpenAI
    use base64::Engine;
    let base64_image = base64::engine::general_purpose::STANDARD.encode(&image_bytes);
    let description: String = state.embedding_provider.describe_image(&base64_image, &mime).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, format!("Vision analysis failed: {}", e)))?;

    // 2. Save binary data
    let file_id = Uuid::new_v4();
    let file_ext = match mime.as_str() {
        "image/png" => "png",
        "image/gif" => "gif",
        "image/webp" => "webp",
        _ => "jpg",
    };
    let file_name = format!("{}.{}", file_id, file_ext);
    let mut file_path = PathBuf::from(&state.storage_path);
    file_path.push(&file_name);

    if let Err(e) = fs::write(&file_path, &image_bytes).await {
        tracing::error!("Failed to save image file: {}", e);
        return Err((StatusCode::INTERNAL_SERVER_ERROR, "Failed to save image file".to_string()));
    }

    // 2.5 Register file in DB to satisfy Foreign Key constraint
    let file_size = image_bytes.len() as i64;
    sqlx::query(
        "INSERT INTO files (id, agent_id, file_name, file_type, file_path, file_size, mime_type)
         VALUES ($1, $2, $3, $4, $5, $6, $7)"
    )
    .bind(file_id)
    .bind(agent_id)
    .bind(&file_name)
    .bind("image")
    .bind(file_path.to_string_lossy().to_string())
    .bind(file_size)
    .bind(&mime)
    .execute(&state.db_pool)
    .await
    .map_err(|e| {
        tracing::error!("Failed to register file in DB: {}", e);
        (StatusCode::INTERNAL_SERVER_ERROR, "Failed to register file".to_string())
    })?;

    let asset_url = format!("{}/assets/{}", state.base_url, file_id);

    // 3. Store as memory
    let memory = Memory {
        id: Uuid::new_v4(),
        agent_id: agent_id.to_string(),
        tenant_id: agent.tenant_id,
        content: format!("[IMAGE ANALYSIS] {}", description),
        memory_type: MemoryType::Episodic,
        visibility: Visibility::Private,
        created_at: Utc::now(),
        document_date: Utc::now(),  // Temporal grounding: document created now
        event_date: None,  // No specific event date unless provided
        session_id: None,  // Not associated with a session
        is_atomic: true,  // Image analysis is considered atomic
        entity_tags: vec![],
        relationship_types: vec![],
        file_id: Some(file_id),
        asset_url: Some(asset_url),
        access_count: 0,
        confidence: 1.0,
        last_accessed_at: Utc::now(),
        metadata: None,
    };

    state.memory_store.store_memory(&memory, &agent.tenant_id).await
        .map_err(|e| {
            tracing::error!("Failed to store image memory: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Failed to store memory".to_string())
        })?;

    // 4. Index description
    if let Err(e) = state.embedding_queue.queue_memory(
        memory.id, 
        memory.agent_id.clone(),
        agent.tenant_id,
        memory.content.clone()
    ).await {
        tracing::error!("Failed to queue image memory for embedding: {}", e);
    }

    audit(&state.db_pool, agent_id, &agent.tenant_id, "memory.image_upload", Some(&memory.id.to_string()), None).await;

    Ok(Json(memory))
}

/// Serve a stored asset
pub async fn get_asset(
    State(state): State<Arc<AppState>>,
    Path(id): Path<String>,
) -> Result<(HeaderMap, Vec<u8>), (StatusCode, String)> {
    // Search for the file in the storage directory
    // Since we don't store the extension in the ID, we look for any file starting with ID
    let mut entries = fs::read_dir(&state.storage_path).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    while let Some(entry) = entries.next_entry().await.map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))? {
        let name = entry.file_name().to_string_lossy().to_string();
        if name.starts_with(&id) {
            let data = fs::read(entry.path()).await
                .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;
            
            let mut headers = HeaderMap::new();
            let mime = match entry.path().extension().and_then(|e| e.to_str()) {
                Some("png") => "image/png",
                Some("gif") => "image/gif",
                Some("webp") => "image/webp",
                Some("pdf") => "application/pdf",
                _ => "image/jpeg",
            };
            headers.insert(header::CONTENT_TYPE, mime.parse().unwrap());
            return Ok((headers, data));
        }
    }

    Err((StatusCode::NOT_FOUND, "Asset not found".to_string()))
}

/// Upload a PDF and extract text chunks for semantic indexing
pub async fn upload_document(
    State(state): State<Arc<AppState>>,
    Extension(agent): Extension<super::middleware::AuthenticatedAgent>,
    mut multipart: Multipart,
) -> Result<Json<Vec<Memory>>, (StatusCode, String)> {
    let agent_id = &agent.id;

    let mut doc_data = None;

    while let Some(field) = multipart.next_field().await.map_err(|e| (StatusCode::BAD_REQUEST, e.to_string()))? {
        let name = field.name().unwrap_or_default().to_string();
        if name == "document" {
            doc_data = Some(field.bytes().await.map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?);
        }
    }

    let doc_bytes = doc_data.ok_or((StatusCode::BAD_REQUEST, "No document found in part".to_string()))?;
    
    // 1. Extract PDF text
    let doc = lopdf::Document::load_mem(&doc_bytes)
        .map_err(|e| (StatusCode::BAD_REQUEST, format!("Invalid PDF: {}", e)))?;
    
    let mut full_text = String::new();
    let pages = doc.get_pages();
    for page_num in 1..=pages.len() {
        if let Ok(text) = doc.extract_text(&[page_num as u32]) {
            full_text.push_str(&text);
            full_text.push(' ');
        }
    }

    if full_text.trim().is_empty() {
        return Err((StatusCode::BAD_REQUEST, "No text extractable from PDF".to_string()));
    }

    // 2. Simple Chunking (approx 1500 chars)
    let mut memories = Vec::new();
    let text_chars: Vec<char> = full_text.chars().collect();
    for chunk_chars in text_chars.chunks(1500) {
        let content: String = chunk_chars.iter().collect();
        if content.trim().is_empty() { continue; }

        let memory = Memory {
            id: Uuid::new_v4(),
            agent_id: agent_id.to_string(),
            tenant_id: agent.tenant_id,
            content: format!("[DOC CHUNK] {}", content.trim()),
            memory_type: MemoryType::Semantic,
            visibility: Visibility::Private,
            created_at: Utc::now(),
            document_date: Utc::now(),  // Temporal grounding: document processed now
            event_date: None,  // No specific event date
            session_id: None,  // Not associated with a session
            is_atomic: false,  // PDF chunks are not atomic (might be split)
            entity_tags: vec![],
            relationship_types: vec![],
            file_id: None,
            asset_url: None,
            access_count: 0,
            confidence: 1.0,
            last_accessed_at: Utc::now(),
            metadata: None,
        };

        state.memory_store.store_memory(&memory, &agent.tenant_id).await
            .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;
        
        if let Err(e) = state.embedding_queue.queue_memory(
            memory.id, 
            memory.agent_id.clone(),
            agent.tenant_id,
            memory.content.clone()
        ).await {
            tracing::error!("Failed to queue doc chunk for embedding: {}", e);
        }
        
        memories.push(memory);
    }

    audit(&state.db_pool, agent_id, &agent.tenant_id, "memory.doc_upload", None, None).await;

    Ok(Json(memories))
}
