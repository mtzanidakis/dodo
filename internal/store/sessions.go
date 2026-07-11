package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/mtzanidakis/dodo/internal/models"
)

type Sessions struct {
	db *sql.DB
}

func (s *Sessions) Create(ctx context.Context, userID, tokenHash, userAgent string, ttl time.Duration) (*models.Session, error) {
	ses := &models.Session{
		ID:        newUUID(),
		UserID:    userID,
		TokenHash: tokenHash,
		UserAgent: userAgent,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(ttl),
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (id, user_id, token_hash, user_agent, created_at, expires_at)
 VALUES (?,?,?,?,?,?)`,
		ses.ID, ses.UserID, ses.TokenHash, ses.UserAgent, formatTime(ses.CreatedAt), formatTime(ses.ExpiresAt))
	if err != nil {
		return nil, err
	}
	return ses, nil
}

func (s *Sessions) Lookup(ctx context.Context, tokenHash string) (*models.Session, error) {
	ses := &models.Session{}
	var (
		ua      sql.NullString
		created sql.NullString
		expires sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, user_agent, created_at, expires_at FROM sessions WHERE token_hash = ?`,
		tokenHash).Scan(&ses.ID, &ses.UserID, &ses.TokenHash, &ua, &created, &expires)
	if err != nil {
		return nil, NotFound(err)
	}
	ses.UserAgent = ua.String
	ses.CreatedAt = parseTime(created)
	ses.ExpiresAt = parseTime(expires)
	return ses, nil
}

func (s *Sessions) Expire(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Sessions) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now.UTC())
	return err
}

// DeleteByUser removes every session belonging to a user. Used to invalidate
// all outstanding cookies after a password change.
func (s *Sessions) DeleteByUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}
