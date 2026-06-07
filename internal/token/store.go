package token

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// Store handles database operations for tokens
type Store struct {
	db *sql.DB
}

// NewStore creates a new token store
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate creates a tokens table if it doesn't exist
func (s *Store) Migrate() error {
	// ✅ 优雅处理 pgcrypto 扩展的权限问题
	_, err := s.db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto;")
	if err != nil {
		if !strings.Contains(err.Error(), "permission denied") {
			return fmt.Errorf("failed to create pgcrypto extension: %w", err)
		}
		log.Printf("⚠️ WARNING: unable to create pgcrypto extension (permission denied), assuming gen_random_uuid() is available.")
	}

	query := `
		CREATE TABLE IF NOT EXISTS tokens (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			token VARCHAR(64) UNIQUE NOT NULL,
			tag VARCHAR(128),
			tenant_id UUID,
			max_uses INTEGER NOT NULL DEFAULT 1,
			used_count INTEGER NOT NULL DEFAULT 0,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			created_by VARCHAR(64),
			status VARCHAR(16) DEFAULT 'active',
			last_used_at TIMESTAMPTZ,
			last_used_by VARCHAR(64)
		);

		CREATE INDEX IF NOT EXISTS idx_tokens_token ON tokens(token);
		CREATE INDEX IF NOT EXISTS idx_tokens_status ON tokens(status);
		CREATE INDEX IF NOT EXISTS idx_tokens_tenant_id ON tokens(tenant_id);
	`
	_, err = s.db.Exec(query)
	return err
}

// Create inserts a new token into the database
func (s *Store) Create(t *Token) error {
	var query string
	var args []interface{}

	if t.TenantID == "" {
		query = `INSERT INTO tokens (token, tag, tenant_id, max_uses, used_count, expires_at, created_by, status)
		         VALUES ($1, $2, NULL, $3, $4, $5, $6, $7)
		         RETURNING id`
		args = []interface{}{t.Token, t.Tag, t.MaxUses, t.UsedCount, t.ExpiresAt, t.CreatedBy, t.Status}
	} else {
		query = `INSERT INTO tokens (token, tag, tenant_id, max_uses, used_count, expires_at, created_by, status)
		         VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		         RETURNING id`
		args = []interface{}{t.Token, t.Tag, t.TenantID, t.MaxUses, t.UsedCount, t.ExpiresAt, t.CreatedBy, t.Status}
	}

	return s.db.QueryRow(query, args...).Scan(&t.ID)
}

// GetByToken retrieves a token by its token string
func (s *Store) GetByToken(tokenStr string) (*Token, error) {
	query := `
		SELECT id, token, tag, COALESCE(tenant_id::text, ''), max_uses, used_count, expires_at, created_at,
		       COALESCE(created_by::text, ''), status, last_used_at, COALESCE(last_used_by::text, '')
		FROM tokens
		WHERE token = $1
	`
	row := s.db.QueryRow(query, tokenStr)
	return s.scanToken(row)
}

// GetByID retrieves a token by its ID
func (s *Store) GetByID(id string) (*Token, error) {
	if id == "" {
		return nil, fmt.Errorf("token id cannot be empty")
	}

	query := `
		SELECT id, token, tag, COALESCE(tenant_id::text, ''), max_uses, used_count, expires_at, created_at,
		       COALESCE(created_by::text, ''), status, last_used_at, COALESCE(last_used_by::text, '')
		FROM tokens
		WHERE id = $1
	`
	row := s.db.QueryRow(query, id)
	return s.scanToken(row)
}

// List retrieves all tokens, optionally filtered by status
func (s *Store) List(status Status) ([]*Token, error) {
	var query string
	var rows *sql.Rows
	var err error

	if status == "" || status == "all" {
		query = `
			SELECT id, token, tag, COALESCE(tenant_id::text, ''), max_uses, used_count, expires_at, created_at,
			       COALESCE(created_by::text, ''), status, last_used_at, COALESCE(last_used_by::text, '')
			FROM tokens
			ORDER BY created_at DESC
		`
		rows, err = s.db.Query(query)
	} else {
		query = `
			SELECT id, token, tag, COALESCE(tenant_id::text, ''), max_uses, used_count, expires_at, created_at,
			       COALESCE(created_by::text, ''), status, last_used_at, COALESCE(last_used_by::text, '')
			FROM tokens
			WHERE status = $1
			ORDER BY created_at DESC
		`
		rows, err = s.db.Query(query, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*Token
	for rows.Next() {
		t, err := s.scanTokenRows(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}

	return tokens, rows.Err()
}

// IncrementUsage increases usage count and records a device
func (s *Store) IncrementUsage(tokenStr, deviceID string) error {
	query := `
		UPDATE tokens
		SET used_count = used_count + 1,
		    last_used_at = NOW(),
		    last_used_by = $2,
		    status = CASE
		        WHEN max_uses > 0 AND used_count + 1 >= max_uses THEN 'exhausted'
		        ELSE status
		    END
		WHERE token = $1 AND status = 'active' AND (max_uses = 0 OR used_count < max_uses)
	`
	result, err := s.db.Exec(query, tokenStr, deviceID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		// ✅ 严谨的二次查询与错误透传
		t, err := s.GetByToken(tokenStr)
		if err != nil {
			return fmt.Errorf("failed to re-fetch token after update: %w", err)
		}
		if t == nil {
			return ErrTokenNotFound
		}

		switch t.Status {
		case StatusExpired:
			return ErrTokenExpired
		case StatusExhausted:
			return ErrTokenExhausted
		case StatusRevoked:
			return ErrTokenRevoked
		default:
			return fmt.Errorf("token not active (current status: %s)", t.Status)
		}
	}
	return nil
}

// UpdateStatus updates a status of a token
func (s *Store) UpdateStatus(id string, status Status) error {
	if id == "" {
		return fmt.Errorf("token id cannot be empty")
	}
	query := `UPDATE tokens SET status = $2 WHERE id = $1`
	_, err := s.db.Exec(query, id, status)
	return err
}

// Revoke revokes a token by its token string
func (s *Store) Revoke(tokenStr string) error {
	query := `UPDATE tokens SET status = 'revoked' WHERE token = $1`
	result, err := s.db.Exec(query, tokenStr)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrTokenNotFound // ✅ 统一使用业务错误
	}
	return nil
}

// RevokeByID revokes a token by its ID
func (s *Store) RevokeByID(id string) error {
	if id == "" {
		return fmt.Errorf("token id cannot be empty")
	}
	query := `UPDATE tokens SET status = 'revoked' WHERE id = $1`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrTokenNotFound // ✅ 统一使用业务错误
	}
	return nil
}

// Delete removes a token from the database
func (s *Store) Delete(tokenStr string) error {
	query := `DELETE FROM tokens WHERE token = $1`
	_, err := s.db.Exec(query, tokenStr)
	return err
}

// CleanupExpired updates the status of expired tokens
func (s *Store) CleanupExpired() (int, error) {
	query := `
		UPDATE tokens
		SET status = 'expired'
		WHERE status = 'active' AND expires_at < NOW()
	`
	result, err := s.db.Exec(query)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// scanDestinations DRY refactoring
func scanDestinations(t *Token, tag *sql.NullString, tenantID *sql.NullString, lastUsedAt *sql.NullTime) []any {
	return []any{
		&t.ID, &t.Token, tag, tenantID, &t.MaxUses,
		&t.UsedCount, &t.ExpiresAt, &t.CreatedAt,
		&t.CreatedBy, &t.Status, lastUsedAt, &t.LastUsedBy,
	}
}

// scanToken scans a row into a Token struct
func (s *Store) scanToken(row *sql.Row) (*Token, error) {
	t := &Token{}
	var tag sql.NullString
	var tenantID sql.NullString
	var lastUsedAt sql.NullTime

	err := row.Scan(scanDestinations(t, &tag, &tenantID, &lastUsedAt)...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if tag.Valid {
		t.Tag = tag.String
	}
	if tenantID.Valid {
		t.TenantID = tenantID.String
	}
	if lastUsedAt.Valid {
		t.LastUsedAt = lastUsedAt.Time
	}
	return t, nil
}

// scanTokenRows scans rows into a Token slice
func (s *Store) scanTokenRows(rows *sql.Rows) (*Token, error) {
	t := &Token{}
	var tag sql.NullString
	var tenantID sql.NullString
	var lastUsedAt sql.NullTime

	err := rows.Scan(scanDestinations(t, &tag, &tenantID, &lastUsedAt)...)
	if err != nil {
		return nil, err
	}
	if tag.Valid {
		t.Tag = tag.String
	}
	if tenantID.Valid {
		t.TenantID = tenantID.String
	}
	if lastUsedAt.Valid {
		t.LastUsedAt = lastUsedAt.Time
	}
	return t, nil
}
