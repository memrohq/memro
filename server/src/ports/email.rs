use async_trait::async_trait;
use anyhow::Result;

#[async_trait]
pub trait EmailService: Send + Sync {
    async fn send_otp(&self, to: &str, code: &str) -> Result<()>;
}
