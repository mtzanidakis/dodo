package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/models"
)

type updateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	Timezone    *string `json:"timezone"`
	Locale      *string `json:"locale"`
	Theme       *string `json:"theme"`
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, toUserDTO(u))
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var req updateProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.DisplayName != nil {
		u.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.Timezone != nil {
		if _, err := loadTZ(*req.Timezone); err != nil {
			writeError(w, errors.Join(models.ErrValidation, err))
			return
		}
		u.Timezone = *req.Timezone
	}
	if req.Locale != nil {
		l := models.Locale(*req.Locale)
		if !l.Valid() {
			writeError(w, errors.Join(models.ErrValidation, errors.New("invalid locale")))
			return
		}
		u.Locale = l
	}
	if req.Theme != nil {
		th := models.Theme(*req.Theme)
		if !th.Valid() {
			writeError(w, errors.Join(models.ErrValidation, errors.New("invalid theme")))
			return
		}
		u.Theme = th
	}
	if err := s.store.Users.Update(r.Context(), u); err != nil {
		writeError(w, err)
		return
	}
	s.hub.Publish(u.ID, "profile.updated", toUserDTO(u))
	s.audit(r, "profile.update", "user", u.ID, nil)
	writeJSON(w, http.StatusOK, toUserDTO(u))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var req changePasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, errors.Join(models.ErrValidation, errors.New("password must be at least 8 characters")))
		return
	}
	if !auth.VerifyPassword(req.CurrentPassword, u.PasswordHash) {
		writeError(w, errors.Join(models.ErrUnauthorized, errors.New("current password incorrect")))
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.store.Users.UpdatePassword(r.Context(), u.ID, hash); err != nil {
		writeError(w, err)
		return
	}
	// Invalidate every outstanding session so a stolen cookie can't survive
	// the password change (the caller re-authenticates on the next request).
	_ = s.store.Sessions.DeleteByUser(r.Context(), u.ID)
	s.audit(r, "password.change", "user", u.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
