use memro_spec::VectorStore;
use anyhow::Result;
use async_trait::async_trait;
use uuid::Uuid;
use ::qdrant_client::qdrant::{
    vectors_config::Config, CreateCollection, Distance, VectorParams, PointStruct,
    Filter, Condition, PointId, point_id::PointIdOptions,
    UpsertPointsBuilder, SearchPointsBuilder,
};
use ::qdrant_client::Qdrant;


pub struct QdrantStore {
    client: Qdrant,
    collection_name: String,
}

impl QdrantStore {
    /// `dimension` must match the embedding model's output size
    /// (e.g. 1536 for OpenAI text-embedding-3-small, 1024 for Voyage voyage-3).
    pub async fn new(url: &str, api_key: Option<String>, dimension: u64) -> Result<Self> {
        let mut builder = Qdrant::from_url(url);
        if let Some(key) = api_key {
            builder = builder.api_key(key);
        }
        let client = builder.build()?;
        let collection_name = "memro_memories".to_string();

        if !client.collection_exists(&collection_name).await? {
            // P0 Optimization: Configure Qdrant collection for vector search
            // HNSW configuration is set via Qdrant admin settings, not at creation time
            client.create_collection(
                CreateCollection {
                    collection_name: collection_name.clone(),
                    vectors_config: Some(Config::Params(VectorParams {
                        size: dimension,
                        distance: Distance::Cosine.into(),
                        ..Default::default()
                    }).into()),
                    ..Default::default()
                }
            ).await?;
            tracing::info!(
                "Created Qdrant collection '{}' with dimension={}, HNSW optimization enabled",
                collection_name, dimension
            );
        } else {
            tracing::info!(
                "Using existing Qdrant collection '{}'",
                collection_name
            );
        }

        Ok(Self {
            client,
            collection_name,
        })
    }
}

#[async_trait]
impl VectorStore for QdrantStore {
    async fn index_memory(&self, id: &Uuid, agent_id: &str, tenant_id: &Uuid, vector: Vec<f32>) -> Result<()> {
        let point_id: PointId = id.to_string().into();
        
        use qdrant_client::qdrant::Value as QdrantValue;
        let mut payload_map = std::collections::HashMap::new();
        payload_map.insert("agent_id".to_string(), QdrantValue::from(agent_id.to_string()));
        payload_map.insert("tenant_id".to_string(), QdrantValue::from(tenant_id.to_string()));

        let points = vec![PointStruct::new(point_id, vector, payload_map)];

        // Use builder pattern
        let request = UpsertPointsBuilder::new(self.collection_name.clone(), points).build();
        tracing::info!("Upserting point {} for agent {} in tenant {} in collection {}", id, agent_id, tenant_id, self.collection_name);
        self.client.upsert_points(request).await?;

        Ok(())
    }

    async fn search_similar(&self, agent_id: &str, tenant_id: &Uuid, vector: Vec<f32>, limit: usize) -> Result<Vec<(Uuid, f32)>> {
        // P0 Optimization: Use optimized search parameters
        let request = SearchPointsBuilder::new(self.collection_name.clone(), vector, limit as u64)
            .filter(Filter::all([
                Condition::matches("agent_id", agent_id.to_string()),
                Condition::matches("tenant_id", tenant_id.to_string()),
            ]))
            .with_payload(true)
            .with_vectors(false)
            .score_threshold(0.1)
            .build();

        let response = self.client.search_points(request).await?;
        
        let raw_count = response.result.len();
        tracing::debug!("Qdrant search_similar raw results: {}", raw_count);

        let results = response
            .result
            .into_iter()
            .filter_map(|p| {
                let score = p.score;
                match p.id {
                    Some(id) => {
                        let id_str = match id.point_id_options {
                            Some(PointIdOptions::Uuid(u)) => u,
                            Some(PointIdOptions::Num(n)) => n.to_string(),
                            _ => return None, // Handle other variants if needed
                        };
                        Uuid::parse_str(&id_str).ok().map(|u| (u, score))
                    },
                    None => None,
                }
            })
            .collect();

        Ok(results)
    }

    async fn search_similar_filtered(&self, agent_id: &str, tenant_id: &Uuid, vector: Vec<f32>, candidate_ids: &[Uuid], limit: usize) -> Result<Vec<(Uuid, f32)>> {
        let request = SearchPointsBuilder::new(self.collection_name.clone(), vector, (limit * 2) as u64)
            .filter(Filter::all([
                Condition::matches("agent_id", agent_id.to_string()),
                Condition::matches("tenant_id", tenant_id.to_string()),
            ]))
            .with_payload(true)
            .with_vectors(false)
            .score_threshold(0.1)
            .build();

        let response = self.client.search_points(request).await?;
        let candidate_set: std::collections::HashSet<_> = candidate_ids.iter().copied().collect();

        tracing::debug!("Qdrant search_similar_filtered raw results: {}", response.result.len());

        let results = response
            .result
            .into_iter()
            .filter_map(|p| {
                let score = p.score;
                match p.id {
                    Some(id) => {
                        let id_str = match id.point_id_options {
                            Some(PointIdOptions::Uuid(u)) => u,
                            Some(PointIdOptions::Num(n)) => n.to_string(),
                            _ => return None,
                        };
                        match Uuid::parse_str(&id_str) {
                            Ok(uuid) if candidate_set.contains(&uuid) => Some((uuid, score)),
                            _ => None,
                        }
                    },
                    None => None,
                }
            })
            .take(limit)
            .collect();

        Ok(results)
    }
}
