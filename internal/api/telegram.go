package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/models"
)

type setTelegramRequest struct {
	BotToken       string `json:"bot_token"`
	AllowedUserIDs string `json:"allowed_user_ids"`
}

type telegramConfigDTO struct {
	BotUsername    *string `json:"bot_username,omitempty"`
	AllowedUserIDs *string `json:"allowed_user_ids,omitempty"`
	ChatID         *string `json:"chat_id,omitempty"`
	ChatUserID     *string `json:"chat_user_id,omitempty"`
	ConfiguredAt   *string `json:"configured_at,omitempty"`
	Status         string  `json:"status"`
}

func (s *Server) handleGetTelegram(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	dto := telegramConfigDTO{Status: "not_configured"}
	if u.TelegramConfiguredAt != nil {
		c := u.TelegramConfiguredAt.UTC().Format(formatRFC3339())
		dto.ConfiguredAt = &c
		dto.Status = "configured"
	}
	if u.TelegramAllowedIDs != "" {
		allowed := u.TelegramAllowedIDs
		dto.AllowedUserIDs = &allowed
	}
	if u.TelegramChatID != "" {
		chatID := u.TelegramChatID
		dto.ChatID = &chatID
	}
	if u.TelegramChatUserID != "" {
		chatUserID := u.TelegramChatUserID
		dto.ChatUserID = &chatUserID
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) handleSetTelegram(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var req setTelegramRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.BotToken == "" {
		writeError(w, errors.Join(models.ErrValidation, errors.New("bot_token required")))
		return
	}
	username, err := s.telegram.ValidateToken(r.Context(), req.BotToken)
	if err != nil {
		writeError(w, errors.Join(models.ErrValidation, err))
		return
	}
	enc, err := s.crypto.Encrypt(req.BotToken)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.store.Users.SetTelegramConfig(r.Context(), u.ID, enc, strings.TrimSpace(req.AllowedUserIDs)); err != nil {
		writeError(w, err)
		return
	}
	_ = s.telegram.StopForUser(u.ID)
	if err := s.telegram.StartForUser(r.Context(), u.ID); err != nil {
		s.logger.Warn("telegram poller start failed", "user_id", u.ID, "error", err)
	}
	s.audit(r, "telegram.config.save", "user", u.ID, map[string]any{"bot_username": username})
	s.hub.Publish(u.ID, "telegram.updated", nil)
	writeJSON(w, http.StatusOK, telegramConfigDTO{BotUsername: &username, Status: "configured"})
}

type updateTelegramRequest struct {
	BotToken       *string `json:"bot_token"`
	AllowedUserIDs *string `json:"allowed_user_ids"`
}

func (s *Server) handleUpdateTelegram(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var req updateTelegramRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	var username string
	if req.BotToken != nil && *req.BotToken != "" {
		var err error
		username, err = s.telegram.ValidateToken(r.Context(), *req.BotToken)
		if err != nil {
			writeError(w, errors.Join(models.ErrValidation, err))
			return
		}
		enc, err := s.crypto.Encrypt(*req.BotToken)
		if err != nil {
			writeError(w, err)
			return
		}
		if err := s.store.Users.SetTelegramConfig(r.Context(), u.ID, enc, strings.TrimSpace(orValue(req.AllowedUserIDs))); err != nil {
			writeError(w, err)
			return
		}
		_ = s.telegram.StopForUser(u.ID)
		if err := s.telegram.StartForUser(r.Context(), u.ID); err != nil {
			s.logger.Warn("telegram poller restart failed", "user_id", u.ID, "error", err)
		}
	} else if req.AllowedUserIDs != nil {
		token, _ := currentBotToken(u)
		if token == "" {
			writeError(w, errors.Join(models.ErrValidation, errors.New("bot_token not configured")))
			return
		}
		if err := s.store.Users.SetTelegramConfig(r.Context(), u.ID, token, strings.TrimSpace(*req.AllowedUserIDs)); err != nil {
			writeError(w, err)
			return
		}
	}
	s.audit(r, "telegram.config.update", "user", u.ID, nil)
	s.hub.Publish(u.ID, "telegram.updated", nil)
	writeJSON(w, http.StatusOK, telegramConfigDTO{BotUsername: strPtrOrNil(username), Status: "configured"})
}

func (s *Server) handleDeleteTelegram(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	_ = s.telegram.StopForUser(u.ID)
	if err := s.store.Users.ClearTelegramConfig(r.Context(), u.ID); err != nil {
		writeError(w, err)
		return
	}
	s.audit(r, "telegram.config.clear", "user", u.ID, nil)
	s.hub.Publish(u.ID, "telegram.updated", nil)
	writeJSON(w, http.StatusOK, telegramConfigDTO{Status: "not_configured"})
}

func (s *Server) handleTestTelegram(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	if u.TelegramChatID == "" {
		writeError(w, errors.Join(models.ErrValidation, errors.New("no telegram chat linked")))
		return
	}
	if err := s.telegram.SendTest(r.Context(), u.ID, u.TelegramChatID, "dodo test notification"); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func orValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func currentBotToken(u *models.User) (string, error) {
	if !u.TelegramEnabled() {
		return "", errors.New("telegram not configured")
	}
	return u.TelegramBotToken, nil
}

func formatRFC3339() string { return "2006-01-02T15:04:05Z07:00" }
