package store

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "time"

    "github.com/mtzanidakis/dodo/internal/models"
)

func (s *Users) CreatePending(ctx context.Context, u *models.User, tokenHash string) error {
    if tokenHash == "" { return models.ErrValidation }
    u.ID = newUUID()
    now := time.Now().UTC()
    u.CreatedAt, u.UpdatedAt = now, now
    if u.Timezone == "" { u.Timezone = "Europe/Athens" }
    if u.Locale == "" { u.Locale = models.LocaleEn }
    if u.Theme == "" { u.Theme = models.ThemeSystem }
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil { return err }
    defer func(){ _ = tx.Rollback() }()
    _, err = tx.ExecContext(ctx, `INSERT INTO users
        (id,email,password_hash,display_name,timezone,locale,theme,date_format,created_at,updated_at)
        VALUES (?,?,?,?,?,?,?,?,?,?)`,
        u.ID,u.Email,u.PasswordHash,u.DisplayName,u.Timezone,u.Locale,u.Theme,"",
        formatTime(now),formatTime(now))
    if err != nil {
        if isUniqueViolation(err) { return models.ErrConflict }
        return err
    }
    _, err = tx.ExecContext(ctx, `INSERT INTO email_verification_tokens
       (id,user_id,token_hash,created_at,expires_at) VALUES (?,?,?,?,?)`,
       newUUID(),u.ID,tokenHash,formatTime(now),formatTime(now.Add(24*time.Hour)))
    if err != nil { return err }
    return tx.Commit()
}

func (s *Users) VerifyEmail(ctx context.Context, tokenHash string) error {
    tx, err := s.db.BeginTx(ctx,nil)
    if err != nil { return err }
    defer func(){ _ = tx.Rollback() }()
    var id string
    err = tx.QueryRowContext(ctx, `SELECT user_id FROM email_verification_tokens
        WHERE token_hash=? AND consumed_at IS NULL AND expires_at>?`,
        tokenHash,formatTime(time.Now().UTC())).Scan(&id)
    if errors.Is(err,sql.ErrNoRows) { return models.ErrNotFound }
    if err != nil { return err }
    now := formatTime(time.Now().UTC())
    res,err := tx.ExecContext(ctx,`UPDATE email_verification_tokens SET consumed_at=?
        WHERE token_hash=? AND consumed_at IS NULL`,now,tokenHash)
    if err != nil { return err }
    count,err := res.RowsAffected()
    if err != nil || count != 1 { return models.ErrConflict }
    verifiedRes,err := tx.ExecContext(ctx,`UPDATE users SET email_verified_at=?,updated_at=?
        WHERE id=? AND deleted_at IS NULL AND disabled_at IS NULL`,now,now,id)
    if err != nil { return err }
    verifiedCount,err := verifiedRes.RowsAffected()
    if err != nil || verifiedCount != 1 { return models.ErrNotFound }
    _,err = tx.ExecContext(ctx,`UPDATE users SET role='admin' WHERE id=?
        AND disabled_at IS NULL AND deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM users WHERE role='admin'
        AND deleted_at IS NULL AND disabled_at IS NULL AND email_verified_at IS NOT NULL)`,id)
    if err != nil { return err }
    return tx.Commit()
}

func (s *Users) SetDefaults(ctx context.Context, id,due string, priority models.Priority, freq string, interval int) error {
    switch due { case "now","24h","week","month": default: return models.ErrValidation }
    if !priority.Valid() { return models.ErrValidation }
    switch freq { case "once","daily","weekly","monthly","yearly": default: return models.ErrValidation }
    if interval<1 || interval>365 { return models.ErrValidation }
    res,err := s.db.ExecContext(ctx,`UPDATE users SET default_due=?,default_priority=?,
        default_recurrence=?,default_recurrence_interval=?,updated_at=?
        WHERE id=? AND deleted_at IS NULL`,due,priority,freq,interval,formatTime(time.Now().UTC()),id)
    if err != nil { return err }
    count,_ := res.RowsAffected()
    if count == 0 { return models.ErrNotFound }
    return nil
}

func (s *Users) SetRole(ctx context.Context, id, role string) error {
    if role != "user" && role != "admin" { return models.ErrValidation }
    return s.adminMutation(ctx,id,`UPDATE users SET role=?,updated_at=? WHERE id=? AND deleted_at IS NULL`,role)
}

func (s *Users) SetDisabled(ctx context.Context,id string, disabled bool) error {
    if disabled {
        return s.adminMutation(ctx,id,`UPDATE users SET disabled_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL`,formatTime(time.Now().UTC()))
    }
    return s.adminMutation(ctx,id,`UPDATE users SET disabled_at=NULL,updated_at=? WHERE id=? AND deleted_at IS NULL`)
}

func (s *Users) DeletePermanently(ctx context.Context,id string) error {
    return s.adminMutation(ctx,id,`DELETE FROM users WHERE id=?`)
}

func (s *Users) adminMutation(ctx context.Context,id,query string,args ...any) error {
    tx,err := s.db.BeginTx(ctx,nil)
    if err != nil { return err }
    defer func(){ _ = tx.Rollback() }()
    var role string
    var disabled,deleted,verified sql.NullString
    err = tx.QueryRowContext(ctx,`SELECT role,disabled_at,deleted_at,email_verified_at FROM users WHERE id=?`,id).
        Scan(&role,&disabled,&deleted,&verified)
    if errors.Is(err,sql.ErrNoRows) { return models.ErrNotFound }
    if err != nil { return err }
    if role=="admin" && !disabled.Valid && !deleted.Valid && verified.Valid {
        var count int
        err = tx.QueryRowContext(ctx,`SELECT COUNT(*) FROM users WHERE role='admin'
            AND id!=? AND disabled_at IS NULL AND deleted_at IS NULL
            AND email_verified_at IS NOT NULL`,id).Scan(&count)
        if err != nil { return err }
        removing := query==`DELETE FROM users WHERE id=?` ||
            query==`UPDATE users SET disabled_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL` ||
            (query==`UPDATE users SET role=?,updated_at=? WHERE id=? AND deleted_at IS NULL` && args[0]=="user")
        if removing && count==0 { return fmt.Errorf("%w: last active administrator",models.ErrConflict) }
    }
    var values []any
    if query==`DELETE FROM users WHERE id=?` { values=[]any{id} } else {
        values=append(values,args...)
        values=append(values,formatTime(time.Now().UTC()),id)
    }
    res,err:=tx.ExecContext(ctx,query,values...)
    if err!=nil { return err }
    n,_:=res.RowsAffected()
    if n==0 { return models.ErrNotFound }
    return tx.Commit()
}
