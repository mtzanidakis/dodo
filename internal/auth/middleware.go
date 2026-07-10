package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
)

type Middleware struct {
	Store *store.Store
}

func (m *Middleware) AuthSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookie)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		user := m.resolveSession(r.Context(), cookie.Value)
		if user != nil {
			r = r.WithContext(WithUser(r.Context(), user))
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) resolveSession(ctx context.Context, cookieValue string) *models.User {
	hash := HashToken(cookieValue)
	ses, err := m.Store.Sessions.Lookup(ctx, hash)
	if err != nil || ses.Expired(time.Now()) {
		return nil
	}
	user, err := m.Store.Users.GetByID(ctx, ses.UserID)
	if err != nil || user.Deleted() {
		_ = m.Store.Sessions.Expire(ctx, ses.ID)
		return nil
	}
	return user
}

func (m *Middleware) AuthBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		user := m.resolveBearer(r.Context(), token)
		if user != nil {
			r = r.WithContext(WithUser(r.Context(), user))
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

func (m *Middleware) resolveBearer(ctx context.Context, token string) *models.User {
	hash := HashToken(token)
	t, err := m.Store.Tokens.LookupByHash(ctx, hash)
	if err != nil {
		return nil
	}
	if t.Revoked() || t.Expired(time.Now()) {
		return nil
	}
	user, err := m.Store.Users.GetByID(ctx, t.UserID)
	if err != nil || user.Deleted() {
		_ = invalidateToken(ctx, m.Store, t.ID)
		return nil
	}
	_ = m.Store.Tokens.Touch(ctx, t.ID)
	return user
}

func invalidateToken(ctx context.Context, s *store.Store, id string) error {
	return s.Tokens.Purge(ctx, id)
}

func (m *Middleware) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromContext(r.Context())
		if u == nil {
			handleUnauthorized(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromContext(r.Context())
		if u == nil || u.Role != models.RoleAdmin {
			handleUnauthorized(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleUnauthorized(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"authentication required"}}`))
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (m *Middleware) CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if bearerToken(r) != "" {
			next.ServeHTTP(w, r)
			return
		}
		sess, err := r.Cookie(SessionCookie)
		if err != nil || sess.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(CSRFCookie)
		if err != nil || cookie.Value == "" {
			csrfReject(w, r)
			return
		}
		header := r.Header.Get("X-CSRF-Token")
		if header == "" || !safeEqual(header, cookie.Value) {
			csrfReject(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func safeEqual(a, b string) bool {
	return strings.EqualFold(a, b) && len(a) == len(b)
}

func csrfReject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"invalid csrf token"}}`))
}

func IssueCSRF(w http.ResponseWriter) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	v := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookie,
		Value:    v,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	})
	return v
}

func (m *Middleware) VerifyPassword(u *models.User, current string) error {
	if !VerifyPassword(current, u.PasswordHash) {
		return errors.New("invalid current password")
	}
	return nil
}
