package models

import "time"

type User struct {
	ID                   string
	Role                 string
	DisabledAt           *time.Time
	EmailVerifiedAt      *time.Time
	DefaultDue           string
	DefaultPriority      Priority
	DefaultRecurrence    string
	DefaultRecurrenceInterval int
	DailyDigestEnabled bool
	DailyDigestTime string
	DailyDigestLastSentDate string
	Email                string
	PasswordHash         string
	DisplayName          string
	Timezone             string
	Locale               Locale
	Theme                Theme
	DateFormat           string
	TelegramBotToken     string
	TelegramAllowedIDs   string
	TelegramChatID       string
	TelegramChatUserID   string
	TelegramConfiguredAt *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time
}

func (u *User) IsAdmin() bool { return u.Active() && u.Role == "admin" }
func (u *User) Active() bool { return u != nil && u.DisabledAt == nil && u.EmailVerifiedAt != nil && !u.Deleted() }

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
