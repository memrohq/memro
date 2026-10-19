from pydantic import BaseModel, ConfigDict
from typing import Optional, List, Dict, Any
from datetime import datetime


class Memory(BaseModel):
    model_config = ConfigDict(extra="ignore")

    id: str
    agent_id: str
    content: str
    memory_type: str  # episodic | semantic | procedural | profile
    visibility: str   # private | shared | public
    created_at: datetime
    asset_url: Optional[str] = None
    file_id: Optional[str] = None
    document_date: Optional[datetime] = None
    event_date: Optional[datetime] = None
    session_id: Optional[str] = None
    is_atomic: bool = False
    metadata: Optional[Dict[str, Any]] = None
    entity_tags: List[str] = []
    confidence: float = 1.0
    access_count: int = 0

    @property
    def type(self) -> str:
        return self.memory_type


class SearchResult(BaseModel):
    memory_id: str
    content: str
    score: float
    memory_type: str
    created_at: str
    asset_url: Optional[str] = None
    metadata: Optional[Dict[str, Any]] = None


class TemporalSearchResult(BaseModel):
    memory_id: str
    content: str
    document_date: datetime
    event_date: Optional[datetime] = None
    is_atomic: bool
    score: float
    temporal_context: str


class ExportData(BaseModel):
    agent_id: str
    exported_at: str
    total: int
    memories: List[Memory]


class HealthStatus(BaseModel):
    status: str          # ok | degraded
    db: str              # ok | error
    vector_store: str    # ok | unavailable
    embeddings: Optional[str] = None  # enabled | disabled


class ReasoningResult(BaseModel):
    answer: str
    why: str
    based_on: List[str]
    confidence: float
    paths: List[List[str]]


class EvolutionResult(BaseModel):
    memories_evolved: int
    patterns_identified: List[str]
    diff: Optional[Any] = None


class TimelineEvent(BaseModel):
    timestamp: str
    action: str
    details: str


class MemoryTimeline(BaseModel):
    memory_id: str
    events: List[TimelineEvent]


class ChainItem(BaseModel):
    memory_id: str
    content: str
    relation_type: Optional[str] = None
    confidence: Optional[float] = None


class MemoryChain(BaseModel):
    root_memory: ChainItem
    related_memories: List[ChainItem]


class TemporalStats(BaseModel):
    total_memories: int
    memories_today: int
    memories_this_week: int
    memories_this_month: int
    average_days_between_memories: float
    memory_types_distribution: Dict[str, int]


class UploadedFile(BaseModel):
    file_id: str
    file_name: str
    file_type: str
    file_size: int
    extracted_text: Optional[str] = None
    memory_id: Optional[str] = None
