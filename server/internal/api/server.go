// Package api is the HTTP layer: routing, middleware and handlers.
package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/memrohq/memro/server/config"
	"github.com/memrohq/memro/server/extension"
	"github.com/memrohq/memro/server/internal/auth"
	"github.com/memrohq/memro/server/internal/connectors"
	"github.com/memrohq/memro/server/internal/email"
	"github.com/memrohq/memro/server/internal/embed"
	"github.com/memrohq/memro/server/internal/engine"
	"github.com/memrohq/memro/server/internal/filestore"
	"github.com/memrohq/memro/server/internal/metrics"
	"github.com/memrohq/memro/server/internal/store"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	engine  *engine.Engine
	embed   *embed.Provider
	indexer engine.Indexer
	files   *filestore.Local
	email   *email.Sender
	replay  *auth.ReplayCache
	meter   extension.UsageMeter
	quota   extension.QuotaPolicy
	extra   map[string]http.Handler

	connectors      *connectors.Manager
	ipLimiter       *RateLimiter
	agentLimiter    *RateLimiter
	otpLimiter      *RateLimiter // sign-in codes requested per email
	identityLimiter *RateLimiter // agents registered per client address
	meeLimiters     map[string]*RateLimiter
	promHandler     http.Handler

	v1LogMu  sync.Mutex
	v1Logged map[string]time.Time
}

type Deps struct {
	Config  config.Config
	Store   *store.Store
	Engine  *engine.Engine
	Embed   *embed.Provider
	Indexer engine.Indexer
	Files   *filestore.Local
	Email   *email.Sender

	// Optional extension points; nil uses the community defaults.
	Usage  extension.UsageMeter
	Quota  extension.QuotaPolicy
	Routes map[string]http.Handler // extra unauthenticated routes, e.g. billing webhooks

	// Connectors syncs Slack and GitHub into memory. When nil, NewServer
	// builds one from Config whose background jobs are not started.
	Connectors *connectors.Manager
}

// StoreUsageMeter records usage in the tenant_usage table.
func StoreUsageMeter(st *store.Store) extension.UsageMeter {
	return extension.UsageMeterFunc(func(ctx context.Context, e extension.UsageEvent) error {
		return st.RecordTenantUsage(ctx, e.TenantID, e.Metric, e.Value)
	})
}

func NewServer(d Deps) *Server {
	if err := d.Config.Validate(); err != nil {
		panic(err) // misconfiguration; app.Run validates this before building the server
	}
	usage, quota := d.Usage, d.Quota
	if usage == nil {
		usage = StoreUsageMeter(d.Store)
	}
	if quota == nil {
		quota = extension.PerAgentCap{Max: d.Config.MemorySoftCap}
	}
	conns := d.Connectors
	if conns == nil {
		var err error
		conns, err = connectors.New(connectors.Deps{Config: d.Config, Pool: d.Store.Pool, Store: d.Store,
			Indexer: d.Indexer, Invalidate: d.Engine.Cache().InvalidateAgent, Quota: quota, Usage: usage})
		if err != nil {
			panic(err) // misconfiguration; app.Run validates this before building the server
		}
	}
	s := &Server{
		connectors: conns,
		cfg:        d.Config, store: d.Store, engine: d.Engine, embed: d.Embed, indexer: d.Indexer,
		files: d.Files, email: d.Email, replay: auth.NewReplayCache(),
		meter: usage, quota: quota, extra: d.Routes,
		ipLimiter:       NewRateLimiter(d.Config.RateLimitMax, d.Config.RateLimitWindow),
		agentLimiter:    NewRateLimiter(d.Config.RateLimitMax, d.Config.RateLimitWindow),
		otpLimiter:      NewRateLimiter(otpCodesPerHour, time.Hour),
		identityLimiter: NewRateLimiter(identitiesPerHour, time.Hour),
		// Per-agent limits for the expensive cognitive endpoints.
		meeLimiters: map[string]*RateLimiter{
			"/mee/reason":  NewRateLimiter(20, time.Minute),
			"/mee/evolve":  NewRateLimiter(5, time.Minute),
			"/mee/resolve": NewRateLimiter(10, time.Minute),
		},
		promHandler: promhttp.Handler(),
	}
	if d.Config.MetricsToken == "" {
		slog.Warn("METRICS_TOKEN is not set; /metrics is open without authentication")
	}
	return s
}

// Handler returns the full HTTP handler with middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	public := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, h) }
	protected := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.requireAgent(h)) }

	public("GET /health", s.health)
	public("GET /metrics", s.requireMetricsToken(s.metrics))
	public("GET /stats/cache", s.requireMetricsToken(s.cacheStats))
	public("POST /identity", s.createIdentity)
	public("GET /identity/pow", s.identityPow)
	public("GET /identity/{id}", s.getIdentity)
	public("GET /assets/{id}", s.getAsset)
	public("POST /auth/login", s.login)
	public("POST /auth/verify", s.verify)
	public("DELETE /auth/logout", s.logout)
	public("POST /auth/device/code", s.deviceCode)
	public("POST /auth/device/token", s.deviceToken)
	public("GET /connectors/{provider}/callback", s.connectorCallback)
	public("POST /connectors/slack/events", s.slackEvents)
	public("POST /connectors/github/webhook", s.githubWebhook)
	public("POST /connectors/linear/webhook", s.linearWebhook)
	public("POST /connectors/sentry/webhook", s.sentryWebhook)
	public("POST /connectors/notion/webhook", s.notionWebhook)
	public("POST /connectors/jira/webhook", s.jiraWebhook)
	public("POST /connectors/gitlab/webhook", s.gitlabWebhook)

	protected("DELETE /identity/{id}", s.deleteIdentity)
	protected("GET /identity/{id}/keys", s.listAgentKeys)
	protected("POST /identity/{id}/keys", s.addAgentKey)
	protected("DELETE /identity/{id}/keys/{key}", s.revokeAgentKey)
	protected("POST /memory", s.createMemory)
	protected("GET /memory/agent/{agent_id}", s.listMemories)
	protected("DELETE /memory/{id}", s.deleteMemory)
	protected("GET /memory/chain/{id}", s.memoryChain)
	protected("POST /memory/generate-atoms", s.generateAtoms)
	protected("POST /memory/from-file", s.memoryFromFile)
	protected("GET /export/{agent_id}", s.export)
	protected("POST /upload", s.upload)
	protected("POST /upload/image", s.uploadImage)
	protected("POST /upload/document", s.uploadDocument)
	protected("GET /search", s.search)
	protected("GET /search/temporal", s.searchTemporal)
	protected("GET /stats/temporal", s.temporalStats)
	protected("GET /analyze/conflicts", s.analyzeConflicts)
	protected("POST /mee/reason", s.reason)
	protected("POST /mee/resolve", s.resolve)
	protected("POST /mee/evolve", s.evolve)
	protected("GET /mee/timeline/{id}", s.timeline)
	protected("GET /usage", s.usage)
	protected("POST /tenant", s.createTenant)
	protected("GET /tenant/{id}", s.getTenant)
	protected("GET /tenant/{id}/usage", s.tenantUsage)
	protected("POST /hooks", s.registerHook)
	protected("GET /hooks/{event_type}", s.getHooks)
	protected("DELETE /hooks", s.deleteHook)
	protected("GET /auth/whoami", s.whoami)
	protected("GET /auth/device", s.deviceInfo)
	protected("POST /auth/device/approve", s.deviceApprove)
	protected("GET /api-keys", s.listAPIKeys)
	protected("POST /api-keys", s.createAPIKey)
	protected("DELETE /api-keys/{id}", s.revokeAPIKey)
	protected("GET /connectors", s.listConnectors)
	protected("POST /connectors/{provider}/authorize", s.authorizeConnector)
	protected("POST /connectors/{provider}/token", s.connectToken)
	protected("GET /connectors/{id}/sources", s.connectorSources)
	protected("PUT /connectors/{id}/sources", s.updateConnectorSources)
	protected("POST /connectors/{id}/sync", s.syncConnector)
	protected("DELETE /connectors/{id}", s.deleteConnector)
	protected("POST /coordination/claim", s.coordinationClaim)
	protected("POST /coordination/heartbeat", s.coordinationHeartbeat)
	protected("POST /coordination/release", s.coordinationRelease)
	protected("POST /coordination/handoff", s.coordinationHandoff)
	protected("POST /coordination/check", s.coordinationCheck)
	protected("GET /coordination/active", s.coordinationActive)
	protected("POST /coordination/intent", s.coordinationIntent)

	for pattern, h := range s.extra {
		mux.Handle(pattern, h)
	}

	var h http.Handler = mux
	h = s.limitBody(h)
	h = s.rateLimitIP(h)
	h = s.cors(h)
	h = observe(h)
	return recoverPanics(h)
}

// --- Request context ------------------------------------------------------------

type Agent struct {
	ID       string
	UserID   *uuid.UUID
	TenantID uuid.UUID
	// Via records how the request authenticated: "session" (dashboard),
	// "api_key" (memro_sk_...) or "signature" (ed25519).
	Via string
	// KeyID is the API key used, when Via is "api_key".
	KeyID *uuid.UUID
	// SigningKey and KeyRole are the agent key that signed the request and its
	// role ("signing" or "recovery"), when Via is "signature" (ADR 0013).
	SigningKey string
	KeyRole    string
}

type ctxKey int

const (
	agentKey ctxKey = iota
	bodyKey
)

func agentFrom(r *http.Request) Agent { return r.Context().Value(agentKey).(Agent) }

// body returns the request body that auth already read and verified.
func body(r *http.Request) []byte {
	if b, ok := r.Context().Value(bodyKey).([]byte); ok {
		return b
	}
	return nil
}

// --- Middleware -----------------------------------------------------------------

func (s *Server) requireAgent(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent, raw, status, msg := s.authenticate(r)
		if status != 0 {
			httpError(w, status, msg)
			return
		}
		// A recovery key may only manage keys (ADR 0013).
		if agent.KeyRole == store.KeyRecovery && !keyManagementRoutes[r.Pattern] {
			httpError(w, http.StatusForbidden, "A recovery key can only manage the agent's keys")
			return
		}
		key := agent.ID + ":" + r.URL.Path
		if !s.agentLimiter.Allow(key) {
			metrics.RateLimitHits.Inc()
			httpError(w, http.StatusTooManyRequests, "Rate limit exceeded for this endpoint")
			return
		}
		if l, ok := s.meeLimiters[r.URL.Path]; ok && !l.Allow(agent.ID) {
			metrics.RateLimitHits.Inc()
			httpError(w, http.StatusTooManyRequests, "Rate limit exceeded for this endpoint")
			return
		}
		ctx := context.WithValue(r.Context(), agentKey, agent)
		ctx = context.WithValue(ctx, bodyKey, raw)
		r = r.WithContext(ctx)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		next(w, r)
	})
}

// authenticate accepts either a dashboard session (Authorization: Bearer) or
// an ed25519-signed agent request (X-Agent-Id / X-Signature / X-Timestamp).
func (s *Server) authenticate(r *http.Request) (Agent, []byte, int, string) {
	ctx := r.Context()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return Agent{}, nil, http.StatusRequestEntityTooLarge, "Request body too large or unreadable"
	}

	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer "+auth.APIKeyPrefix) {
		id, keyID, err := s.store.AgentForAPIKey(ctx, auth.HashToken(strings.TrimPrefix(h, "Bearer ")))
		if err != nil {
			return Agent{}, nil, http.StatusInternalServerError, "Database error"
		}
		if id == nil {
			return Agent{}, nil, http.StatusUnauthorized, "Invalid or revoked API key"
		}
		return Agent{ID: id.ID, UserID: id.UserID, TenantID: id.TenantID, Via: "api_key", KeyID: &keyID}, raw, 0, ""
	}

	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		ses, err := s.store.GetSessionByTokenHash(ctx, auth.HashToken(strings.TrimPrefix(h, "Bearer ")))
		if err != nil {
			return Agent{}, nil, http.StatusInternalServerError, "Database error"
		}
		if ses == nil {
			return Agent{}, nil, http.StatusUnauthorized, "Invalid session token"
		}
		if time.Now().After(ses.ExpiresAt) {
			return Agent{}, nil, http.StatusUnauthorized, "Session expired"
		}
		id, err := s.store.GetIdentityByUser(ctx, ses.UserID)
		if err != nil {
			return Agent{}, nil, http.StatusInternalServerError, "Database error"
		}
		if id == nil {
			return Agent{}, nil, http.StatusUnauthorized, "No agent associated with this user"
		}
		return Agent{ID: id.ID, UserID: id.UserID, TenantID: id.TenantID, Via: "session"}, raw, 0, ""
	}

	agentID, sig, ts := r.Header.Get("X-Agent-Id"), r.Header.Get("X-Signature"), r.Header.Get("X-Timestamp")
	if agentID == "" {
		return Agent{}, nil, http.StatusUnauthorized, "Missing X-Agent-Id or Authorization header"
	}
	if sig == "" {
		return Agent{}, nil, http.StatusUnauthorized, "Missing X-Signature header"
	}
	if len(sig) != 128 {
		return Agent{}, nil, http.StatusUnauthorized, "Invalid signature"
	}
	sigBytes, err := hex.DecodeString(sig)
	if err != nil {
		return Agent{}, nil, http.StatusUnauthorized, "Invalid signature"
	}
	canonicalSig := hex.EncodeToString(sigBytes)
	if ts == "" {
		return Agent{}, nil, http.StatusUnauthorized, "Missing X-Timestamp header"
	}
	if err := auth.CheckTimestamp(ts, time.Now()); err != nil {
		if errors.Is(err, auth.ErrExpired) {
			return Agent{}, nil, http.StatusUnauthorized, "Request timestamp expired"
		}
		return Agent{}, nil, http.StatusUnauthorized, "Invalid X-Timestamp"
	}
	if _, err := auth.ParsePublicKey(agentID); err != nil {
		return Agent{}, nil, http.StatusUnauthorized, "Invalid X-Agent-Id"
	}
	// The key that signed: X-Key-Id, or the agent ID itself (its original
	// key), which is how every request was signed before ADR 0013.
	keyHex := r.Header.Get("X-Key-Id")
	if keyHex == "" {
		keyHex = agentID
	}
	pub, err := auth.ParsePublicKey(keyHex)
	if err != nil {
		return Agent{}, nil, http.StatusUnauthorized, "Invalid X-Key-Id"
	}
	identity, err := s.store.GetIdentity(ctx, agentID)
	if err != nil {
		return Agent{}, nil, http.StatusInternalServerError, "Database error"
	}
	if identity == nil {
		return Agent{}, nil, http.StatusUnauthorized, "Agent identity not registered"
	}
	role, err := s.store.ActiveKeyRole(ctx, agentID, keyHex)
	if err != nil {
		return Agent{}, nil, http.StatusInternalServerError, "Database error"
	}
	if role == "" {
		return Agent{}, nil, http.StatusUnauthorized, "This key is not an active key of the agent"
	}

	switch r.Header.Get("X-Signature-Version") {
	case "2":
		payload := auth.PayloadV2(ts, r.Method, r.URL.RequestURI(), raw)
		if auth.VerifyBytes(pub, payload, sigBytes) != nil {
			return Agent{}, nil, http.StatusUnauthorized, "Invalid signature"
		}
		if !s.replay.Use(canonicalSig, time.Now()) {
			return Agent{}, nil, http.StatusUnauthorized, "Request already used"
		}
	case "", "1":
		if !s.cfg.AllowLegacySignatures {
			return Agent{}, nil, http.StatusUnauthorized, "Legacy signatures are disabled; send X-Signature-Version: 2"
		}
		if auth.VerifyBytes(pub, auth.PayloadV1(ts, raw), sigBytes) != nil {
			return Agent{}, nil, http.StatusUnauthorized, "Invalid signature"
		}
		if !s.replay.Use(sig, time.Now()) {
			return Agent{}, nil, http.StatusUnauthorized, "Request already used"
		}
		metrics.LegacySignatureRequests.Inc()
		s.logLegacyV1(agentID)
	default:
		return Agent{}, nil, http.StatusUnauthorized, "Unsupported X-Signature-Version"
	}
	return Agent{ID: identity.ID, UserID: identity.UserID, TenantID: identity.TenantID, Via: "signature",
		SigningKey: keyHex, KeyRole: role}, raw, 0, ""
}

func (s *Server) logLegacyV1(agentID string) {
	s.v1LogMu.Lock()
	defer s.v1LogMu.Unlock()
	if s.v1Logged == nil {
		s.v1Logged = make(map[string]time.Time)
	}
	now := time.Now()
	if last, ok := s.v1Logged[agentID]; !ok || now.Sub(last) >= time.Hour {
		s.v1Logged[agentID] = now
		slog.Info("accepted request with legacy v1 signature", "agent_id", agentID)
	}
	if len(s.v1Logged) > 10_000 {
		for k, exp := range s.v1Logged {
			if now.Sub(exp) >= 2*time.Hour {
				delete(s.v1Logged, k)
			}
		}
	}
}

// signedWebhooks are authenticated by the sender's HMAC signature and
// arrive from a few provider addresses shared by every customer, so the
// per-IP limit would drop legitimate deliveries.
var signedWebhooks = map[string]bool{
	"/connectors/slack/events":   true,
	"/connectors/github/webhook": true,
	"/connectors/linear/webhook": true,
	"/connectors/sentry/webhook": true,
	"/connectors/notion/webhook": true,
	"/connectors/jira/webhook":   true,
	"/connectors/gitlab/webhook": true,
}

func (s *Server) rateLimitIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metricsExempt := (r.URL.Path == "/metrics" || r.URL.Path == "/stats/cache") && s.validMetricsToken(r)
		if r.URL.Path != "/health" && !metricsExempt && !signedWebhooks[r.URL.Path] && s.ipLimiter != nil && !s.ipLimiter.Allow(s.clientIP(r)) {
			metrics.RateLimitHits.Inc()
			httpError(w, http.StatusTooManyRequests, "Rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		h := w.Header()
		switch {
		case len(s.cfg.AllowedOrigins) == 0:
			h.Set("Access-Control-Allow-Origin", "*")
		case origin != "" && slices.Contains(s.cfg.AllowedOrigins, origin):
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
		}
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Agent-Id, X-Key-Id, X-Signature, X-Timestamp, X-Signature-Version")
		h.Set("Access-Control-Max-Age", "600")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		code := strconv.Itoa(sw.status)
		metrics.HTTPRequestSeconds.WithLabelValues(route, code).Observe(time.Since(start).Seconds())
		if sw.status >= 400 {
			metrics.HTTPErrors.WithLabelValues(code).Inc()
		}
		if r.URL.Path != "/health" && r.URL.Path != "/metrics" && r.URL.Path != "/stats/cache" {
			slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
		}
	})
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("panic", "err", v, "stack", string(debug.Stack()))
				httpError(w, http.StatusInternalServerError, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// clientIP returns the caller's address. Behind a trusted proxy it prefers
// X-Real-IP, which the proxy sets itself, and otherwise the last
// X-Forwarded-For entry, the one the proxy appended. Earlier entries come
// from the client and can be forged.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxyHeaders {
		if real := strings.TrimSpace(r.Header.Get("X-Real-Ip")); real != "" {
			return real
		}
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			parts := strings.Split(fwd, ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
				return last
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// --- Rate limiter ---------------------------------------------------------------

// RateLimiter is a fixed-window counter per key.
type RateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*window
}

type window struct {
	start time.Time
	count int
}

func NewRateLimiter(max int, w time.Duration) *RateLimiter {
	return &RateLimiter{max: max, window: w, hits: map[string]*window{}}
}

func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.hits) > 50_000 {
		for k, w := range l.hits {
			if now.Sub(w.start) >= l.window {
				delete(l.hits, k)
			}
		}
	}
	w, ok := l.hits[key]
	if !ok || now.Sub(w.start) >= l.window {
		l.hits[key] = &window{start: now, count: 1}
		return true
	}
	if w.count >= l.max {
		return false
	}
	w.count++
	return true
}

func (s *Server) validMetricsToken(r *http.Request) bool {
	if s.cfg.MetricsToken == "" {
		return true
	}
	authHeader := r.Header.Get("Authorization")
	expected := "Bearer " + s.cfg.MetricsToken
	return subtle.ConstantTimeCompare([]byte(authHeader), []byte(expected)) == 1
}

func (s *Server) requireMetricsToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validMetricsToken(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	h := s.promHandler
	if h == nil {
		h = promhttp.Handler()
	}
	h.ServeHTTP(w, r)
}

// --- Responses ------------------------------------------------------------------

// httpError writes a plain-text error, matching the Rust backend's format.
func httpError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, msg)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(raw []byte, v any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("empty request body")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("Invalid JSON: %w", err)
	}
	return nil
}

func readJSON(r *http.Request, v any) error {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.New("request body too large or unreadable")
	}
	return decodeJSON(raw, v)
}

func strPtr(s string) *string { return &s }
