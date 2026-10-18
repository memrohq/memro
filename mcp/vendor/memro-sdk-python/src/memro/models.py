from pydantic import BaseModel
from typing import Optional, List, Dict, Any
from datetime import datetime


class Memory(BaseModel):
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
