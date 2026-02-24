use anyhow::Result;
use std::path::PathBuf;
use tokio::fs;
use tokio::io::AsyncWriteExt;

#[derive(Debug, Clone)]
pub enum StorageType {
    Local,
    S3,
}

#[derive(Debug, Clone)]
pub struct FileStorage {
    storage_type: StorageType,
    base_path: PathBuf,
}

impl FileStorage {
    pub fn new() -> Result<Self> {
        let storage_type = match std::env::var("FILE_STORAGE_TYPE")
            .unwrap_or_else(|_| "local".to_string())
            .as_str()
        {
            "s3" => StorageType::S3,
            _ => StorageType::Local,
        };

        let base_path = std::env::var("FILE_STORAGE_PATH")
            .unwrap_or_else(|_| "./data/files".to_string())
            .into();

        Ok(Self {
            storage_type,
            base_path,
        })
    }

    /// Store file and return the storage path
    pub async fn store(&self, file_name: &str, data: Vec<u8>) -> Result<String> {
        match self.storage_type {
            StorageType::Local => self.store_local(file_name, data).await,
            StorageType::S3 => self.store_s3(file_name, data).await,
        }
    }

    /// Store file locally
    async fn store_local(&self, file_name: &str, data: Vec<u8>) -> Result<String> {
        // Create storage directory if it doesn't exist
        fs::create_dir_all(&self.base_path).await?;

        // Generate unique file path
        let file_id = uuid::Uuid::new_v4();
        let extension = std::path::Path::new(file_name)
            .extension()
            .and_then(|e| e.to_str())
            .unwrap_or("bin");
        
        let file_path = self.base_path.join(format!("{}.{}", file_id, extension));

        // Write file
        let mut file = fs::File::create(&file_path).await?;
        file.write_all(&data).await?;
        file.flush().await?;

        Ok(file_path.to_string_lossy().to_string())
    }

    /// Store file in S3
    async fn store_s3(&self, _file_name: &str, _data: Vec<u8>) -> Result<String> {
        // TODO: Implement S3 storage using aws-sdk-s3
        tracing::warn!("S3 storage not yet implemented, falling back to local");
        self.store_local(_file_name, _data).await
    }

    /// Retrieve file data
    pub async fn retrieve(&self, file_path: &str) -> Result<Vec<u8>> {
        match self.storage_type {
            StorageType::Local => self.retrieve_local(file_path).await,
            StorageType::S3 => self.retrieve_s3(file_path).await,
        }
    }

    /// Retrieve file from local storage
    async fn retrieve_local(&self, file_path: &str) -> Result<Vec<u8>> {
        let data = fs::read(file_path).await?;
        Ok(data)
    }

    /// Retrieve file from S3
    async fn retrieve_s3(&self, _file_path: &str) -> Result<Vec<u8>> {
        // TODO: Implement S3 retrieval
        tracing::warn!("S3 retrieval not yet implemented");
        anyhow::bail!("S3 retrieval not implemented")
    }

    /// Delete file
    pub async fn delete(&self, file_path: &str) -> Result<()> {
        match self.storage_type {
            StorageType::Local => fs::remove_file(file_path).await?,
            StorageType::S3 => {
                // TODO: Implement S3 deletion
                tracing::warn!("S3 deletion not yet implemented");
            }
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_store_and_retrieve_local() {
        let storage = FileStorage::new().unwrap();
        
        let data = b"Hello, world!".to_vec();
        let path = storage.store("test.txt", data.clone()).await.unwrap();
        
        let retrieved = storage.retrieve(&path).await.unwrap();
        assert_eq!(retrieved, data);
        
        // Cleanup
        storage.delete(&path).await.ok();
    }
}
