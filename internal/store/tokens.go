package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/mtzanidakis/dodo/internal/models"
)

type Tokens struct {
	db *sql.DB
}

func tokenColumns() string {
	return `id, user_id, name, token_prefix, token_hash, last_used_at, expires_at, created_at, revoked_at`
}

func scanToken(row interface {
	Scan(...any) error
}, t *models.APIToken) error {
	var (
		lastUsed  sql.NullString
		expires   sql.NullString
		revoked   sql.NullString
		createdAt sql.NullString
	)
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenPrefix, &t.TokenHash,
		&lastUsed, &expires, &createdAt, &revoked)
	if err != nil {
		return err
	}
	t.LastUsedAt = parseNullableTime(lastUsed)
	t.ExpiresAt = parseNullableTime(expires)
	t.RevokedAt = parseNullableTime(revoked)
	t.CreatedAt = parseTime(createdAt)
	return nil
}

func (s *Tokens) Create(ctx context.Context, userID, name, prefix, hash string) (*models.APIToken, error) {
	t := &models.APIToken{
		ID:          newUUID(),
		UserID:      userID,
		Name:        name,
		TokenPrefix: prefix,
		TokenHash:   hash,
		CreatedAt:   time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO api_tokens (id, user_id, name, token_prefix, token_hash, created_at)
 VALUES (?,?,?,?,?,?)`,
		t.ID, t.UserID, t.Name, t.TokenPrefix, t.TokenHash, formatTime(t.CreatedAt))
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Tokens) LookupByHash(ctx context.Context, hash string) (*models.APIToken, error) {
	t := &models.APIToken{}
	err := scanToken(s.db.QueryRowContext(ctx,
		"SELECT "+tokenColumns()+" FROM api_tokens WHERE token_hash = ?", hash), t)
	if err != nil {
		return nil, NotFound(err)
	}
	return t, nil
}

func (s *Tokens) List(ctx context.Context, userID string) ([]*models.APIToken, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+tokenColumns()+" FROM api_tokens WHERE user_id = ? ORDER BY created_at", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*models.APIToken
	for rows.Next() {
		t := &models.APIToken{}
		if err := scanToken(rows, t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetByPrefix looks up an active (non-revoked) token by its public prefix
// across all users. Used by the admin CLI's `token revoke --prefix`.
func (s *Tokens) GetByPrefix(ctx context.Context, prefix string) (*models.APIToken, error) {
	t := &models.APIToken{}
	err := scanToken(s.db.QueryRowContext(ctx,
		"SELECT "+tokenColumns()+" FROM api_tokens WHERE token_prefix = ? AND revoked_at IS NULL", prefix), t)
	if err != nil {
		return nil, NotFound(err)
	}
	return t, nil
}

func (s *Tokens) Get(ctx context.Context, userID, id string) (*models.APIToken, error) {
	t := &models.APIToken{}
	err := scanToken(s.db.QueryRowContext(ctx,
		"SELECT "+tokenColumns()+" FROM api_tokens WHERE id = ? AND user_id = ?", id, userID), t)
	if err != nil {
		return nil, NotFound(err)
	}
	return t, nil
}

func (s *Tokens) Revoke(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE api_tokens SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at IS NULL`,
		formatTime(time.Now().UTC()), id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.ErrNotFound
	}
	return nil
}

func (s *Tokens) Touch(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at=? WHERE id=?`,
		formatTime(time.Now().UTC()), id)
	return err
}

func (s *Tokens) Purge(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	return err
}
