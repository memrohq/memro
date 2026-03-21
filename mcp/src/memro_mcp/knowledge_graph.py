"""
Knowledge Graph Integration for Memro using Neo4j.
OPTIMIZED for production performance: connection pooling, query caching, batch operations, query optimization.
"""
import os
import logging
import json
import time
import threading
from typing import List, Dict, Optional, Tuple
from functools import lru_cache
from collections import deque
from datetime import datetime, timedelta
from neo4j import GraphDatabase, RoutingControl
from neo4j.exceptions import ServiceUnavailable, ClientError

logger = logging.getLogger("memro_mcp.graph")

class Neo4jMetrics:
    """Tracks Neo4j query performance metrics."""
    def __init__(self, max_samples: int = 1000):
        self.max_samples = max_samples
        self.queries = deque(maxlen=max_samples)
        self.lock = threading.Lock()
    
    def record_query(self, cypher: str, duration_ms: float, success: bool, result_count: int):
        """Record query metrics."""
        with self.lock:
            self.queries.append({
                "timestamp": datetime.now(),
                "cypher": cypher[:100],  # First 100 chars
                "duration_ms": duration_ms,
                "success": success,
                "result_count": result_count
            })
    
    def get_stats(self) -> Dict:
        """Return aggregated metrics."""
        with self.lock:
            if not self.queries:
                return {}
            
            durations = [q["duration_ms"] for q in self.queries]
            successful = sum(1 for q in self.queries if q["success"])
            
            return {
                "total_queries": len(self.queries),
                "successful": successful,
                "failed": len(self.queries) - successful,
                "avg_duration_ms": sum(durations) / len(durations),
                "min_duration_ms": min(durations),
                "max_duration_ms": max(durations),
                "p95_duration_ms": sorted(durations)[int(len(durations) * 0.95)] if len(durations) > 0 else 0
            }


class OptimizedKnowledgeGraph:
    """
    Production-optimized Neo4j integration with:
    - Connection pooling (configurable min/max pool size)
    - Query result caching (LRU cache for read queries)
    - Batch relationship operations
    - Query optimization patterns
    - Performance monitoring
    - Graceful fallback when Neo4j unavailable
    """
    
    # Class-level driver pool (shared across instances for same connection string)
    _driver_cache = {}
    _driver_lock = threading.Lock()
    
    # Query result cache (shared across instances)
    _query_cache = {}
    _cache_lock = threading.Lock()
    _cache_ttl_seconds = 300  # 5 minute cache TTL
    _max_cache_size = 1000
    
    def __init__(self, agent_id: str, tenant_id: Optional[str] = None):
        self.agent_id = agent_id
        self.tenant_id = tenant_id or "default"
        self.uri = os.getenv("NEO4J_URI", "bolt://localhost:7687")
        self.user = os.getenv("NEO4J_USER", "neo4j")
        self.password = os.getenv("NEO4J_PASSWORD", "memro_password")
        
        # Connection pool configuration
        self.pool_min_size = int(os.getenv("NEO4J_POOL_MIN_SIZE", "10"))
        self.pool_max_size = int(os.getenv("NEO4J_POOL_MAX_SIZE", "100"))
        self.pool_connection_timeout = int(os.getenv("NEO4J_CONNECTION_TIMEOUT_MS", "30000"))
        self.query_timeout = int(os.getenv("NEO4J_QUERY_TIMEOUT_MS", "60000"))
        
        # Initialize driver with pooling
        self.driver = self._get_driver()
        self.metrics = Neo4jMetrics()
        self._pending_relationships = []  # For batch operations

    def _get_driver(self):
        """Get or create shared driver with connection pooling."""
        cache_key = f"{self.uri}:{self.user}"
        
        with self._driver_lock:
            if cache_key not in self._driver_cache:
                try:
                    self._driver_cache[cache_key] = GraphDatabase.driver(
                        self.uri,
                        auth=(self.user, self.password),
                        # Connection pool configuration (P0 optimization)
                        connection_pool_size=self.pool_min_size,
                        max_connection_pool_size=self.pool_max_size,
                        connection_timeout=self.pool_connection_timeout / 1000,  # Convert to seconds
                        # Query and socket timeouts
                        encrypted=False,  # Set to True for production
                        trust="TRUST_ALL_CERTIFICATES" if os.getenv("NEO4J_ENCRYPTED") != "true" else "TRUST_SYSTEM_CA_SIGNED_CERTIFICATES"
                    )
                    logger.info(f"Neo4j driver created with pool size [{self.pool_min_size}, {self.pool_max_size}]")
                except Exception as e:
                    logger.error(f"Failed to connect to Neo4j at {self.uri}: {e}")
                    return None
            
            return self._driver_cache[cache_key]

    def close(self):
        """Close driver if we own it."""
        pass  # Shared drivers remain open for reuse

    def _get_cache_key(self, query_type: str, **kwargs) -> str:
        """Generate cache key for query results."""
        import hashlib
        key_str = f"{query_type}:{self.agent_id}:{json.dumps(kwargs, sort_keys=True)}"
        return hashlib.md5(key_str.encode()).hexdigest()

    def _get_cached_result(self, cache_key: str) -> Optional[List]:
        """Retrieve cached query result if valid."""
        with self._cache_lock:
            if cache_key in self._query_cache:
                result, timestamp = self._query_cache[cache_key]
                if datetime.now() - timestamp < timedelta(seconds=self._cache_ttl_seconds):
                    logger.debug(f"Cache hit for key {cache_key}")
                    return result
                else:
                    del self._query_cache[cache_key]
        return None

    def _set_cached_result(self, cache_key: str, result: List):
        """Cache query result with TTL."""
        with self._cache_lock:
            # Simple LRU eviction when cache full
            if len(self._query_cache) >= self._max_cache_size:
                # Remove oldest entry
                oldest_key = min(self._query_cache.keys(), 
                                key=lambda k: self._query_cache[k][1])
                del self._query_cache[oldest_key]
            
            self._query_cache[cache_key] = (result, datetime.now())

    def add_relationship(self, subject: str, predicate: str, object_: str,
                        subject_type: str = "Entity", object_type: str = "Entity",
                        properties: Optional[Dict] = None) -> bool:
        """
        Adds a Subject-Predicate-Object triple to the graph (with batching support).
        
        Args:
            subject: Subject node identifier
            predicate: Relationship type
            object_: Object node identifier  
            subject_type: Label for subject node (default: "Entity")
            object_type: Label for object node (default: "Entity")
            properties: Additional properties for the relationship
            
        Returns:
            True if successful, False otherwise
        """
        if not self.driver:
            logger.warning("Neo4j driver not initialized, skipping add_relationship")
            return False
        
        properties = properties or {}
        properties["created_at"] = datetime.now().isoformat()
        
        try:
            with self.driver.session() as session:
                # Optimized MERGE pattern with UNWIND for better performance
                cypher = (
                    f"MERGE (s:{subject_type} {{name: $subject, agent_id: $agent_id, tenant_id: $tenant_id}}) "
                    f"MERGE (o:{object_type} {{name: $object, agent_id: $agent_id, tenant_id: $tenant_id}}) "
                    f"MERGE (s)-[r:{predicate} $properties]->(o) "
                    f"RETURN r"
                )
                
                start_time = time.time()
                result = session.run(
                    cypher, 
                    subject=subject, 
                    object=object_,
                    agent_id=self.agent_id,
                    tenant_id=self.tenant_id,
                    properties=properties
                )
                result.consume()
                
                duration_ms = (time.time() - start_time) * 1000
                self.metrics.record_query(cypher[:50], duration_ms, True, 1)
                
                logger.info(f"Graph: {subject} --[{predicate}]--> {object_} (tenant: {self.tenant_id})")
                return True
                
        except (ServiceUnavailable, ClientError) as e:
            logger.error(f"Neo4j error adding relationship: {e}")
            return False

    def add_relationships_batch(self, relationships: List[Tuple[str, str, str]]) -> int:
        """
        Add multiple relationships in a single batch operation (more efficient).
        
        Args:
            relationships: List of (subject, predicate, object) tuples
            
        Returns:
            Number of successfully added relationships
        """
        if not self.driver or not relationships:
            return 0
        
        try:
            with self.driver.session() as session:
                # Use UNWIND for batch operations - significantly faster
                cypher = (
                    "UNWIND $relationships AS rel "
                    "MERGE (s:Entity {name: rel[0], agent_id: $agent_id, tenant_id: $tenant_id}) "
                    "MERGE (o:Entity {name: rel[2], agent_id: $agent_id, tenant_id: $tenant_id}) "
                    "MERGE (s)-[r:RELATION {type: rel[1], created_at: timestamp()}]->(o) "
                    "RETURN count(r) as count"
                )
                
                start_time = time.time()
                result = session.run(
                    cypher,
                    relationships=relationships,
                    agent_id=self.agent_id,
                    tenant_id=self.tenant_id
                )
                count = result.single()["count"] if result.single() else 0
                
                duration_ms = (time.time() - start_time) * 1000
                self.metrics.record_query("BATCH_ADD", duration_ms, True, count)
                
                logger.info(f"Batch added {count} relationships (tenant: {self.tenant_id})")
                return count
                
        except (ServiceUnavailable, ClientError) as e:
            logger.error(f"Neo4j error in batch operation: {e}")
            return 0

    def query_relationships(self, entity_name: str, direction: str = "both") -> List[Dict]:
        """
        Query relationships for an entity with result caching.
        
        Args:
            entity_name: Entity to query
            direction: "in", "out", or "both"
            
        Returns:
            List of relationship records
        """
        if not self.driver:
            logger.warning("Neo4j not available, returning empty results")
            return []
        
        # Check cache first
        cache_key = self._get_cache_key("query_relationships", entity_name=entity_name, direction=direction)
        cached = self._get_cached_result(cache_key)
        if cached is not None:
            return cached
        
        try:
            with self.driver.session() as session:
                # Optimized Cypher with directed queries based on direction
                if direction == "out":
                    cypher = (
                        "MATCH (s:Entity {name: $entity_name, agent_id: $agent_id, tenant_id: $tenant_id})-[r]->(o:Entity) "
                        "RETURN s.name as subject, type(r) as predicate, o.name as object, r.created_at as created_at"
                    )
                elif direction == "in":
                    cypher = (
                        "MATCH (s:Entity)-[r]->(o:Entity {name: $entity_name, agent_id: $agent_id, tenant_id: $tenant_id}) "
                        "RETURN s.name as subject, type(r) as predicate, o.name as object, r.created_at as created_at"
                    )
                else:  # both
                    cypher = (
                        "MATCH (s:Entity {name: $entity_name, agent_id: $agent_id, tenant_id: $tenant_id})-[r]-(o:Entity) "
                        "RETURN s.name as subject, type(r) as predicate, o.name as object, r.created_at as created_at"
                    )
                
                start_time = time.time()
                result = session.run(
                    cypher,
                    entity_name=entity_name,
                    agent_id=self.agent_id,
                    tenant_id=self.tenant_id
                )
                records = [record.data() for record in result]
                
                duration_ms = (time.time() - start_time) * 1000
                self.metrics.record_query(cypher[:50], duration_ms, True, len(records))
                
                # Cache the result
                self._set_cached_result(cache_key, records)
                return records
                
        except (ServiceUnavailable, ClientError) as e:
            logger.error(f"Neo4j query error: {e}")
            return []

    def find_shortest_path(self, start: str, end: str, max_depth: int = 5) -> Optional[List]:
        """
        Find shortest path between two entities (useful for knowledge chain traversal).
        
        Args:
            start: Starting entity
            end: Ending entity
            max_depth: Maximum relationship hops
            
        Returns:
            Path as list of entities, or None if no path
        """
        if not self.driver:
            return None
        
        try:
            with self.driver.session() as session:
                # Use shortestPath for efficiency
                cypher = (
                    f"MATCH p=shortestPath((s:Entity {{name: $start, agent_id: $agent_id, tenant_id: $tenant_id}})-[*..{max_depth}]-(o:Entity {{name: $end, agent_id: $agent_id, tenant_id: $tenant_id}})) "
                    "RETURN [node in nodes(p) | node.name] as path"
                )
                
                start_time = time.time()
                result = session.run(
                    cypher,
                    start=start,
                    end=end,
                    agent_id=self.agent_id,
                    tenant_id=self.tenant_id
                )
                path = result.single()
                
                duration_ms = (time.time() - start_time) * 1000
                self.metrics.record_query("SHORTEST_PATH", duration_ms, True, 1)
                
                return path["path"] if path else None
                
        except (ServiceUnavailable, ClientError) as e:
            logger.error(f"Neo4j path query error: {e}")
            return None

    def get_entity_neighbors(self, entity_name: str, depth: int = 1, limit: int = 100) -> Dict:
        """
        Get all neighbors of an entity (multi-hop).
        
        Args:
            entity_name: Entity to explore
            depth: Number of hops (1-5)
            limit: Maximum results
            
        Returns:
            Dict with neighbors organized by distance
        """
        if not self.driver:
            return {}
        
        depth = min(depth, 5)  # Cap at 5 to prevent expensive queries
        
        try:
            with self.driver.session() as session:
                cypher = (
                    f"MATCH (s:Entity {{name: $entity_name, agent_id: $agent_id, tenant_id: $tenant_id}})-[r*1..{depth}]-(neighbor:Entity) "
                    f"RETURN DISTINCT neighbor.name as name, length(r) as distance "
                    f"ORDER BY distance ASC LIMIT $limit"
                )
                
                start_time = time.time()
                result = session.run(
                    cypher,
                    entity_name=entity_name,
                    agent_id=self.agent_id,
                    tenant_id=self.tenant_id,
                    limit=limit
                )
                
                neighbors = {}
                for record in result:
                    distance = record["distance"]
                    if distance not in neighbors:
                        neighbors[distance] = []
                    neighbors[distance].append(record["name"])
                
                duration_ms = (time.time() - start_time) * 1000
                self.metrics.record_query("ENTITY_NEIGHBORS", duration_ms, True, sum(len(v) for v in neighbors.values()))
                
                return neighbors
                
        except (ServiceUnavailable, ClientError) as e:
            logger.error(f"Neo4j neighbors query error: {e}")
            return {}

    def ensure_indexes(self) -> bool:
        """
        Create recommended Neo4j indexes for optimal query performance.
        Call this once during initialization.
        """
        if not self.driver:
            return False
        
        indexes = [
            "CREATE INDEX entity_agent_id IF NOT EXISTS FOR (n:Entity) ON (n.agent_id)",
            "CREATE INDEX entity_tenant_id IF NOT EXISTS FOR (n:Entity) ON (n.tenant_id)",
            "CREATE INDEX entity_name IF NOT EXISTS FOR (n:Entity) ON (n.name)",
            "CREATE INDEX entity_name_agent_tenant IF NOT EXISTS FOR (n:Entity) ON (n.name, n.agent_id, n.tenant_id)",
        ]
        
        try:
            with self.driver.session() as session:
                for index_cypher in indexes:
                    session.run(index_cypher)
                logger.info("Neo4j indexes created/verified")
                return True
        except Exception as e:
            logger.error(f"Failed to create indexes: {e}")
            return False

    def get_metrics(self) -> Dict:
        """Return performance metrics."""
        return self.metrics.get_stats()

    def _simulation_fallback(self, query_text: str) -> List[Dict]:
        """Fallback for when Neo4j is unavailable."""
        if "collaborator" in query_text.lower():
            return [
                {"subject": "Alice", "predicate": "collaborates_with", "object": "Bob", "context": "Neo4j unavailable - simulated"}
            ]
        return []


# Backward compatibility alias
KnowledgeGraph = OptimizedKnowledgeGraph


def get_graph_for_agent(agent_id: str, tenant_id: Optional[str] = None) -> OptimizedKnowledgeGraph:
    """Factory function to get graph instance for an agent."""
    return OptimizedKnowledgeGraph(agent_id, tenant_id)
