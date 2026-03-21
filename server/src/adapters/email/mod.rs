use lettre::{
    transport::smtp::authentication::Credentials, AsyncSmtpTransport, AsyncTransport, Message,
    Tokio1Executor,
};
use async_trait::async_trait;
use anyhow::Result;
use crate::ports::email::EmailService;

pub struct LettreEmailService {
    transport: AsyncSmtpTransport<Tokio1Executor>,
    from_address: String,
}

impl LettreEmailService {
    pub fn new(host: &str, port: u16, user: &str, pass: &str) -> Self {
        tracing::info!("Initializing LettreEmailService with host={}, port={}, user={}", host, port, user);
        let credentials = Credentials::new(user.to_string(), pass.to_string());
        let transport = AsyncSmtpTransport::<Tokio1Executor>::starttls_relay(host)
            .unwrap()
            .port(port)
            .credentials(credentials)
            .build();

        Self {
            transport,
            from_address: user.to_string(),
        }
    }
}

#[async_trait]
impl EmailService for LettreEmailService {
    async fn send_otp(&self, to: &str, code: &str) -> Result<()> {
        tracing::debug!("Attempting to send OTP email to {} from {}", to, self.from_address);
        
        let from_mailbox: lettre::message::Mailbox = self.from_address.parse()
            .map_err(|e| anyhow::anyhow!("Invalid 'from' address '{}': {}", self.from_address, e))?;
        let to_mailbox: lettre::message::Mailbox = to.parse()
            .map_err(|e| anyhow::anyhow!("Invalid 'to' address '{}': {}", to, e))?;

        let email = Message::builder()
            .from(from_mailbox)
            .to(to_mailbox)
            .subject("Your Memro Login Code")
            .singlepart(lettre::message::SinglePart::plain(format!(
                "Your Memro OTP code is: {}\n\nThis code will expire in 15 minutes.",
                code
            )))?;

        self.transport.send(email).await
            .map_err(|e| anyhow::anyhow!("SMTP Error: {}", e))?;
            
        tracing::info!("Successfully dispatched OTP email to {}", to);
        Ok(())
    }
}
