package api

import (
	"context"
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

// ConfigureTelegram validates a bot token, stores it encrypted along with the
// allowed sender ids, and (re)starts the user's long-poller. It is shared by
// the JSON API handler and the browser (/account) form so both paths keep
// encryption and poller lifecycle in one place.
func (s *Server) ConfigureTelegram(ctx context.Context, userID, botToken, allowedUserIDs string) (string, error) {
	if botToken == "" {
		return "", errors.Join(models.ErrValidation, errors.New("bot_token required"))
	}
	username, err := s.telegram.ValidateToken(ctx, botToken)
	if err != nil {
		return "", errors.Join(models.ErrValidation, err)
	}
	enc, err := s.crypto.Encrypt(botToken)
	if err != nil {
		return "", err
	}
	if err := s.store.Users.SetTelegramConfig(ctx, userID, enc, strings.TrimSpace(allowedUserIDs)); err != nil {
		return "", err
	}
	_ = s.telegram.StopForUser(userID)
	if err := s.telegram.StartForUser(ctx, userID); err != nil {
		s.logger.Warn("telegram poller start failed", "user_id", userID, "error", err)
	}
	s.hub.Publish(userID, "telegram.updated", nil)
	return username, nil
}

// ClearTelegram stops the poller and removes all telegram config for the user.
func (s *Server) ClearTelegram(ctx context.Context, userID string) error {
	_ = s.telegram.StopForUser(userID)
	if err := s.store.Users.ClearTelegramConfig(ctx, userID); err != nil {
		return err
	}
	s.hub.Publish(userID, "telegram.updated", nil)
	return nil
}

// TestTelegram sends a test notification to the linked chat.
func (s *Server) TestTelegram(ctx context.Context, userID, chatID string) error {
	if chatID == "" {
		return errors.Join(models.ErrValidation, errors.New("no telegram chat linked"))
	}
	return s.telegram.SendTest(ctx, userID, chatID, "dodo test notification")
}

func (s *Server) handleSetTelegram(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var req setTelegramRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	username, err := s.ConfigureTelegram(r.Context(), u.ID, req.BotToken, req.AllowedUserIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	s.audit(r, "telegram.config.save", "user", u.ID, map[string]any{"bot_username": username})
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
	if err := s.ClearTelegram(r.Context(), u.ID); err != nil {
		writeError(w, err)
		return
	}
	s.audit(r, "telegram.config.clear", "user", u.ID, nil)
	writeJSON(w, http.StatusOK, telegramConfigDTO{Status: "not_configured"})
}

func (s *Server) handleTestTelegram(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	if err := s.TestTelegram(r.Context(), u.ID, u.TelegramChatID); err != nil {
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
