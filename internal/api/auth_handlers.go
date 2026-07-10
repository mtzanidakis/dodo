package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/models"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte("ok"))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		writeError(w, errors.Join(models.ErrValidation, errors.New("email and password required")))
		return
	}
	if !s.loginRL.Allow(r, req.Email) {
		s.audit(r, "login.locked_out", "user", "", map[string]any{"email": req.Email})
		writeJSON(w, http.StatusTooManyRequests, errorEnvelope{Error: apiError{Code: "rate_limited", Message: "too many failed login attempts"}})
		return
	}

	user, err := s.store.Users.GetByEmail(r.Context(), req.Email)
	if err != nil || !auth.VerifyPassword(req.Password, safeHash(user)) {
		s.loginRL.RecordFailure(r, req.Email)
		s.audit(r, "login.failed", "user", "", map[string]any{"email": req.Email})
		writeJSON(w, http.StatusUnauthorized, errorEnvelope{Error: apiError{Code: "unauthorized", Message: "invalid email or password"}})
		return
	}
	s.loginRL.Reset(r, req.Email)

	gen, err := auth.GenerateSession()
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.store.Sessions.Create(r.Context(), user.ID, gen.Hash, r.UserAgent(), 30*24*time.Hour); err != nil {
		writeError(w, err)
		return
	}
	auth.SetSessionCookie(w, auth.SessionCookieOptions{Value: gen.Full, Secure: auth.IsSecure(r), Duration: 30 * 24 * time.Hour})
	csrf := auth.IssueCSRF(w)
	s.audit(r, "login.success", "user", user.ID, map[string]any{"csrf_issued": csrf != ""})

	writeJSON(w, http.StatusOK, toUserDTO(user))
}

func safeHash(u *models.User) string {
	if u == nil {
		return ""
	}
	return u.PasswordHash
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil && c.Value != "" {
		hash := auth.HashToken(c.Value)
		if ses, err := s.store.Sessions.Lookup(r.Context(), hash); err == nil {
			_ = s.store.Sessions.Expire(r.Context(), ses.ID)
		}
	}
	auth.ClearSessionCookie(w, auth.IsSecure(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, toUserDTO(u))
}

func (s *Server) audit(r *http.Request, action, targetType, targetID string, meta map[string]any) {
	u := auth.UserFromContext(r.Context())
	uid := ""
	if u != nil {
		uid = u.ID
	}
	_ = s.store.Audit.Log(r.Context(), uid, action, targetType, targetID, meta)
}
