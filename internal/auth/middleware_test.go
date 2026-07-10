package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
)

func newAuthStore(t *testing.T) (*store.Store, *models.User, string) {
	t.Helper()
	t.Parallel()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	s := store.New(d)
	ctx := context.Background()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := &models.User{Email: "user@example.com", PasswordHash: hash}
	if err := s.Users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return s, u, "password123"
}

func TestHashVerifyPassword(t *testing.T) {
	t.Parallel()
	hash, err := auth.HashPassword("hunter2xxx")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !auth.VerifyPassword("hunter2xxx", hash) {
		t.Fatalf("verify should succeed")
	}
	if auth.VerifyPassword("wrong", hash) {
		t.Fatalf("verify should fail for wrong")
	}
	if auth.VerifyPassword("hunter2xxx", "") {
		t.Fatalf("verify should fail for empty hash")
	}
}

func TestGenerateAPITokenUniquenessAndPrefix(t *testing.T) {
	t.Parallel()
	a, err := auth.GenerateAPIToken()
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	b, _ := auth.GenerateAPIToken()
	if a.Full == b.Full {
		t.Fatalf("tokens should differ")
	}
	if !strings.HasPrefix(a.Full, "dodo_") {
		t.Fatalf("token should start with dodo_, got %q", a.Full)
	}
	if len(a.Prefix) != 12 {
		t.Fatalf("prefix length should be 12, got %d", len(a.Prefix))
	}
	if a.Hash == "" || a.Hash == a.Full {
		t.Fatalf("hash mismatch")
	}
	if auth.HashToken(a.Full) != a.Hash {
		t.Fatalf("hash mismatch")
	}
}

func TestGenerateSession(t *testing.T) {
	t.Parallel()
	a, _ := auth.GenerateSession()
	b, _ := auth.GenerateSession()
	if a.Full == b.Full {
		t.Fatalf("sessions should differ")
	}
	if !strings.HasPrefix(a.Full, "dodo_") {
		t.Fatalf("session should start with dodo_")
	}
	if a.Hash == "" || auth.HashToken(a.Full) != a.Hash {
		t.Fatalf("session hash mismatch")
	}
}

func TestAuthBearerResolvesUserAndTouches(t *testing.T) {
	s, u, _ := newAuthStore(t)
	ctx := context.Background()
	gen, _ := auth.GenerateAPIToken()
	if _, err := s.Tokens.Create(ctx, u.ID, "agent", gen.Prefix, gen.Hash); err != nil {
		t.Fatalf("create token: %v", err)
	}

	mw := &auth.Middleware{Store: s}
	called := false
	handler := mw.AuthBearer(mw.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		got := auth.UserFromContext(r.Context())
		if got == nil || got.ID != u.ID {
			t.Fatalf("wrong user in context")
		}
	})))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+gen.Full)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Fatalf("handler not called")
	}

	tok, _ := s.Tokens.LookupByHash(ctx, gen.Hash)
	if tok.LastUsedAt == nil {
		t.Fatalf("last_used_at should be touched")
	}
}

func TestAuthBearerRejectsRevokedToken(t *testing.T) {
	s, u, _ := newAuthStore(t)
	ctx := context.Background()
	gen, _ := auth.GenerateAPIToken()
	tok, _ := s.Tokens.Create(ctx, u.ID, "agent", gen.Prefix, gen.Hash)
	_ = s.Tokens.Revoke(ctx, u.ID, tok.ID)

	mw := &auth.Middleware{Store: s}
	called := false
	handler := mw.AuthBearer(mw.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+gen.Full)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if called {
		t.Fatalf("revoked token should not reach handler")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthSessionResolvesAndRejectsSoftDeleted(t *testing.T) {
	s, u, _ := newAuthStore(t)
	ctx := context.Background()
	gen, _ := auth.GenerateSession()
	if _, err := s.Sessions.Create(ctx, u.ID, gen.Hash, "ua", 24*time.Hour); err != nil {
		t.Fatalf("create session: %v", err)
	}

	mw := &auth.Middleware{Store: s}
	called := false
	handler := mw.AuthSession(mw.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		got := auth.UserFromContext(r.Context())
		if got == nil || got.ID != u.ID {
			t.Fatalf("wrong user")
		}
	})))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: gen.Full})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Fatalf("session should resolve user")
	}
}

func TestAuthSoftDeletedUserInvalidatesSession(t *testing.T) {
	s, u, _ := newAuthStore(t)
	ctx := context.Background()
	gen, _ := auth.GenerateSession()
	if _, err := s.Sessions.Create(ctx, u.ID, gen.Hash, "ua", 24*time.Hour); err != nil {
		t.Fatalf("create session: %v", err)
	}
	_ = s.Users.SoftDelete(ctx, u.ID)

	mw := &auth.Middleware{Store: s}
	called := false
	handler := mw.AuthSession(mw.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: gen.Full})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if called {
		t.Fatalf("soft-deleted user should be rejected")
	}
	if _, err := s.Sessions.Lookup(ctx, gen.Hash); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("session should be expired, got %v", err)
	}
}

func TestCSRFRejectsSessionPostWithoutHeader(t *testing.T) {
	s, _, _ := newAuthStore(t)
	mw := &auth.Middleware{Store: s}
	handler := mw.CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("should not reach handler")
	}))
	req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookie, Value: "abc"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCSRFAllowsBearerPost(t *testing.T) {
	s, _, _ := newAuthStore(t)
	mw := &auth.Middleware{Store: s}
	called := false
	handler := mw.CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	req.Header.Set("Authorization", "Bearer dodo_x")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Fatalf("bearer request should bypass csrf")
	}
}

func TestLoginRateLimiterLockout(t *testing.T) {
	t.Parallel()
	l := auth.NewLoginRateLimiter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	email := "x@y.com"
	for i := 0; i < 10; i++ {
		l.RecordFailure(req, email)
	}
	if l.Allow(req, email) {
		t.Fatalf("should be locked out after 10 failures")
	}
	if !l.Allow(req, "other@y.com") {
		t.Fatalf("other email should be allowed")
	}
}
