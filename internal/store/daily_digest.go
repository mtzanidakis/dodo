package store

import (
    "context"
    "fmt"
    "regexp"
    "time"

    "github.com/mtzanidakis/dodo/internal/models"
)

var dailyDigestClock = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)

func (s *Users) SetDailyDigest(ctx context.Context, userID string, enabled bool, clock string) error {
    if !dailyDigestClock.MatchString(clock) {
        return fmt.Errorf("%w: time must be HH:MM", models.ErrValidation)
    }
    flag := 0
    if enabled { flag = 1 }
    res, err := s.db.ExecContext(ctx, `UPDATE users SET
        daily_digest_enabled=?, daily_digest_time=?, updated_at=?
        WHERE id=? AND deleted_at IS NULL`, flag, clock, formatTime(time.Now().UTC()), userID)
    if err != nil { return err }
    affected,err:=res.RowsAffected()
    if err!=nil { return err }
    if affected==0 { return models.ErrNotFound }
    return nil
}

func (s *Users) ListDailyDigestEnabled(ctx context.Context) ([]*models.User, error) {
    rows,err:=s.db.QueryContext(ctx,"SELECT "+userColumns()+`
        FROM users WHERE deleted_at IS NULL AND disabled_at IS NULL
        AND email_verified_at IS NOT NULL AND daily_digest_enabled=1
        AND telegram_bot_token IS NOT NULL AND telegram_bot_token!=''
        AND telegram_chat_id IS NOT NULL AND telegram_chat_id!=''`)
    if err!=nil { return nil,err }
    defer rows.Close()
    var out []*models.User
    for rows.Next() {
        u:=&models.User{}
        if err:=scanUser(rows,u);err!=nil{return nil,err}
        out=append(out,u)
    }
    return out,rows.Err()
}

func (s *Users) MarkDailyDigestSent(ctx context.Context, userID, localDate string) error {
    if len(localDate)!=10 { return models.ErrValidation }
    res,err:=s.db.ExecContext(ctx,`UPDATE users SET daily_digest_last_sent_date=?
        WHERE id=? AND daily_digest_enabled=1 AND disabled_at IS NULL
        AND deleted_at IS NULL`,localDate,userID)
    if err!=nil{return err}
    n,err:=res.RowsAffected()
    if err!=nil{return err}
    if n==0{return models.ErrNotFound}
    return nil
}

func (s *Tasks) ListCandidatesForDigest(ctx context.Context, userID string, dayStart, dayEnd time.Time) ([]*models.Task,error) {
    rows,err:=s.db.QueryContext(ctx,`SELECT `+taskColumns()+`
        FROM tasks WHERE user_id=? AND completed_at IS NULL
        AND due_at < ? AND (recurrence_end_at IS NULL OR recurrence_end_at >= ?)
        ORDER BY due_at,id`,userID,formatTime(dayEnd),formatTime(dayStart))
    if err!=nil{return nil,err}
    defer rows.Close()
    var tasks []*models.Task
    for rows.Next() {
       t:=&models.Task{}
       if err:=scanTask(rows,t);err!=nil{return nil,err}
       tasks=append(tasks,t)
    }
    return tasks,rows.Err()
}

