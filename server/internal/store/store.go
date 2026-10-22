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
	"github.com/jackc/pgx/v5/pgconn"
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

// --- Agent keys (ADR 0013) ---------------------------------------------------

// Key roles: a signing key acts for the agent; a recovery key may only manage
// keys.
const (
	KeySigning  = "signing"
	KeyRecovery = "recovery"
)

// ErrKeyInUse means the public key already belongs to an agent (possibly
// revoked); keys are never reused.
var ErrKeyInUse = errors.New("key already in use")

type AgentKey struct {
	PublicKey string     `json:"public_key"`
	Role      string     `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// ActiveKeyRole returns the role of an active key of the agent, or "" when
// the key is not an active key of that agent.
func (s *Store) ActiveKeyRole(ctx context.Context, agentID, publicKey string) (string, error) {
	var role string
	err := s.Pool.QueryRow(ctx, `SELECT role FROM agent_keys
		WHERE agent_id = $1 AND public_key = $2 AND revoked_at IS NULL`, agentID, publicKey).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return role, err
}

// KeyAgent returns the agent a public key belongs to (active or revoked), or "".
func (s *Store) KeyAgent(ctx context.Context, publicKey string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT agent_id FROM agent_keys WHERE public_key = $1`, publicKey).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *Store) ListAgentKeys(ctx context.Context, agentID string) ([]AgentKey, error) {
	rows, err := s.Pool.Query(ctx, `SELECT public_key, role, created_at, revoked_at FROM agent_keys
		WHERE agent_id = $1 ORDER BY created_at, public_key`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []AgentKey{}
	for rows.Next() {
		var k AgentKey
		if err := rows.Scan(&k.PublicKey, &k.Role, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) AddAgentKey(ctx context.Context, agentID, publicKey, role string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO agent_keys (public_key, agent_id, role) VALUES ($1, $2, $3)`,
		publicKey, agentID, role)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrKeyInUse
	}
	return err
}

// Results of RevokeAgentKey.
var (
	ErrKeyNotFound = errors.New("key not found")
	ErrLastKey     = errors.New("last signing key")
)

// RevokeAgentKey revokes an active key. The agent's last active signing key
// may go only while an active recovery key remains or the agent has an owner,
// so nobody is locked out by accident. The check and the revocation run in
// one transaction with the agent's keys locked.
func (s *Store) RevokeAgentKey(ctx context.Context, agentID, publicKey string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var owned bool
	if err := tx.QueryRow(ctx, `SELECT user_id IS NOT NULL FROM agent_identities WHERE id = $1 FOR UPDATE`, agentID).Scan(&owned); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrKeyNotFound
		}
		return err
	}
	var role string
	err = tx.QueryRow(ctx, `SELECT role FROM agent_keys WHERE agent_id = $1 AND public_key = $2 AND revoked_at IS NULL`,
		agentID, publicKey).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrKeyNotFound
	} else if err != nil {
		return err
	}
	if role == KeySigning && !owned {
		var otherSigning, recovery int
		if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE role = 'signing' AND public_key <> $2),
				count(*) FILTER (WHERE role = 'recovery')
			FROM agent_keys WHERE agent_id = $1 AND revoked_at IS NULL`, agentID, publicKey).Scan(&otherSigning, &recovery); err != nil {
			return err
		}
		if otherSigning == 0 && recovery == 0 {
			return ErrLastKey
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE agent_keys SET revoked_at = NOW() WHERE public_key = $1`, publicKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
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

// ReserveOTPAttempt counts a verification attempt before the code is
// checked. It reports false once max attempts have been used, so concurrent
// guesses cannot get past the limit.
func (s *Store) ReserveOTPAttempt(ctx context.Context, userID uuid.UUID, max int) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE users SET otp_attempts = otp_attempts + 1
		WHERE id = $1 AND otp_code IS NOT NULL AND otp_attempts < $2`, userID, max)
	return tag.RowsAffected() > 0, err
}

// ConsumeUserOTP clears the code if it is still the given one, so each code
// signs in at most once.
func (s *Store) ConsumeUserOTP(ctx context.Context, userID uuid.UUID, otpHash string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE users SET otp_code = NULL, otp_expires_at = NULL, otp_attempts = 0
		WHERE id = $1 AND otp_code = $2`, userID, otpHash)
	return tag.RowsAffected() > 0, err
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

// --- API keys -------------------------------------------------------------------

type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func (s *Store) CreateAPIKey(ctx context.Context, id, tenantID uuid.UUID, agentID, name, prefix, hash string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO api_keys (id, tenant_id, agent_id, name, prefix, key_hash)
		VALUES ($1, $2, $3, $4, $5, $6)`, id, tenantID, agentID, name, prefix, hash)
	return err
}

// AgentForAPIKey returns the identity behind an active key and records use
// (at most once a minute, to avoid a write per request).
func (s *Store) AgentForAPIKey(ctx context.Context, hash string) (*domain.AgentIdentity, uuid.UUID, error) {
	var a domain.AgentIdentity
	var keyID uuid.UUID
	err := s.Pool.QueryRow(ctx, `SELECT k.id, i.id, i.user_id, i.tenant_id, i.created_at
		FROM api_keys k JOIN agent_identities i ON i.id = k.agent_id
		WHERE k.key_hash = $1 AND k.revoked_at IS NULL`, hash).
		Scan(&keyID, &a.ID, &a.UserID, &a.TenantID, &a.CreatedAt)
	if err != nil {
		id, err := nilIfNoRows(&a, err)
		return id, uuid.Nil, err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE api_keys SET last_used_at = NOW()
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < NOW() - INTERVAL '1 minute')`, keyID)
	return &a, keyID, nil
}

func (s *Store) ListAPIKeys(ctx context.Context, agentID string) ([]APIKey, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, name, prefix, created_at, last_used_at FROM api_keys
		WHERE agent_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []APIKey{}
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.CreatedAt, &k.LastUsedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// RevokeAPIKey revokes a key and, in the same statement, deletes the
// webhooks it registered.
func (s *Store) RevokeAPIKey(ctx context.Context, id uuid.UUID, agentID string) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `WITH revoked AS (
			UPDATE api_keys SET revoked_at = NOW()
			WHERE id = $1 AND agent_id = $2 AND revoked_at IS NULL RETURNING id
		), hooks_gone AS (
			DELETE FROM hooks WHERE api_key_id IN (SELECT id FROM revoked)
		)
		SELECT count(*) FROM revoked`, id, agentID).Scan(&n)
	return n > 0, err
}

// --- Device-code login ---------------------------------------------------------------

type DeviceCode struct {
	UserCode   string
	ClientName string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ApprovedAt *time.Time
	ConsumedAt *time.Time
	AgentID    *string
	TenantID   *uuid.UUID
}

func (s *Store) CreateDeviceCode(ctx context.Context, deviceHash, userCode, clientName string, expires time.Time) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM device_codes WHERE expires_at < NOW() - INTERVAL '1 day'`)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO device_codes (device_code_hash, user_code, client_name, expires_at)
		VALUES ($1, $2, $3, $4)`, deviceHash, userCode, clientName, expires)
	return err
}

func (s *Store) DeviceCodeByUserCode(ctx context.Context, userCode string) (*DeviceCode, error) {
	var d DeviceCode
	err := s.Pool.QueryRow(ctx, `SELECT user_code, client_name, created_at, expires_at, approved_at, consumed_at, agent_id, tenant_id
		FROM device_codes WHERE user_code = $1`, userCode).
		Scan(&d.UserCode, &d.ClientName, &d.CreatedAt, &d.ExpiresAt, &d.ApprovedAt, &d.ConsumedAt, &d.AgentID, &d.TenantID)
	return nilIfNoRows(&d, err)
}

// ApproveDeviceCode binds an unexpired, unapproved code to an agent.
func (s *Store) ApproveDeviceCode(ctx context.Context, userCode, agentID string, tenantID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE device_codes SET approved_at = NOW(), agent_id = $2, tenant_id = $3
		WHERE user_code = $1 AND approved_at IS NULL AND expires_at > NOW()`, userCode, agentID, tenantID)
	return tag.RowsAffected() > 0, err
}

// ConsumeDeviceCode atomically marks an approved code as used and returns it,
// so a device code can be exchanged for an API key exactly once.
func (s *Store) ConsumeDeviceCode(ctx context.Context, deviceHash string) (*DeviceCode, error) {
	var d DeviceCode
	err := s.Pool.QueryRow(ctx, `UPDATE device_codes SET consumed_at = NOW()
		WHERE device_code_hash = $1 AND approved_at IS NOT NULL AND consumed_at IS NULL AND expires_at > NOW()
		RETURNING user_code, client_name, expires_at, approved_at, consumed_at, agent_id, tenant_id`, deviceHash).
		Scan(&d.UserCode, &d.ClientName, &d.ExpiresAt, &d.ApprovedAt, &d.ConsumedAt, &d.AgentID, &d.TenantID)
	return nilIfNoRows(&d, err)
}

// DeviceCodeStatus reports why a device code cannot be exchanged yet.
func (s *Store) DeviceCodeStatus(ctx context.Context, deviceHash string) (*DeviceCode, error) {
	var d DeviceCode
	err := s.Pool.QueryRow(ctx, `SELECT user_code, client_name, expires_at, approved_at, consumed_at, agent_id, tenant_id
		FROM device_codes WHERE device_code_hash = $1`, deviceHash).
		Scan(&d.UserCode, &d.ClientName, &d.ExpiresAt, &d.ApprovedAt, &d.ConsumedAt, &d.AgentID, &d.TenantID)
	return nilIfNoRows(&d, err)
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

// DeleteMemory deletes a memory and returns the file it was made from, if any.
func (s *Store) DeleteMemory(ctx context.Context, id uuid.UUID, agentID string, tenantID uuid.UUID) (bool, *uuid.UUID, error) {
	var fileID *uuid.UUID
	err := s.Pool.QueryRow(ctx, `DELETE FROM memories WHERE id = $1 AND agent_id = $2 AND tenant_id = $3 RETURNING file_id`,
		id, agentID, tenantID).Scan(&fileID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	return err == nil, fileID, err
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

// FileBytes is the total size of an agent's stored files.
func (s *Store) FileBytes(ctx context.Context, agentID string) (int64, error) {
	var n int64
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(file_size), 0) FROM files WHERE agent_id = $1`, agentID).Scan(&n)
	return n, err
}

func (s *Store) InsertFile(ctx context.Context, f *File) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO files (id, agent_id, tenant_id, file_name, file_type, file_path, file_size, mime_type, extracted_text)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		f.ID, f.AgentID, f.TenantID, f.FileName, f.FileType, f.FilePath, f.FileSize, f.MimeType, f.ExtractedText)
	return err
}

// DeleteOrphanFile removes a file record no memory refers to any more and
// returns its path ("" if the file is still in use or already gone).
func (s *Store) DeleteOrphanFile(ctx context.Context, fileID uuid.UUID, agentID string) (string, error) {
	var path string
	err := s.Pool.QueryRow(ctx, `DELETE FROM files f WHERE f.id = $1 AND f.agent_id = $2
		AND NOT EXISTS (SELECT 1 FROM memories m WHERE m.file_id = f.id) RETURNING f.file_path`, fileID, agentID).Scan(&path)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return path, err
}

// DeleteFilesForAgent removes an agent's file records and returns their
// paths so the caller can delete them from storage.
func (s *Store) DeleteFilesForAgent(ctx context.Context, agentID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `DELETE FROM files WHERE agent_id = $1 RETURNING file_path`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

func (s *Store) GetFile(ctx context.Context, id uuid.UUID) (*File, error) {
	var f File
	err := s.Pool.QueryRow(ctx, `SELECT id, agent_id, tenant_id, file_name, file_type, file_path, file_size, mime_type, extracted_text
		FROM files WHERE id = $1`, id).
		Scan(&f.ID, &f.AgentID, &f.TenantID, &f.FileName, &f.FileType, &f.FilePath, &f.FileSize, &f.MimeType, &f.ExtractedText)
	return nilIfNoRows(&f, err)
}

func (s *Store) GetFileForAgent(ctx context.Context, id uuid.UUID, agentID string, tenantID uuid.UUID) (*File, error) {
	var f File
	err := s.Pool.QueryRow(ctx, `SELECT id, agent_id, tenant_id, file_name, file_type, file_path, file_size, mime_type, extracted_text
		FROM files WHERE id = $1 AND agent_id = $2 AND tenant_id = $3`, id, agentID, tenantID).
		Scan(&f.ID, &f.AgentID, &f.TenantID, &f.FileName, &f.FileType, &f.FilePath, &f.FileSize, &f.MimeType, &f.ExtractedText)
	return nilIfNoRows(&f, err)
}

// --- Tenants ----------------------------------------------------------------------

// IsPlatformAdmin reports whether a user is Memro staff (platform_admins).
func (s *Store) IsPlatformAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM platform_admins WHERE user_id = $1)`, userID).Scan(&ok)
	return ok, err
}

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

// Hooks belong to the agent that registered them and only fire for its own
// memories.
// RegisterHook adds a webhook. apiKeyID is the key that registered it (nil
// for a session or signature); revoking that key deletes the hook. Registering
// an existing hook with a session or signature makes it outlive the key.
func (s *Store) RegisterHook(ctx context.Context, tenantID uuid.UUID, agentID string, apiKeyID *uuid.UUID, eventType, url string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO hooks (tenant_id, agent_id, event_type, url, api_key_id) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (agent_id, event_type, url) DO UPDATE SET api_key_id = NULL WHERE EXCLUDED.api_key_id IS NULL`,
		tenantID, agentID, eventType, url, apiKeyID)
	return err
}

func (s *Store) DeleteHook(ctx context.Context, agentID, eventType, url string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM hooks WHERE agent_id = $1 AND event_type = $2 AND url = $3`, agentID, eventType, url)
	return tag.RowsAffected() > 0, err
}

func (s *Store) GetHooks(ctx context.Context, agentID, eventType string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT url FROM hooks WHERE agent_id = $1 AND event_type = $2 ORDER BY created_at`, agentID, eventType)
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
