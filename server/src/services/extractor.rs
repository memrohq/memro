use anyhow::Result;
use std::path::Path;
use tokio::fs;
use crate::services::EmbeddingProvider;

/// Extract text content from different file types
pub struct ContentExtractor;

impl ContentExtractor {
    pub fn new() -> Self {
        Self
    }

    /// Extract text from any supported file type
    /// Now requires EmbeddingProvider for Vision and Whisper transcription
    pub async fn extract(&self, file_path: &str, file_type: &str, provider: &EmbeddingProvider) -> Result<String> {
        match file_type {
            "pdf" => self.extract_pdf(file_path).await,
            "image" => self.extract_image(file_path, provider).await,
            "audio" | "video" => self.extract_audio(file_path, provider).await,
            "text" => self.extract_text(file_path).await,
            "docx" => self.extract_docx(file_path).await,
            _ => Ok(String::new()),
        }
    }

    /// Extract text from PDF
    async fn extract_pdf(&self, file_path: &str) -> Result<String> {
        let bytes = fs::read(file_path).await?;
        let doc = lopdf::Document::load_mem(&bytes)?;
        
        let mut full_text = String::new();
        let pages = doc.get_pages();
        for page_num in 1..=pages.len() {
            if let Ok(text) = doc.extract_text(&[page_num as u32]) {
                full_text.push_str(&text);
                full_text.push(' ');
            }
        }
        
        if full_text.trim().is_empty() {
             return Ok(format!("[PDF: {} (no text extracted)]", file_path));
        }
        
        Ok(full_text.trim().to_string())
    }

    /// Extract text from image using existing Vision provider
    async fn extract_image(&self, file_path: &str, provider: &EmbeddingProvider) -> Result<String> {
        let bytes = fs::read(file_path).await?;
        let mime = mime_guess::from_path(file_path).first_or_octet_stream().to_string();
        
        use base64::Engine;
        let base64_image = base64::engine::general_purpose::STANDARD.encode(&bytes);
        
        let description = provider.describe_image(&base64_image, &mime).await?;
        Ok(format!("[IMAGE ANALYSIS] {}", description))
    }

    /// Extract text from audio/video using existing Whisper provider
    async fn extract_audio(&self, file_path: &str, provider: &EmbeddingProvider) -> Result<String> {
        let transcription = provider.transcribe_audio(file_path).await?;
        Ok(transcription)
    }

    /// Placeholder for Docx
    async fn extract_docx(&self, file_path: &str) -> Result<String> {
        tracing::warn!("Docx extraction placeholder for {}", file_path);
        Ok(format!("[Word Document content from {}]", file_path))
    }

    /// Extract text from plain text file
    async fn extract_text(&self, file_path: &str) -> Result<String> {
        let content = fs::read_to_string(file_path).await?;
        Ok(content)
    }

    /// Determine file type from extension
    pub fn detect_file_type(file_name: &str) -> String {
        let path = Path::new(file_name);
        let extension = path.extension()
            .and_then(|e| e.to_str())
            .unwrap_or("")
            .to_lowercase();

        match extension.as_str() {
            "pdf" => "pdf",
            "png" | "jpg" | "jpeg" | "gif" | "webp" | "bmp" | "heic" | "tiff" => "image",
            "mp3" | "wav" | "m4a" | "ogg" | "flac" => "audio",
            "mp4" | "mov" | "avi" | "mkv" | "webm" => "audio", // Video processed as audio
            "txt" | "md" | "json" | "csv" | "yaml" | "yml" | "xml" | "html" | "css" | 
            "rs" | "py" | "js" | "ts" | "tsx" | "jsx" | "go" | "c" | "cpp" | "h" | "sh" | "sql" => "text",
            "docx" | "doc" => "docx",
            "xlsx" | "xls" => "xlsx",
            "pptx" | "ppt" => "pptx",
            _ => "unknown",
        }.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_detect_file_type() {
        assert_eq!(ContentExtractor::detect_file_type("document.pdf"), "pdf");
        assert_eq!(ContentExtractor::detect_file_type("image.png"), "image");
        assert_eq!(ContentExtractor::detect_file_type("audio.mp3"), "audio");
        assert_eq!(ContentExtractor::detect_file_type("video.mp4"), "audio");
        assert_eq!(ContentExtractor::detect_file_type("main.rs"), "text");
        assert_eq!(ContentExtractor::detect_file_type("script.py"), "text");
    }

    #[tokio::test]
    async fn test_extract_text() {
        let extractor = ContentExtractor::new();
        
        let temp_path = "/tmp/test_code.rs";
        tokio::fs::write(temp_path, "fn main() {}").await.unwrap();
        
        let content = extractor.extract_text(temp_path).await.unwrap();
        assert_eq!(content, "fn main() {}");
        
        tokio::fs::remove_file(temp_path).await.ok();
    }
}
