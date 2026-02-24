use crate::ports::{IdentityStore, MemoryStore};
use crate::domain::{AgentIdentity, Memory, MemoryType, Visibility};
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
        content: r.get("content"),
        memory_type: memory_type_from_str(&memory_type_str),
        visibility: visibility_from_str(&visibility_str),
        created_at: r.get("created_at"),
    }
}

#[async_trait]
impl IdentityStore for PostgresStore {

    async fn store_identity(&self, identity: &AgentIdentity) -> Result<()> {
        sqlx::query(
            "INSERT INTO agent_identities (id, created_at) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING"
        )
        .bind(&identity.id)
        .bind(identity.created_at)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn get_identity(&self, id: &str) -> Result<Option<AgentIdentity>> {
        let row = sqlx::query(
            "SELECT id, created_at FROM agent_identities WHERE id = $1"
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(|r| {
            AgentIdentity {
                id: r.get("id"),
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
}

#[async_trait]
impl MemoryStore for PostgresStore {
    async fn store_memory(&self, memory: &Memory) -> Result<()> {
        sqlx::query(
            "INSERT INTO memories (id, agent_id, content, memory_type, visibility, created_at)
             VALUES ($1, $2, $3, $4, $5, $6)"
        )
        .bind(memory.id)
        .bind(&memory.agent_id)
        .bind(&memory.content)
        .bind(memory_type_to_str(&memory.memory_type))
        .bind(visibility_to_str(&memory.visibility))
        .bind(memory.created_at)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    async fn get_memories_by_agent(&self, agent_id: &str) -> Result<Vec<Memory>> {
        let rows = sqlx::query(
            "SELECT id, agent_id, content, memory_type, visibility, created_at
             FROM memories WHERE agent_id = $1 ORDER BY created_at DESC"
        )
        .bind(agent_id)
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.iter().map(row_to_memory).collect())
    }

    async fn delete_memory(&self, id: &Uuid, agent_id: &str) -> Result<bool> {
        let result = sqlx::query(
            "DELETE FROM memories WHERE id = $1 AND agent_id = $2"
        )
        .bind(id)
        .bind(agent_id)
        .execute(&self.pool)
        .await?;

        Ok(result.rows_affected() > 0)
    }

    async fn get_memories_filtered(
        &self,
        agent_id: &str,
        memory_type: Option<&str>,
        visibility: Option<&str>,
        limit: Option<i64>,
    ) -> Result<Vec<Memory>> {
        let effective_limit = limit.unwrap_or(50).min(100);

        let rows = match (memory_type, visibility) {
            (Some(mt), Some(vis)) => {
                sqlx::query(
                    "SELECT id, agent_id, content, memory_type, visibility, created_at
                     FROM memories WHERE agent_id = $1 AND memory_type = $2 AND visibility = $3
                     ORDER BY created_at DESC LIMIT $4"
                )
                .bind(agent_id).bind(mt).bind(vis).bind(effective_limit)
                .fetch_all(&self.pool).await?
            }
            (Some(mt), None) => {
                sqlx::query(
                    "SELECT id, agent_id, content, memory_type, visibility, created_at
                     FROM memories WHERE agent_id = $1 AND memory_type = $2
                     ORDER BY created_at DESC LIMIT $3"
                )
                .bind(agent_id).bind(mt).bind(effective_limit)
                .fetch_all(&self.pool).await?
            }
            (None, Some(vis)) => {
                sqlx::query(
                    "SELECT id, agent_id, content, memory_type, visibility, created_at
                     FROM memories WHERE agent_id = $1 AND visibility = $2
                     ORDER BY created_at DESC LIMIT $3"
                )
                .bind(agent_id).bind(vis).bind(effective_limit)
                .fetch_all(&self.pool).await?
            }
            (None, None) => {
                sqlx::query(
                    "SELECT id, agent_id, content, memory_type, visibility, created_at
                     FROM memories WHERE agent_id = $1
                     ORDER BY created_at DESC LIMIT $2"
                )
                .bind(agent_id).bind(effective_limit)
                .fetch_all(&self.pool).await?
            }
        };

        Ok(rows.iter().map(row_to_memory).collect())
    }
}
