use async_trait::async_trait;
pub mod email;
use crate::domain::{AgentIdentity, Memory, User, AuthSession, Tenant, Organization, TenantRateLimit, CollaborationTask};
use anyhow::Result;
use uuid::Uuid;
pub use memro_spec::{VectorStore, EmbeddingService, SearchQuery, SearchResponse, RankingSignals};

#[async_trait]
pub trait TaskStore: Send + Sync {
    async fn create_task(&self, task: &CollaborationTask) -> Result<()>;
    async fn get_task(&self, id: &Uuid, tenant_id: &Uuid) -> Result<Option<CollaborationTask>>;
    async fn update_task(&self, task: &CollaborationTask) -> Result<()>;
    async fn list_tasks(&self, tenant_id: &Uuid) -> Result<Vec<CollaborationTask>>;
    async fn get_active_locks(&self, tenant_id: &Uuid) -> Result<Vec<String>>; // Returns all locked scopes
}

// MemoryEngine trait has been moved to memro-spec

#[async_trait]
pub trait TenantStore: Send + Sync {
    async fn create_tenant(&self, tenant: &Tenant) -> Result<()>;
    async fn get_tenant(&self, id: &Uuid) -> Result<Option<Tenant>>;
    async fn get_tenant_by_domain(&self, domain: &str) -> Result<Option<Tenant>>;
    async fn update_tenant(&self, tenant: &Tenant) -> Result<()>;
    async fn delete_tenant(&self, id: &Uuid) -> Result<()>;

    async fn create_organization(&self, org: &Organization) -> Result<()>;
    async fn get_organizations_by_tenant(&self, tenant_id: &Uuid) -> Result<Vec<Organization>>;

    async fn get_tenant_rate_limit(&self, tenant_id: &Uuid) -> Result<Option<TenantRateLimit>>;
    async fn set_tenant_rate_limit(&self, limit: &TenantRateLimit) -> Result<()>;

    async fn record_tenant_usage(&self, tenant_id: &Uuid, metric: &str, value: i64) -> Result<()>;
    async fn get_tenant_usage(&self, tenant_id: &Uuid, metric: &str, since: chrono::DateTime<chrono::Utc>) -> Result<i64>;
}

#[async_trait]
pub trait IdentityStore: Send + Sync {
    async fn store_identity(&self, identity: &AgentIdentity) -> Result<()>;
    async fn get_identity(&self, id: &str) -> Result<Option<AgentIdentity>>;
    async fn get_identities_by_tenant(&self, tenant_id: &Uuid) -> Result<Vec<AgentIdentity>>;
    async fn delete_identity(&self, id: &str) -> Result<()>;

    async fn create_user(&self, user: &User, tenant_id: &Uuid) -> Result<()>;
    async fn get_user_by_email(&self, email: &str, tenant_id: &Uuid) -> Result<Option<User>>;
    async fn get_user_by_id(&self, id: &Uuid) -> Result<Option<User>>;

    async fn set_user_otp(&self, email: &str, tenant_id: &Uuid, otp: &str, expires_at: chrono::DateTime<chrono::Utc>) -> Result<()>;
    async fn clear_user_otp(&self, email: &str, tenant_id: &Uuid) -> Result<()>;

    async fn create_session(&self, session: &AuthSession, tenant_id: &Uuid) -> Result<()>;
    async fn get_session(&self, id: &Uuid, tenant_id: &Uuid) -> Result<Option<AuthSession>>;
    async fn get_session_by_token(&self, token: &str) -> Result<Option<AuthSession>>; // Token is globally unique
    async fn delete_session(&self, id: &Uuid, tenant_id: &Uuid) -> Result<()>;
    async fn delete_session_by_token(&self, token: &str) -> Result<()>;
    async fn get_agent_identity_by_user_id(&self, user_id: &Uuid) -> Result<Option<AgentIdentity>>;
}

#[async_trait]
pub trait MemoryStore: Send + Sync {
    async fn store_memory(&self, memory: &Memory, tenant_id: &Uuid) -> Result<()>;
    async fn get_memories_by_agent(&self, agent_id: &str, tenant_id: &Uuid) -> Result<Vec<Memory>>;
    async fn delete_memory(&self, id: &Uuid, agent_id: &str, tenant_id: &Uuid) -> Result<bool>;
    async fn get_memories_filtered(
        &self,
        agent_id: &str,
        tenant_id: &Uuid,
        memory_type: Option<&str>,
        visibility: Option<&str>,
        limit: Option<i64>,
    ) -> Result<Vec<Memory>>;
    async fn get_memories_temporal(
        &self,
        agent_id: &str,
        tenant_id: &Uuid,
        after_date: Option<chrono::DateTime<chrono::Utc>>,
        before_date: Option<chrono::DateTime<chrono::Utc>>,
        after_event_date: Option<chrono::DateTime<chrono::Utc>>,
        before_event_date: Option<chrono::DateTime<chrono::Utc>>,
        session_id: Option<Uuid>,
        is_atomic: Option<bool>,
        limit: i64,
    ) -> Result<Vec<Memory>>;
}

// VectorStore and EmbeddingService have been moved to memro-spec

pub trait CryptoService: Send + Sync {
    /// Validate that a hex-encoded string is a valid ed25519 public key.
    fn validate_public_key(&self, public_key: &str) -> bool;
    fn verify_signature(&self, public_key: &str, message: &[u8], signature: &str) -> bool;
}

#[async_trait]
pub trait HookStore: Send + Sync {
    async fn register_hook(&self, tenant_id: &Uuid, event_type: &str, url: &str) -> Result<()>;
    async fn get_hooks(&self, tenant_id: &Uuid, event_type: &str) -> Result<Vec<String>>;
    async fn notify_event(&self, tenant_id: &Uuid, event_type: &str, payload: serde_json::Value) -> Result<()>;
}



