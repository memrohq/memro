// Package metrics defines the Prometheus metrics exposed on /metrics. Names
// match the Rust backend so existing dashboards keep working.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	MemoriesCreated         = counter("memro_memories_created_total", "Total memories created")
	MemoriesDeleted         = counter("memro_memories_deleted_total", "Total memories deleted")
	SearchesTotal           = counter("memro_searches_total", "Total searches executed")
	IdentitiesCreated       = counter("memro_identities_created_total", "Total agent identities created")
	TenantsCreated          = counter("memro_tenants_created_total", "Total tenants created")
	SearchCacheHits         = counter("memro_search_cache_hits_total", "Search result cache hits")
	SearchCacheMisses       = counter("memro_search_cache_misses_total", "Search result cache misses")
	RateLimitHits           = counter("memro_rate_limit_hits_total", "Requests rejected by the rate limiter")
	LegacySignatureRequests = counter("memro_legacy_signature_requests_total", "Requests accepted with legacy v1 signatures")
	HTTPErrors              = promauto.NewCounterVec(prometheus.CounterOpts{Name: "memro_http_errors_total", Help: "HTTP error responses"}, []string{"status"})
	EmbeddingQueue          = promauto.NewGauge(prometheus.GaugeOpts{Name: "memro_embedding_queue_depth", Help: "Memories waiting to be embedded"})
	EmbeddingFailures       = counter("memro_embedding_failures_total", "Embedding batches that failed")
	ConnectorJobs           = promauto.NewCounterVec(prometheus.CounterOpts{Name: "memro_connector_jobs_total", Help: "Connector jobs run, by kind and result"}, []string{"kind", "result"})
	ConnectorWebhooks       = promauto.NewCounterVec(prometheus.CounterOpts{Name: "memro_connector_webhooks_total", Help: "Connector webhooks received, by provider and result"}, []string{"provider", "result"})
	ConnectorMemories       = promauto.NewCounterVec(prometheus.CounterOpts{Name: "memro_connector_memories_total", Help: "Memories created or deleted by connectors"}, []string{"provider", "action"})
	ConnectorQueue          = promauto.NewGauge(prometheus.GaugeOpts{Name: "memro_connector_jobs_due", Help: "Connector jobs due to run"})
	HTTPRequestSeconds      = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "memro_http_request_duration_seconds", Help: "HTTP request latency", Buckets: prometheus.DefBuckets,
	}, []string{"route", "status"})
	CoordinationClaims      = promauto.NewCounterVec(prometheus.CounterOpts{Name: "memro_coordination_claims_total", Help: "Total coordination claims by outcome"}, []string{"outcome"})
	CoordinationExpirations = counter("memro_coordination_expirations_total", "Total expired coordination leases reaped")
	CoordinationHandoffs    = counter("memro_coordination_handoffs_total", "Total coordination task handoffs")
)

func counter(name, help string) prometheus.Counter {
	return promauto.NewCounter(prometheus.CounterOpts{Name: name, Help: help})
}
