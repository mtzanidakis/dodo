package models

import "time"

type User struct {
	ID                   string
	Email                string
	PasswordHash         string
	DisplayName          string
	Timezone             string
	Locale               Locale
	Theme                Theme
	TelegramBotToken     string
	TelegramAllowedIDs   string
	TelegramChatID       string
	TelegramChatUserID   string
	TelegramConfiguredAt *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time
}

func (u *User) TelegramEnabled() bool {
	if u == nil {
		return false
	}
	return u.TelegramBotToken != ""
}

func (u *User) TelegramLinked() bool {
	return u.TelegramEnabled() && u.TelegramChatID != ""
}

func (u *User) Deleted() bool {
	return u != nil && u.DeletedAt != nil
}
