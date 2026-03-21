use crate::ports::{IdentityStore, MemoryStore, TenantStore, HookStore};
use crate::domain::{AgentIdentity, Memory, MemoryType, Visibility, User, AuthSession, Tenant, Organization, TenantRateLimit};
use sqlx::{PgPool, Row};
use async_trait::async_trait;
use anyhow::Result;
use uuid::Uuid;

pub struct PostgresStore {
    pool: PgPool,
}

impl PostgresStore {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub fn pool(&self) -> &PgPool {
        &self.pool
    }
}

fn memory_type_to_str(t: &MemoryType) -> &'static str {
    match t {
        MemoryType::Episodic => "episodic",
        MemoryType::Semantic => "semantic",
        MemoryType::Procedural => "procedural",
        MemoryType::Profile => "profile",
    }
}

fn memory_type_from_str(s: &str) -> MemoryType {
    match s {
        "episodic" => MemoryType::Episodic,
        "semantic" => MemoryType::Semantic,
        "procedural" => MemoryType::Procedural,
        "profile" => MemoryType::Profile,
        _ => MemoryType::Episodic,
    }
}

fn visibility_to_str(v: &Visibility) -> &'static str {
    match v {
        Visibility::Private => "private",
        Visibility::Shared => "shared",
        Visibility::Public => "public",
    }
}

fn visibility_from_str(s: &str) -> Visibility {
    match s {
        "private" => Visibility::Private,
        "shared" => Visibility::Shared,
        "public" => Visibility::Public,
        _ => Visibility::Private,
    }
}

fn row_to_memory(r: &sqlx::postgres::PgRow) -> Memory {
    let memory_type_str: String = r.get("memory_type");
    let visibility_str: String = r.get("visibility");
    Memory {
        id: r.get("id"),
        agent_id: r.get("agent_id"),
        tenant_id: r.get("tenant_id"),
        content: r.get("content"),
        memory_type: memory_type_from_str(&memory_type_str),
        visibility: visibility_from_str(&visibility_str),
        created_at: r.get("created_at"),
        file_id: r.get("file_id"),
        asset_url: r.get("asset_url"),
        document_date: r.get("document_date"),
        event_date: r.get("event_date"),
        session_id: r.get("session_id"),
        is_atomic: r.get("is_atomic"),
        entity_tags: r.try_get("entity_tags").unwrap_or_default(),
        relationship_types: r.get("relationship_types"),
        access_count: r.get("access_count"),
        confidence: r.get("confidence"),
        last_accessed_at: r.get("last_accessed_at"),
        metadata: r.get("metadata"),
    }
}

#[async_trait]
impl IdentityStore for PostgresStore {

    async fn store_identity(&self, identity: &AgentIdentity) -> Result<()> {
        sqlx::query(
            "INSERT INTO agent_identities (id, user_id, tenant_id, created_at) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO UPDATE SET user_id = $2, tenant_id = $3"
        )
        .bind(&identity.id)
        .bind(&identity.user_id)
        .bind(&identity.tenant_id)
        .bind(identity.created_at)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn get_identity(&self, id: &str) -> Result<Option<AgentIdentity>> {
        let row = sqlx::query(
            "SELECT id, user_id, tenant_id, created_at FROM agent_identities WHERE id = $1"
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(|r| {
            AgentIdentity {
                id: r.get("id"),
                user_id: r.get("user_id"),
                tenant_id: r.get("tenant_id"),
                created_at: r.get("created_at"),
            }
        }))
    }

    async fn delete_identity(&self, id: &str) -> Result<()> {
        sqlx::query("DELETE FROM agent_identities WHERE id = $1")
            .bind(id)
            .execute(&self.pool)
            .await?;
        Ok(())
    }

    async fn get_identities_by_tenant(&self, tenant_id: &Uuid) -> Result<Vec<AgentIdentity>> {
        let rows = sqlx::query(
            "SELECT id, user_id, tenant_id, created_at FROM agent_identities WHERE tenant_id = $1 ORDER BY created_at DESC"
        )
        .bind(tenant_id)
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.iter().map(|r| {
            AgentIdentity {
                id: r.get("id"),
                user_id: r.get("user_id"),
                tenant_id: r.get("tenant_id"),
                created_at: r.get("created_at"),
            }
        }).collect())
    }

    async fn create_user(&self, user: &User, tenant_id: &Uuid) -> Result<()> {
        sqlx::query(
            "INSERT INTO users (id, email, tenant_id, created_at) VALUES ($1, $2, $3, $4) ON CONFLICT (email, tenant_id) DO NOTHING"
        )
        .bind(user.id)
        .bind(&user.email)
        .bind(tenant_id)
        .bind(user.created_at)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn get_user_by_email(&self, email: &str, tenant_id: &Uuid) -> Result<Option<User>> {
        let row = sqlx::query("SELECT id, email, tenant_id, otp_code, otp_expires_at, created_at FROM users WHERE email = $1 AND tenant_id = $2")
        .bind(email)
        .bind(tenant_id)
        .fetch_optional(&self.pool).await?;

        Ok(row.map(|r| User {
            id: r.get("id"),
            email: r.get("email"),
            tenant_id: r.get("tenant_id"),
            otp_code: r.try_get("otp_code").unwrap_or(None),
            otp_expires_at: r.try_get("otp_expires_at").unwrap_or(None),
            created_at: r.get("created_at"),
        }))
    }

    async fn get_user_by_id(&self, id: &Uuid) -> Result<Option<User>> {
        let row = sqlx::query("SELECT id, email, tenant_id, otp_code, otp_expires_at, created_at FROM users WHERE id = $1")
        .bind(id)
        .fetch_optional(&self.pool).await?;

        Ok(row.map(|r| User {
            id: r.get("id"),
            email: r.get("email"),
            tenant_id: r.get("tenant_id"),
            otp_code: r.try_get("otp_code").unwrap_or(None),
            otp_expires_at: r.try_get("otp_expires_at").unwrap_or(None),
            created_at: r.get("created_at"),
        }))
    }

    async fn set_user_otp(&self, email: &str, tenant_id: &Uuid, otp: &str, expires_at: chrono::DateTime<chrono::Utc>) -> Result<()> {
        sqlx::query("UPDATE users SET otp_code = $1, otp_expires_at = $2 WHERE email = $3 AND tenant_id = $4")
            .bind(otp)
            .bind(expires_at)
            .bind(email)
            .bind(tenant_id)
            .execute(&self.pool)
            .await?;
        Ok(())
    }

    async fn clear_user_otp(&self, email: &str, tenant_id: &Uuid) -> Result<()> {
        sqlx::query("UPDATE users SET otp_code = NULL, otp_expires_at = NULL WHERE email = $1 AND tenant_id = $2")
            .bind(email)
            .bind(tenant_id)
            .execute(&self.pool)
            .await?;
        Ok(())
    }

    async fn create_session(&self, session: &AuthSession, tenant_id: &Uuid) -> Result<()> {
        sqlx::query(
            "INSERT INTO sessions (id, user_id, tenant_id, token_hash, expires_at, created_at) VALUES ($1, $2, $3, $4, $5, $6)"
        )
        .bind(session.id)
        .bind(session.user_id)
        .bind(tenant_id)
        .bind(&session.token_hash)
        .bind(session.expires_at)
        .bind(session.created_at)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn get_session(&self, id: &Uuid, tenant_id: &Uuid) -> Result<Option<AuthSession>> {
        let row = sqlx::query("SELECT id, user_id, tenant_id, token_hash, expires_at, created_at FROM sessions WHERE id = $1 AND tenant_id = $2")
        .bind(id)
        .bind(tenant_id)
        .fetch_optional(&self.pool).await?;

        Ok(row.map(|r| AuthSession {
            id: r.get("id"),
            user_id: r.get("user_id"),
            tenant_id: r.get("tenant_id"),
            token_hash: r.get("token_hash"),
            expires_at: r.get("expires_at"),
            created_at: r.get("created_at"),
        }))
    }

    async fn get_session_by_token(&self, token: &str) -> Result<Option<AuthSession>> {
        let row = sqlx::query("SELECT id, user_id, tenant_id, token_hash, expires_at, created_at FROM sessions WHERE token_hash = $1")
        .bind(token)
        .fetch_optional(&self.pool).await?;

        Ok(row.map(|r| AuthSession {
            id: r.get("id"),
            user_id: r.get("user_id"),
            tenant_id: r.get("tenant_id"),
            token_hash: r.get("token_hash"),
            expires_at: r.get("expires_at"),
            created_at: r.get("created_at"),
        }))
    }

    async fn delete_session(&self, id: &Uuid, tenant_id: &Uuid) -> Result<()> {
        sqlx::query("DELETE FROM sessions WHERE id = $1 AND tenant_id = $2").bind(id).bind(tenant_id).execute(&self.pool).await?;
        Ok(())
    }

    async fn delete_session_by_token(&self, token: &str) -> Result<()> {
        sqlx::query("DELETE FROM sessions WHERE token_hash = $1").bind(token).execute(&self.pool).await?;
        Ok(())
    }

    async fn get_agent_identity_by_user_id(&self, user_id: &Uuid) -> Result<Option<AgentIdentity>> {
        let row = sqlx::query(
            "SELECT id, user_id, tenant_id, created_at FROM agent_identities WHERE user_id = $1"
        )
        .bind(user_id)
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(|r| AgentIdentity {
            id: r.get("id"),
            user_id: r.get("user_id"),
            tenant_id: r.get("tenant_id"),
            created_at: r.get("created_at"),
        }))
    }
}

#[async_trait]
impl MemoryStore for PostgresStore {
    async fn store_memory(&self, memory: &Memory, tenant_id: &Uuid) -> Result<()> {
        // P2: Extract entities for pre-filtering
        let entities = crate::services::EntityExtractor::extract_entities(&memory.content);
        
        // P3: Initialize relationship_types as empty (will be populated by background job)
        let relationship_types: Vec<String> = vec![];
        
        sqlx::query(
            "INSERT INTO memories (id, agent_id, tenant_id, content, memory_type, visibility, created_at, file_id, asset_url, document_date, event_date, session_id, is_atomic, entity_tags, relationship_types, metadata)
             VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)"
        )
        .bind(memory.id)
        .bind(&memory.agent_id)
        .bind(tenant_id)
        .bind(&memory.content)
        .bind(memory_type_to_str(&memory.memory_type))
        .bind(visibility_to_str(&memory.visibility))
        .bind(memory.created_at)
        .bind(memory.file_id)
        .bind(&memory.asset_url)
        .bind(memory.document_date)
        .bind(memory.event_date)
        .bind(memory.session_id)
        .bind(memory.is_atomic)
        .bind(&entities)  // P2: Store extracted entities
        .bind(&relationship_types)  // P3: Initialize relationship types
        .bind(&memory.metadata)
        .execute(&self.pool)
        .await?;

        // Notify hook: memory_created
        let payload = serde_json::json!({
            "event": "memory_created",
            "memory_id": memory.id,
            "agent_id": memory.agent_id,
            "content": memory.content,
            "memory_type": memory_type_to_str(&memory.memory_type),
        });
        let _ = self.notify_event(tenant_id, "memory_created", payload).await;

        Ok(())
    }


    async fn get_memories_by_agent(&self, agent_id: &str, tenant_id: &Uuid) -> Result<Vec<Memory>> {
        let rows = sqlx::query(
            "SELECT id, agent_id, tenant_id, content, memory_type, visibility, created_at, file_id, asset_url, document_date, event_date, session_id, is_atomic, entity_tags, relationship_types, access_count, confidence, last_accessed_at, metadata
             FROM memories WHERE agent_id = $1 AND tenant_id = $2 ORDER BY created_at DESC"
        )
        .bind(agent_id)
        .bind(tenant_id)
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.iter().map(row_to_memory).collect())
    }

    async fn delete_memory(&self, id: &Uuid, agent_id: &str, tenant_id: &Uuid) -> Result<bool> {
        let result = sqlx::query(
            "DELETE FROM memories WHERE id = $1 AND agent_id = $2 AND tenant_id = $3"
        )
        .bind(id)
        .bind(agent_id)
        .bind(tenant_id)
        .execute(&self.pool)
        .await?;

        Ok(result.rows_affected() > 0)
    }

    async fn get_memories_filtered(
        &self,
        agent_id: &str,
        tenant_id: &Uuid,
        memory_type: Option<&str>,
        visibility: Option<&str>,
        limit: Option<i64>,
    ) -> Result<Vec<Memory>> {
        let effective_limit = limit.unwrap_or(50).min(100);

        let rows = match (memory_type, visibility) {
            (Some(mt), Some(vis)) => {
                sqlx::query(
                    "SELECT id, agent_id, tenant_id, content, memory_type, visibility, created_at, file_id, asset_url, document_date, event_date, session_id, is_atomic, entity_tags, relationship_types, access_count, confidence, last_accessed_at, metadata
                     FROM memories WHERE agent_id = $1 AND tenant_id = $2 AND memory_type = $3 AND visibility = $4
                     ORDER BY created_at DESC LIMIT $5"
                )
                .bind(agent_id).bind(tenant_id).bind(mt).bind(vis).bind(effective_limit)
                .fetch_all(&self.pool).await?
            }
            (Some(mt), None) => {
                sqlx::query(
                    "SELECT id, agent_id, tenant_id, content, memory_type, visibility, created_at, file_id, asset_url, document_date, event_date, session_id, is_atomic, entity_tags, relationship_types, access_count, confidence, last_accessed_at, metadata
                     FROM memories WHERE agent_id = $1 AND tenant_id = $2 AND memory_type = $3
                     ORDER BY created_at DESC LIMIT $4"
                )
                .bind(agent_id).bind(tenant_id).bind(mt).bind(effective_limit)
                .fetch_all(&self.pool).await?
            }
            (None, Some(vis)) => {
                sqlx::query(
                    "SELECT id, agent_id, tenant_id, content, memory_type, visibility, created_at, file_id, asset_url, document_date, event_date, session_id, is_atomic, entity_tags, relationship_types, access_count, confidence, last_accessed_at, metadata
                     FROM memories WHERE agent_id = $1 AND tenant_id = $2 AND visibility = $3
                     ORDER BY created_at DESC LIMIT $4"
                )
                .bind(agent_id).bind(tenant_id).bind(vis).bind(effective_limit)
                .fetch_all(&self.pool).await?
            }
            (None, None) => {
                sqlx::query(
                    "SELECT id, agent_id, tenant_id, content, memory_type, visibility, created_at, file_id, asset_url, document_date, event_date, session_id, is_atomic, entity_tags, relationship_types, access_count, confidence, last_accessed_at, metadata
                     FROM memories WHERE agent_id = $1 AND tenant_id = $2
                     ORDER BY created_at DESC LIMIT $3"
                )
                .bind(agent_id).bind(tenant_id).bind(effective_limit)
                .fetch_all(&self.pool).await?
            }
        };

        Ok(rows.iter().map(row_to_memory).collect())
    }

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
    ) -> Result<Vec<Memory>> {
        let mut query = String::from(
            "SELECT id, agent_id, tenant_id, content, memory_type, visibility, created_at, file_id, asset_url, document_date, event_date, session_id, is_atomic, entity_tags, relationship_types, access_count, confidence, last_accessed_at, metadata
             FROM memories WHERE agent_id = $1 AND tenant_id = $2"
        );
        let mut arg_count = 2;
        
        if after_date.is_some() {
            arg_count += 1;
            query.push_str(&format!(" AND document_date >= ${}", arg_count));
        }
        if before_date.is_some() {
            arg_count += 1;
            query.push_str(&format!(" AND document_date <= ${}", arg_count));
        }
        if after_event_date.is_some() {
            arg_count += 1;
            query.push_str(&format!(" AND event_date >= ${}", arg_count));
        }
        if before_event_date.is_some() {
            arg_count += 1;
            query.push_str(&format!(" AND event_date <= ${}", arg_count));
        }
        if session_id.is_some() {
            arg_count += 1;
            query.push_str(&format!(" AND session_id = ${}", arg_count));
        }
        if is_atomic.is_some() {
            arg_count += 1;
            query.push_str(&format!(" AND is_atomic = ${}", arg_count));
        }
        
        arg_count += 1;
        query.push_str(&format!(" ORDER BY document_date DESC LIMIT ${}", arg_count));
        
        let mut q = sqlx::query(&query)
            .bind(agent_id)
            .bind(tenant_id);
            
        if let Some(d) = after_date { q = q.bind(d); }
        if let Some(d) = before_date { q = q.bind(d); }
        if let Some(d) = after_event_date { q = q.bind(d); }
        if let Some(d) = before_event_date { q = q.bind(d); }
        if let Some(sid) = session_id { q = q.bind(sid); }
        if let Some(atomic) = is_atomic { q = q.bind(atomic); }
        
        let rows = q.bind(limit).fetch_all(&self.pool).await?;
        Ok(rows.iter().map(row_to_memory).collect())
    }
}

#[async_trait]
impl crate::ports::HookStore for PostgresStore {
    async fn register_hook(&self, tenant_id: &Uuid, event_type: &str, url: &str) -> Result<()> {
        sqlx::query(
            "INSERT INTO hooks (tenant_id, event_type, url) VALUES ($1, $2, $3)
             ON CONFLICT (tenant_id, event_type, url) DO NOTHING"
        )
        .bind(tenant_id)
        .bind(event_type)
        .bind(url)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn get_hooks(&self, tenant_id: &Uuid, event_type: &str) -> Result<Vec<String>> {
        let rows = sqlx::query(
            "SELECT url FROM hooks WHERE tenant_id = $1 AND event_type = $2"
        )
        .bind(tenant_id)
        .bind(event_type)
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.iter().map(|r| r.get("url")).collect())
    }

    async fn notify_event(&self, tenant_id: &Uuid, event_type: &str, payload: serde_json::Value) -> Result<()> {
        let hooks = self.get_hooks(tenant_id, event_type).await?;
        if hooks.is_empty() {
            return Ok(());
        }
        
        // Spawn background task for notifications
        let event_type = event_type.to_string();
        tokio::spawn(async move {
            let client = reqwest::Client::new();
            for url in hooks {
                tracing::debug!("Sending {} notification to {}", event_type, url);
                let _ = client.post(&url)
                    .json(&payload)
                    .send()
                    .await
                    .map_err(|e| tracing::error!("Failed to send {} notification to {}: {}", event_type, url, e));
            }
        });
        
        Ok(())
    }
}

#[async_trait]
impl TenantStore for PostgresStore {
    async fn create_tenant(&self, tenant: &Tenant) -> Result<()> {
        sqlx::query(
            "INSERT INTO tenants (id, name, domain, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)"
        )
        .bind(tenant.id)
        .bind(&tenant.name)
        .bind(&tenant.domain)
        .bind(tenant.created_at)
        .bind(tenant.updated_at)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn get_tenant(&self, id: &Uuid) -> Result<Option<Tenant>> {
        let row = sqlx::query("SELECT id, name, domain, created_at, updated_at FROM tenants WHERE id = $1")
        .bind(id)
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(|r| Tenant {
            id: r.get("id"),
            name: r.get("name"),
            domain: r.get("domain"),
            created_at: r.get("created_at"),
            updated_at: r.get("updated_at"),
        }))
    }

    async fn get_tenant_by_domain(&self, domain: &str) -> Result<Option<Tenant>> {
        let row = sqlx::query("SELECT id, name, domain, created_at, updated_at FROM tenants WHERE domain = $1")
        .bind(domain)
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(|r| Tenant {
            id: r.get("id"),
            name: r.get("name"),
            domain: r.get("domain"),
            created_at: r.get("created_at"),
            updated_at: r.get("updated_at"),
        }))
    }

    async fn update_tenant(&self, tenant: &Tenant) -> Result<()> {
        sqlx::query(
            "UPDATE tenants SET name = $1, domain = $2, updated_at = $3 WHERE id = $4"
        )
        .bind(&tenant.name)
        .bind(&tenant.domain)
        .bind(tenant.updated_at)
        .bind(tenant.id)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn delete_tenant(&self, id: &Uuid) -> Result<()> {
        sqlx::query("DELETE FROM tenants WHERE id = $1")
        .bind(id)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn create_organization(&self, org: &Organization) -> Result<()> {
        sqlx::query(
            "INSERT INTO organizations (id, tenant_id, name, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)"
        )
        .bind(org.id)
        .bind(org.tenant_id)
        .bind(&org.name)
        .bind(org.created_at)
        .bind(org.updated_at)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn get_organizations_by_tenant(&self, tenant_id: &Uuid) -> Result<Vec<Organization>> {
        let rows = sqlx::query("SELECT id, tenant_id, name, created_at, updated_at FROM organizations WHERE tenant_id = $1 ORDER BY created_at DESC")
        .bind(tenant_id)
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.iter().map(|r| Organization {
            id: r.get("id"),
            tenant_id: r.get("tenant_id"),
            name: r.get("name"),
            created_at: r.get("created_at"),
            updated_at: r.get("updated_at"),
        }).collect())
    }

    async fn get_tenant_rate_limit(&self, tenant_id: &Uuid) -> Result<Option<TenantRateLimit>> {
        let row = sqlx::query("SELECT tenant_id, requests_per_minute, burst_limit, created_at, updated_at FROM tenant_rate_limits WHERE tenant_id = $1")
        .bind(tenant_id)
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(|r| TenantRateLimit {
            tenant_id: r.get("tenant_id"),
            requests_per_minute: r.get("requests_per_minute"),
            burst_limit: r.get("burst_limit"),
            created_at: r.get("created_at"),
            updated_at: r.get("updated_at"),
        }))
    }

    async fn set_tenant_rate_limit(&self, limit: &TenantRateLimit) -> Result<()> {
        sqlx::query(
            "INSERT INTO tenant_rate_limits (tenant_id, requests_per_minute, burst_limit, created_at, updated_at)
             VALUES ($1, $2, $3, $4, $5)
             ON CONFLICT (tenant_id) DO UPDATE SET
             requests_per_minute = $2, burst_limit = $3, updated_at = $5"
        )
        .bind(limit.tenant_id)
        .bind(limit.requests_per_minute)
        .bind(limit.burst_limit)
        .bind(limit.created_at)
        .bind(limit.updated_at)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn record_tenant_usage(&self, tenant_id: &Uuid, metric: &str, value: i64) -> Result<()> {
        sqlx::query(
            "INSERT INTO tenant_usage (tenant_id, metric, value, recorded_at) VALUES ($1, $2, $3, $4)"
        )
        .bind(tenant_id)
        .bind(metric)
        .bind(value)
        .bind(chrono::Utc::now())
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn get_tenant_usage(&self, tenant_id: &Uuid, metric: &str, since: chrono::DateTime<chrono::Utc>) -> Result<i64> {
        let row = sqlx::query(
            "SELECT COALESCE(SUM(value), 0) as total FROM tenant_usage WHERE tenant_id = $1 AND metric = $2 AND recorded_at >= $3"
        )
        .bind(tenant_id)
        .bind(metric)
        .bind(since)
        .fetch_one(&self.pool)
        .await?;

        Ok(row.get("total"))
    }
}
