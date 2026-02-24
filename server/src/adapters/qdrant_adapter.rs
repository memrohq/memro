use crate::ports::VectorStore;
use anyhow::Result;
use async_trait::async_trait;
use uuid::Uuid;
use ::qdrant_client::qdrant::{
    vectors_config::Config, CreateCollection, Distance, VectorParams, PointStruct,
    SearchPoints, Filter, Condition, PointId, point_id::PointIdOptions,
    UpsertPointsBuilder, SearchPointsBuilder,
};
use ::qdrant_client::Qdrant;
use serde_json::{json, Map, Value};


pub struct QdrantStore {
    client: Qdrant,
    collection_name: String,
}

impl QdrantStore {
    pub async fn new(url: &str) -> Result<Self> {
        // Use new Qdrant API
        let client = Qdrant::from_url(url).build()?;
        let collection_name = "memro_memories".to_string();

        // Create collection if not exists
        if !client.collection_exists(&collection_name).await? {
            client.create_collection(
                CreateCollection {
                    collection_name: collection_name.clone(),
                    vectors_config: Some(Config::Params(VectorParams {
                        size: 1536,
                        distance: Distance::Cosine.into(),
                        ..Default::default()
                    }).into()),
                    ..Default::default()
                }
            ).await?;
        }

        Ok(Self {
            client,
            collection_name,
        })
    }
}

#[async_trait]
impl VectorStore for QdrantStore {
    async fn index_memory(&self, id: &Uuid, agent_id: &str, vector: Vec<f32>) -> Result<()> {
        let point_id: PointId = id.to_string().into();
        
        use qdrant_client::qdrant::Value as QdrantValue;
        let mut payload_map = std::collections::HashMap::new();
        payload_map.insert("agent_id".to_string(), QdrantValue::from(agent_id.to_string()));

        let points = vec![PointStruct::new(point_id, vector, payload_map)];

        // Use builder pattern
        let request = UpsertPointsBuilder::new(self.collection_name.clone(), points).build();
        self.client.upsert_points(request).await?;

        Ok(())
    }

    async fn search_similar(&self, agent_id: &str, vector: Vec<f32>, limit: usize) -> Result<Vec<Uuid>> {
        let request = SearchPointsBuilder::new(self.collection_name.clone(), vector, limit as u64)
            .filter(Filter::all([
                Condition::matches("agent_id", agent_id.to_string()),
            ]))
            .with_payload(true)
            .build();

        let response = self.client.search_points(request).await?;

        let ids = response
            .result
            .into_iter()
            .filter_map(|p| {
                match p.id {
                    Some(id) => {
                        let id_str = match id.point_id_options {
                            Some(PointIdOptions::Uuid(u)) => u,
                            Some(PointIdOptions::Num(n)) => n.to_string(),
                            None => return None,
                        };
                        Uuid::parse_str(&id_str).ok()
                    },
                    None => None,
                }
            })
            .collect();

        Ok(ids)
    }
}
