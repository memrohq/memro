// Package store contains all Postgres queries used by the API.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/memrohq/memro/server/internal/domain"
)

type Store struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func (s *Store) Ping(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, "SELECT 1")
	return err
}

// VectorReady reports whether the pgvector extension is installed.
func (s *Store) VectorReady(ctx context.Context) bool {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')`).Scan(&ok)
	return err == nil && ok
}

// --- Identities -------------------------------------------------------------

// CreateIdentity registers a public key. Re-registering an existing key is a
// no-op: it never changes the owner or tenant of an existing identity.
func (s *Store) CreateIdentity(ctx context.Context, id string, userID *uuid.UUID, tenantID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO agent_identities (id, user_id, tenant_id, created_at)
		VALUES ($1, $2, $3, NOW()) ON CONFLICT (id) DO NOTHING`, id, userID, tenantID)
	return err
}

func (s *Store) GetIdentity(ctx context.Context, id string) (*domain.AgentIdentity, error) {
	var a domain.AgentIdentity
	err := s.Pool.QueryRow(ctx, `SELECT id, user_id, tenant_id, created_at FROM agent_identities WHERE id = $1`, id).
		Scan(&a.ID, &a.UserID, &a.TenantID, &a.CreatedAt)
	return nilIfNoRows(&a, err)
}

func (s *Store) GetIdentityByUser(ctx context.Context, userID uuid.UUID) (*domain.AgentIdentity, error) {
	var a domain.AgentIdentity
	err := s.Pool.QueryRow(ctx, `SELECT id, user_id, tenant_id, created_at FROM agent_identities
		WHERE user_id = $1 ORDER BY created_at LIMIT 1`, userID).
		Scan(&a.ID, &a.UserID, &a.TenantID, &a.CreatedAt)
	return nilIfNoRows(&a, err)
}

func (s *Store) DeleteIdentity(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM agent_identities WHERE id = $1`, id)
	return err
}

// --- Users & sessions ---------------------------------------------------------

func (s *Store) GetUserByEmail(ctx context.Context, email string, tenantID uuid.UUID) (*domain.User, error) {
	var u domain.User
	err := s.Pool.QueryRow(ctx, `SELECT id, email, tenant_id, otp_code, otp_expires_at, otp_attempts, created_at
		FROM users WHERE email = $1 AND tenant_id = $2`, email, tenantID).
		Scan(&u.ID, &u.Email, &u.TenantID, &u.OTPHash, &u.OTPExpiresAt, &u.OTPAttempts, &u.CreatedAt)
	return nilIfNoRows(&u, err)
}

func (s *Store) EnsureUser(ctx context.Context, email string, tenantID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO users (id, email, tenant_id, created_at) VALUES ($1, $2, $3, NOW())
		ON CONFLICT (email, tenant_id) DO NOTHING`, uuid.New(), email, tenantID)
	return err
}

func (s *Store) SetUserOTP(ctx context.Context, email string, tenantID uuid.UUID, otpHash string, expires time.Time) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET otp_code = $1, otp_expires_at = $2, otp_attempts = 0
		WHERE email = $3 AND tenant_id = $4`, otpHash, expires, email, tenantID)
	return err
}

func (s *Store) IncrementOTPAttempts(ctx context.Context, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET otp_attempts = otp_attempts + 1 WHERE id = $1`, userID)
	return err
}

func (s *Store) ClearUserOTP(ctx context.Context, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET otp_code = NULL, otp_expires_at = NULL, otp_attempts = 0 WHERE id = $1`, userID)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID, tenantID uuid.UUID, tokenHash string, expires time.Time) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, tenant_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())`, uuid.New(), userID, tenantID, tokenHash, expires)
	return err
}

func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error) {
	var ses domain.Session
	err := s.Pool.QueryRow(ctx, `SELECT id, user_id, tenant_id, expires_at FROM sessions WHERE token_hash = $1`, tokenHash).
		Scan(&ses.ID, &ses.UserID, &ses.TenantID, &ses.ExpiresAt)
	return nilIfNoRows(&ses, err)
}

func (s *Store) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

// --- Memories -----------------------------------------------------------------

const memoryColumns = `id, agent_id, tenant_id, content, memory_type, visibility, created_at, file_id, asset_url,
	COALESCE(document_date, created_at), event_date, session_id, COALESCE(is_atomic, false),
	COALESCE(entity_tags, '{}'), COALESCE(relationship_types, '{}'), COALESCE(access_count, 0),
	COALESCE(confidence, 1.0), COALESCE(last_accessed_at, created_at), metadata`

func scanMemory(row pgx.Row) (domain.Memory, error) {
	var m domain.Memory
	var memType, vis string
	var meta []byte
	err := row.Scan(&m.ID, &m.AgentID, &m.TenantID, &m.Content, &memType, &vis, &m.CreatedAt, &m.FileID, &m.AssetURL,
		&m.DocumentDate, &m.EventDate, &m.SessionID, &m.IsAtomic, &m.EntityTags, &m.RelationshipTypes,
		&m.AccessCount, &m.Confidence, &m.LastAccessedAt, &meta)
	if err != nil {
		return m, err
	}
	m.MemoryType = domain.MemoryType(memType)
	m.Visibility = domain.Visibility(vis)
	if len(meta) > 0 {
		m.Metadata = json.RawMessage(meta)
	}
	return m, nil
}

func collectMemories(rows pgx.Rows, err error) ([]domain.Memory, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Memory{}
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) InsertMemory(ctx context.Context, m *domain.Memory) error {
	var meta any
	if len(m.Metadata) > 0 && string(m.Metadata) != "null" {
		meta = []byte(m.Metadata)
	}
	if m.EntityTags == nil {
		m.EntityTags = []string{}
	}
	if m.RelationshipTypes == nil {
		m.RelationshipTypes = []string{}
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO memories (id, agent_id, tenant_id, content, memory_type, visibility,
			created_at, file_id, asset_url, document_date, event_date, session_id, is_atomic, entity_tags,
			relationship_types, access_count, confidence, last_accessed_at, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		m.ID, m.AgentID, m.TenantID, m.Content, string(m.MemoryType), string(m.Visibility), m.CreatedAt,
		m.FileID, m.AssetURL, m.DocumentDate, m.EventDate, m.SessionID, m.IsAtomic, m.EntityTags,
		m.RelationshipTypes, m.AccessCount, m.Confidence, m.LastAccessedAt, meta)
	return err
}

func (s *Store) CountMemories(ctx context.Context, agentID string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM memories WHERE agent_id = $1`, agentID).Scan(&n)
	return n, err
}

func (s *Store) ListMemories(ctx context.Context, agentID string, tenantID uuid.UUID, memType, visibility string, limit int) ([]domain.Memory, error) {
	q := `SELECT ` + memoryColumns + ` FROM memories WHERE agent_id = $1 AND tenant_id = $2`
	args := []any{agentID, tenantID}
	if memType != "" {
		args = append(args, memType)
		q += fmt.Sprintf(" AND memory_type = $%d", len(args))
	}
	if visibility != "" {
		args = append(args, visibility)
		q += fmt.Sprintf(" AND visibility = $%d", len(args))
	}
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args))
	return collectMemories(s.Pool.Query(ctx, q, args...))
}

func (s *Store) AllMemories(ctx context.Context, agentID string, tenantID uuid.UUID) ([]domain.Memory, error) {
	return collectMemories(s.Pool.Query(ctx, `SELECT `+memoryColumns+` FROM memories
		WHERE agent_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, agentID, tenantID))
}

func (s *Store) GetMemory(ctx context.Context, id uuid.UUID, agentID string, tenantID uuid.UUID) (*domain.Memory, error) {
	m, err := scanMemory(s.Pool.QueryRow(ctx, `SELECT `+memoryColumns+` FROM memories
		WHERE id = $1 AND agent_id = $2 AND tenant_id = $3`, id, agentID, tenantID))
	return nilIfNoRows(&m, err)
}

func (s *Store) DeleteMemory(ctx context.Context, id uuid.UUID, agentID string, tenantID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM memories WHERE id = $1 AND agent_id = $2 AND tenant_id = $3`, id, agentID, tenantID)
	return tag.RowsAffected() > 0, err
}

// --- Embeddings ---------------------------------------------------------------

func (s *Store) UpsertEmbedding(ctx context.Context, memoryID uuid.UUID, tenantID uuid.UUID, agentID, model string, vec []float32) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO memory_embeddings (memory_id, tenant_id, agent_id, model, dims, embedding)
		SELECT $1, $2, $3, $4, $5, $6 WHERE EXISTS (SELECT 1 FROM memories WHERE id = $1)
		ON CONFLICT (memory_id) DO UPDATE SET model = EXCLUDED.model, dims = EXCLUDED.dims,
			embedding = EXCLUDED.embedding, created_at = NOW()`,
		memoryID, tenantID, agentID, model, len(vec), pgvector.NewVector(vec))
	return err
}

type PendingEmbedding struct {
	MemoryID uuid.UUID
	AgentID  string
	TenantID uuid.UUID
	Content  string
}

// MemoriesNeedingEmbedding returns memories with no embedding for the
// current model, e.g. after a model change or a failed embedding call.
func (s *Store) MemoriesNeedingEmbedding(ctx context.Context, model string, limit int) ([]PendingEmbedding, error) {
	rows, err := s.Pool.Query(ctx, `SELECT m.id, m.agent_id, m.tenant_id, m.content FROM memories m
		LEFT JOIN memory_embeddings e ON e.memory_id = m.id AND e.model = $1
		WHERE e.memory_id IS NULL ORDER BY m.created_at DESC LIMIT $2`, model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingEmbedding
	for rows.Next() {
		var p PendingEmbedding
		if err := rows.Scan(&p.MemoryID, &p.AgentID, &p.TenantID, &p.Content); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- Files ----------------------------------------------------------------------

type File struct {
	ID            uuid.UUID
	AgentID       string
	TenantID      uuid.UUID
	FileName      string
	FileType      string
	FilePath      string
	FileSize      int64
	MimeType      string
	ExtractedText *string
}

func (s *Store) InsertFile(ctx context.Context, f *File) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO files (id, agent_id, tenant_id, file_name, file_type, file_path, file_size, mime_type, extracted_text)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		f.ID, f.AgentID, f.TenantID, f.FileName, f.FileType, f.FilePath, f.FileSize, f.MimeType, f.ExtractedText)
	return err
}

func (s *Store) GetFile(ctx context.Context, id uuid.UUID) (*File, error) {
	var f File
	err := s.Pool.QueryRow(ctx, `SELECT id, agent_id, tenant_id, file_name, file_type, file_path, file_size, mime_type, extracted_text
		FROM files WHERE id = $1`, id).
		Scan(&f.ID, &f.AgentID, &f.TenantID, &f.FileName, &f.FileType, &f.FilePath, &f.FileSize, &f.MimeType, &f.ExtractedText)
	return nilIfNoRows(&f, err)
}

// --- Tenants ----------------------------------------------------------------------

func (s *Store) CreateTenant(ctx context.Context, t *domain.Tenant) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO tenants (id, name, domain, created_at, updated_at) VALUES ($1,$2,$3,$4,$5)`,
		t.ID, t.Name, t.Domain, t.CreatedAt, t.UpdatedAt)
	return err
}

func (s *Store) GetTenant(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	var t domain.Tenant
	err := s.Pool.QueryRow(ctx, `SELECT id, name, domain, created_at, updated_at FROM tenants WHERE id = $1`, id).
		Scan(&t.ID, &t.Name, &t.Domain, &t.CreatedAt, &t.UpdatedAt)
	return nilIfNoRows(&t, err)
}

func (s *Store) TenantUsage(ctx context.Context, tenantID uuid.UUID, metric string, since time.Time) (int64, error) {
	var total int64
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(metric_value), 0)::BIGINT FROM tenant_usage
		WHERE tenant_id = $1 AND metric_name = $2 AND recorded_at >= $3`, tenantID, metric, since).Scan(&total)
	return total, err
}

func (s *Store) RecordTenantUsage(ctx context.Context, tenantID uuid.UUID, metric string, value int64) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO tenant_usage (tenant_id, metric_name, metric_value) VALUES ($1, $2, $3)`,
		tenantID, metric, value)
	return err
}

// --- Hooks ------------------------------------------------------------------------

func (s *Store) RegisterHook(ctx context.Context, tenantID uuid.UUID, eventType, url string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO hooks (tenant_id, event_type, url) VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, event_type, url) DO NOTHING`, tenantID, eventType, url)
	return err
}

func (s *Store) GetHooks(ctx context.Context, tenantID uuid.UUID, eventType string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT url FROM hooks WHERE tenant_id = $1 AND event_type = $2 ORDER BY created_at`, tenantID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	urls := []string{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		urls = append(urls, u)
	}
	return urls, rows.Err()
}

// --- Audit log ----------------------------------------------------------------------

func (s *Store) Audit(ctx context.Context, agentID string, tenantID uuid.UUID, action string, resourceID, ip *string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO audit_log (agent_id, tenant_id, action, resource_id, ip_addr) VALUES ($1,$2,$3,$4,$5)`,
		agentID, tenantID, action, resourceID, ip)
	return err
}

type AuditEntry struct {
	CreatedAt  time.Time
	Action     string
	ResourceID string
}

func (s *Store) AuditByResource(ctx context.Context, resourceID string) ([]AuditEntry, error) {
	rows, err := s.Pool.Query(ctx, `SELECT created_at, action, COALESCE(resource_id, '') FROM audit_log
		WHERE resource_id = $1 ORDER BY created_at ASC`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.CreatedAt, &e.Action, &e.ResourceID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ActionCountsToday counts today's audit actions for an agent.
func (s *Store) ActionCountsToday(ctx context.Context, agentID string) (map[string]int64, error) {
	rows, err := s.Pool.Query(ctx, `SELECT action, COUNT(*) FROM audit_log
		WHERE agent_id = $1 AND created_at > CURRENT_DATE GROUP BY action`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var a string
		var n int64
		if err := rows.Scan(&a, &n); err != nil {
			return nil, err
		}
		out[a] = n
	}
	return out, rows.Err()
}

// --- Relations ------------------------------------------------------------------------

func (s *Store) RelationsTouching(ctx context.Context, ids []uuid.UUID) ([]domain.Relation, error) {
	rows, err := s.Pool.Query(ctx, `SELECT source_memory_id, target_memory_id, relation_type, COALESCE(confidence, 1.0)::REAL
		FROM memory_relations WHERE source_memory_id = ANY($1) OR target_memory_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Relation
	for rows.Next() {
		var r domain.Relation
		var t string
		if err := rows.Scan(&r.From, &r.To, &t, &r.Confidence); err != nil {
			return nil, err
		}
		r.Type = domain.RelationType(t)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CountRelationsCreatedBy(ctx context.Context, agentID string, relType string) (int64, error) {
	q := `SELECT COUNT(*) FROM memory_relations WHERE created_by = $1`
	args := []any{agentID}
	if relType != "" {
		q += ` AND relation_type = $2`
		args = append(args, relType)
	}
	var n int64
	err := s.Pool.QueryRow(ctx, q, args...).Scan(&n)
	return n, err
}

func nilIfNoRows[T any](v *T, err error) (*T, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}
