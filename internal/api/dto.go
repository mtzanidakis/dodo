package api

import (
	"time"

	"github.com/mtzanidakis/dodo/internal/models"
)

type userDTO struct {
	ID                  string  `json:"id"`
	Email               string  `json:"email"`
	Role                string  `json:"role"`
	DisplayName         string  `json:"display_name"`
	Timezone            string  `json:"timezone"`
	Locale              string  `json:"locale"`
	Theme               string  `json:"theme"`
	TelegramConfigured  bool    `json:"telegram_configured"`
	TelegramLinked      bool    `json:"telegram_linked"`
	TelegramBotUsername *string `json:"telegram_bot_username,omitempty"`
	TelegramChatID      *string `json:"telegram_chat_id,omitempty"`
	CreatedAt           string  `json:"created_at"`
}

func toUserDTO(u *models.User) userDTO {
	return userDTO{
		ID:                 u.ID,
		Email:              u.Email,
		Role:               string(u.Role),
		DisplayName:        u.DisplayName,
		Timezone:           u.Timezone,
		Locale:             string(u.Locale),
		Theme:              string(u.Theme),
		TelegramConfigured: u.TelegramEnabled(),
		TelegramLinked:     u.TelegramLinked(),
		CreatedAt:          u.CreatedAt.UTC().Format(time.RFC3339),
	}
}

type taskDTO struct {
	ID                   string  `json:"id"`
	Title                string  `json:"title"`
	Description          string  `json:"description"`
	Priority             string  `json:"priority"`
	Kind                 string  `json:"kind"`
	DueAt                string  `json:"due_at"`
	DurationMinutes      int     `json:"duration_minutes"`
	CompletedAt          *string `json:"completed_at,omitempty"`
	RecurrenceFreq       *string `json:"recurrence_freq,omitempty"`
	RecurrenceInterval   int     `json:"recurrence_interval"`
	RecurrenceByDay      *string `json:"recurrence_by_day,omitempty"`
	RecurrenceByMonthDay *int    `json:"recurrence_by_month_day,omitempty"`
	RecurrenceEndAt      *string `json:"recurrence_end_at,omitempty"`
	SnoozedUntil         *string `json:"snoozed_until,omitempty"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

func toTaskDTO(t *models.Task) taskDTO {
	dto := taskDTO{
		ID:                   t.ID,
		Title:                t.Title,
		Description:          t.Description,
		Priority:             string(t.Priority),
		Kind:                 string(t.Kind),
		DueAt:                t.DueAt.UTC().Format(time.RFC3339),
		DurationMinutes:      t.DurationMinutes,
		RecurrenceInterval:   t.RecurrenceInterval,
		RecurrenceByDay:      nilStr(t.RecurrenceByDay),
		RecurrenceByMonthDay: t.RecurrenceByMonthDay,
		CreatedAt:            t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:            t.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if t.CompletedAt != nil {
		s := t.CompletedAt.UTC().Format(time.RFC3339)
		dto.CompletedAt = &s
	}
	if t.RecurrenceFreq != nil {
		s := string(*t.RecurrenceFreq)
		dto.RecurrenceFreq = &s
	}
	if t.RecurrenceEndAt != nil {
		s := t.RecurrenceEndAt.UTC().Format(time.RFC3339)
		dto.RecurrenceEndAt = &s
	}
	if t.SnoozedUntil != nil {
		s := t.SnoozedUntil.UTC().Format(time.RFC3339)
		dto.SnoozedUntil = &s
	}
	return dto
}

func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type tokenDTO struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	ExpiresAt  *string `json:"expires_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
	RevokedAt  *string `json:"revoked_at,omitempty"`
	Token      string  `json:"token,omitempty"`
}

func toTokenDTO(t *models.APIToken, fullToken string) tokenDTO {
	dto := tokenDTO{
		ID:        t.ID,
		Name:      t.Name,
		Prefix:    t.TokenPrefix,
		CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339),
		Token:     fullToken,
	}
	if t.LastUsedAt != nil {
		s := t.LastUsedAt.UTC().Format(time.RFC3339)
		dto.LastUsedAt = &s
	}
	if t.ExpiresAt != nil {
		s := t.ExpiresAt.UTC().Format(time.RFC3339)
		dto.ExpiresAt = &s
	}
	if t.RevokedAt != nil {
		s := t.RevokedAt.UTC().Format(time.RFC3339)
		dto.RevokedAt = &s
	}
	return dto
}

type completionDTO struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	Title       string `json:"title"`
	Priority    string `json:"priority"`
	DueAt       string `json:"due_at"`
	CompletedAt string `json:"completed_at"`
	CreatedAt   string `json:"created_at"`
}

func toCompletionDTO(c *models.TaskCompletion) completionDTO {
	return completionDTO{
		ID: c.ID, TaskID: c.TaskID, Title: c.Title, Priority: string(c.Priority),
		DueAt: c.DueAt.UTC().Format(time.RFC3339), CompletedAt: c.CompletedAt.UTC().Format(time.RFC3339),
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
	}
}

type listEnvelope[T any] struct {
	Items  []T     `json:"items"`
	Cursor *string `json:"cursor,omitempty"`
}
