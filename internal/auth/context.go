package auth

import (
	"context"

	"github.com/mtzanidakis/dodo/internal/models"
)

type contextKey string

const userKey contextKey = "user"

func WithUser(ctx context.Context, u *models.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func UserFromContext(ctx context.Context) *models.User {
	v, _ := ctx.Value(userKey).(*models.User)
	return v
}

const (
	SessionCookie = "dodo_session"
	CSRFCookie    = "dodo_csrf"
	SessionTTL    = 30 * 24 * 60 * 60
)
