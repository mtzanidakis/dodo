package models

import "time"

type APIToken struct {
	ID          string
	UserID      string
	Name        string
	TokenPrefix string
	TokenHash   string
	LastUsedAt  *time.Time
	ExpiresAt   *time.Time
	CreatedAt   time.Time
	RevokedAt   *time.Time
}

func (t *APIToken) Revoked() bool {
	return t.RevokedAt != nil
}

func (t *APIToken) Expired(now time.Time) bool {
	return t.ExpiresAt != nil && t.ExpiresAt.Before(now)
}

type Session struct {
	ID        string
	UserID    string
	TokenHash string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (s *Session) Expired(now time.Time) bool {
	return s.ExpiresAt.Before(now)
}
