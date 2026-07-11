package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mtzanidakis/dodo/internal/models"
)

type Users struct {
	db *sql.DB
}

func userColumns() string {
	return `id, email, password_hash, display_name, timezone, locale, theme,
        telegram_bot_token, telegram_allowed_user_ids, telegram_chat_id, telegram_chat_user_id,
        telegram_configured_at, created_at, updated_at, deleted_at`
}

func scanUser(row interface {
	Scan(...any) error
}, u *models.User) error {
	var (
		deletedAt    sql.NullString
		botToken     sql.NullString
		allowed      sql.NullString
		chatID       sql.NullString
		chatUserID   sql.NullString
		configuredAt sql.NullString
		createdAt    sql.NullString
		updatedAt    sql.NullString
	)
	err := row.Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Timezone, &u.Locale, &u.Theme,
		&botToken, &allowed, &chatID, &chatUserID, &configuredAt,
		&createdAt, &updatedAt, &deletedAt,
	)
	if err != nil {
		return err
	}
	if botToken.Valid {
		u.TelegramBotToken = botToken.String
	}
	u.TelegramAllowedIDs = allowed.String
	u.TelegramChatID = chatID.String
	u.TelegramChatUserID = chatUserID.String
	u.TelegramConfiguredAt = parseNullableTime(configuredAt)
	u.DeletedAt = parseNullableTime(deletedAt)
	u.CreatedAt = parseTime(createdAt)
	u.UpdatedAt = parseTime(updatedAt)
	return nil
}

func (s *Users) Create(ctx context.Context, u *models.User) error {
	u.ID = newUUID()
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	if u.Timezone == "" {
		u.Timezone = "Europe/Athens"
	}
	if u.Locale == "" {
		u.Locale = models.LocaleEn
	}
	if u.Theme == "" {
		u.Theme = models.ThemeSystem
	}

	var botToken, allowed, chatID, chatUserID, configuredAt, deletedAt any
	_, err := s.db.ExecContext(ctx, `INSERT INTO users
(id, email, password_hash, display_name, timezone, locale, theme,
 telegram_bot_token, telegram_allowed_user_ids, telegram_chat_id, telegram_chat_user_id,
 telegram_configured_at, created_at, updated_at, deleted_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.ID, u.Email, u.PasswordHash, u.DisplayName, u.Timezone, u.Locale, u.Theme,
		botToken, allowed, chatID, chatUserID, configuredAt, formatTime(u.CreatedAt), formatTime(u.UpdatedAt), deletedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: email %s already exists", models.ErrConflict, u.Email)
		}
		return err
	}
	return nil
}

func (s *Users) GetByID(ctx context.Context, id string) (*models.User, error) {
	u := &models.User{}
	err := scanUser(s.db.QueryRowContext(ctx,
		"SELECT "+userColumns()+" FROM users WHERE id = ?", id), u)
	if err != nil {
		return nil, NotFound(err)
	}
	return u, nil
}

func (s *Users) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	u := &models.User{}
	err := scanUser(s.db.QueryRowContext(ctx,
		"SELECT "+userColumns()+" FROM users WHERE email = ? AND deleted_at IS NULL", email), u)
	if err != nil {
		return nil, NotFound(err)
	}
	return u, nil
}

func (s *Users) GetByTelegramWebhookSecret(_ context.Context, _ string) (*models.User, error) {
	return nil, errors.New("webhooks not supported; use long polling")
}

func (s *Users) List(ctx context.Context) ([]*models.User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+userColumns()+" FROM users WHERE deleted_at IS NULL ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*models.User
	for rows.Next() {
		u := &models.User{}
		if err := scanUser(rows, u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Users) ListTelegramEnabled(ctx context.Context) ([]*models.User, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+userColumns()+" FROM users WHERE deleted_at IS NULL AND telegram_bot_token IS NOT NULL AND telegram_bot_token != ''")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*models.User
	for rows.Next() {
		u := &models.User{}
		if err := scanUser(rows, u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Users) Update(ctx context.Context, u *models.User) error {
	u.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE users SET
 email=?, display_name=?, timezone=?, locale=?, theme=?, updated_at=? WHERE id=? AND deleted_at IS NULL`,
		u.Email, u.DisplayName, u.Timezone, u.Locale, u.Theme, formatTime(u.UpdatedAt), u.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: email %s already exists", models.ErrConflict, u.Email)
		}
		return err
	}
	return nil
}

func (s *Users) UpdatePassword(ctx context.Context, userID, hash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash=?, updated_at=? WHERE id=? AND deleted_at IS NULL`,
		hash, formatTime(time.Now().UTC()), userID)
	if err != nil {
		return err
	}
	return nil
}

func (s *Users) SetTelegramConfig(ctx context.Context, userID, encryptedToken, allowedUserIDs string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET
 telegram_bot_token=?, telegram_allowed_user_ids=?, telegram_configured_at=?, updated_at=? WHERE id=? AND deleted_at IS NULL`,
		encryptedToken, allowedUserIDs, formatTime(time.Now().UTC()), formatTime(time.Now().UTC()), userID)
	return err
}

func (s *Users) GetTelegramConfig(ctx context.Context, userID string) (encryptedToken, allowedUserIDs string, chatID, chatUserID string, configuredAt *time.Time, err error) {
	var (
		botToken sql.NullString
		allowed  sql.NullString
		cid      sql.NullString
		cuid     sql.NullString
		confAt   sql.NullString
	)
	err = s.db.QueryRowContext(ctx, `SELECT telegram_bot_token, telegram_allowed_user_ids, telegram_chat_id, telegram_chat_user_id, telegram_configured_at FROM users WHERE id = ?`, userID).Scan(
		&botToken, &allowed, &cid, &cuid, &confAt)
	if err != nil {
		return "", "", "", "", nil, NotFound(err)
	}
	return botToken.String, allowed.String, cid.String, cuid.String, parseNullableTime(confAt), nil
}

func (s *Users) SetTelegramChatID(ctx context.Context, userID, chatID, chatUserID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET telegram_chat_id=?, telegram_chat_user_id=?, updated_at=? WHERE id=? AND deleted_at IS NULL`,
		chatID, chatUserID, formatTime(time.Now().UTC()), userID)
	return err
}

func (s *Users) ClearTelegramConfig(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET
 telegram_bot_token=NULL, telegram_allowed_user_ids=NULL, telegram_chat_id=NULL, telegram_chat_user_id=NULL,
 telegram_configured_at=NULL, updated_at=? WHERE id=? AND deleted_at IS NULL`,
		formatTime(time.Now().UTC()), userID)
	return err
}

func (s *Users) SoftDelete(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `UPDATE users SET deleted_at=?, updated_at=? WHERE id=? AND deleted_at IS NULL`,
		formatTime(now), formatTime(now), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.ErrNotFound
	}
	return nil
}
