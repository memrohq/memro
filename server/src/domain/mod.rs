pub mod memory;

use serde::{Deserialize, Serialize};
use chrono::{DateTime, Utc};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentIdentity {
    pub id: String, // Public Key (Hex or Base58)
    pub created_at: DateTime<Utc>,
}

pub use memory::{Memory, MemoryType, Visibility};
