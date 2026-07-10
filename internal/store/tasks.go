package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/models"
)

type Tasks struct {
	db *sql.DB
}

func taskColumns() string {
	return `id, user_id, title, description, priority, kind, due_at, duration_minutes, completed_at,
        recurrence_freq, recurrence_interval, recurrence_by_day, recurrence_by_month_day,
        recurrence_end_at, last_notified_at, snoozed_until, created_at, updated_at`
}

func scanTask(row interface {
	Scan(...any) error
}, t *models.Task) error {
	var (
		dueAt        sql.NullString
		completedAt  sql.NullString
		createdAt    sql.NullString
		updatedAt    sql.NullString
		recFreq      sql.NullString
		recInterval  sql.NullInt64
		recByDay     sql.NullString
		recByMonth   sql.NullInt64
		recEnd       sql.NullString
		lastNotified sql.NullString
		snoozed      sql.NullString
	)
	err := row.Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Priority, &t.Kind,
		&dueAt, &t.DurationMinutes, &completedAt,
		&recFreq, &recInterval, &recByDay, &recByMonth, &recEnd, &lastNotified, &snoozed,
		&createdAt, &updatedAt)
	if err != nil {
		return err
	}
	t.DueAt = parseTime(dueAt)
	t.CompletedAt = parseNullableTime(completedAt)
	t.CreatedAt = parseTime(createdAt)
	t.UpdatedAt = parseTime(updatedAt)
	t.RecurrenceInterval = 1
	if recFreq.Valid && recFreq.String != "" {
		f := models.RecurrenceFreq(recFreq.String)
		t.RecurrenceFreq = &f
	}
	if recInterval.Valid {
		t.RecurrenceInterval = int(recInterval.Int64)
	}
	t.RecurrenceByDay = recByDay.String
	t.RecurrenceByMonthDay = parseIntPtr(recByMonth)
	t.RecurrenceEndAt = parseNullableTime(recEnd)
	t.LastNotifiedAt = parseNullableTime(lastNotified)
	t.SnoozedUntil = parseNullableTime(snoozed)
	return nil
}

func ptrTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func nullableFreq(f *models.RecurrenceFreq) any {
	if f == nil {
		return nil
	}
	return string(*f)
}

func freqByMonthVal(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func (s *Tasks) Create(ctx context.Context, t *models.Task) error {
	t.ID = newUUID()
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Priority == "" {
		t.Priority = models.PriorityNormal
	}
	if t.Kind == "" {
		if t.Recurring() {
			t.Kind = models.KindRecurring
		} else {
			t.Kind = models.KindOneoff
		}
	}
	if t.RecurrenceInterval < 1 {
		t.RecurrenceInterval = 1
	}

	var recFreq any
	if t.RecurrenceFreq != nil {
		recFreq = string(*t.RecurrenceFreq)
	}
	var recByMonth any
	if t.RecurrenceByMonthDay != nil {
		recByMonth = *t.RecurrenceByMonthDay
	}

	var snoozed any
	if t.SnoozedUntil != nil {
		snoozed = formatTime(*t.SnoozedUntil)
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO tasks
(id, user_id, title, description, priority, kind, due_at, duration_minutes, completed_at,
 recurrence_freq, recurrence_interval, recurrence_by_day, recurrence_by_month_day,
 recurrence_end_at, last_notified_at, snoozed_until, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.UserID, t.Title, t.Description, t.Priority, t.Kind, formatTime(t.DueAt), t.DurationMinutes, nil,
		recFreq, t.RecurrenceInterval, nullableStr(t.RecurrenceByDay), recByMonth,
		formatTime(ptrTime(t.RecurrenceEndAt)), nil, snoozed, formatTime(t.CreatedAt), formatTime(t.UpdatedAt))
	if err != nil {
		return err
	}
	return nil
}

func (s *Tasks) Get(ctx context.Context, userID, id string) (*models.Task, error) {
	t := &models.Task{}
	err := scanTask(s.db.QueryRowContext(ctx,
		"SELECT "+taskColumns()+" FROM tasks WHERE id = ? AND user_id = ?", id, userID), t)
	if err != nil {
		return nil, NotFound(err)
	}
	return t, nil
}

func (s *Tasks) List(ctx context.Context, userID string, f models.TaskFilter) ([]*models.Task, string, error) {
	var (
		conds []string
		args  []any
		limit = f.Limit
	)
	conds = append(conds, "user_id = ?")
	args = append(args, userID)

	switch f.Filter {
	case "completed":
		conds = append(conds, "completed_at IS NOT NULL")
	case "pending":
		conds = append(conds, "completed_at IS NULL")
	}

	if f.Priority != nil && *f.Priority != "" {
		conds = append(conds, "priority = ?")
		args = append(args, string(*f.Priority))
	}
	if f.From != nil {
		conds = append(conds, "due_at >= ?")
		args = append(args, formatTime(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "due_at <= ?")
		args = append(args, formatTime(*f.To))
	}

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	if f.Cursor != "" {
		curDue, curID, ok := decodeCursor(f.Cursor)
		if ok {
			conds = append(conds, "(due_at, id) > (?, ?)")
			args = append(args, formatTime(curDue), curID)
		}
	}

	query := "SELECT " + taskColumns() + " FROM tasks WHERE " + strings.Join(conds, " AND ") +
		" ORDER BY due_at ASC, id ASC LIMIT ?"
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = rows.Close() }()

	var out []*models.Task
	for rows.Next() {
		t := &models.Task{}
		if err := scanTask(rows, t); err != nil {
			return nil, "", err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	cursor := ""
	if len(out) > limit {
		last := out[limit-1]
		cursor = encodeCursor(last.DueAt, last.ID)
		out = out[:limit]
	}
	return out, cursor, nil
}

func decodeCursor(c string) (time.Time, string, bool) {
	parts := strings.SplitN(c, "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", false
	}
	t := db.ParseUTC(parts[0])
	if t.IsZero() {
		return time.Time{}, "", false
	}
	return t, parts[1], true
}

func encodeCursor(t time.Time, id string) string {
	return t.UTC().Format(time.RFC3339Nano) + "|" + id
}

func (s *Tasks) ListDue(ctx context.Context, now time.Time, maxRows int) ([]*models.Task, error) {
	if maxRows <= 0 {
		maxRows = 500
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT `+taskColumns()+` FROM tasks
WHERE completed_at IS NULL
  AND due_at <= ?
  AND (snoozed_until IS NULL OR snoozed_until <= ?)
ORDER BY due_at ASC LIMIT ?`, formatTime(now), formatTime(now), maxRows)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*models.Task
	for rows.Next() {
		t := &models.Task{}
		if err := scanTask(rows, t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Tasks) Update(ctx context.Context, t *models.Task) error {
	t.UpdatedAt = time.Now().UTC()
	var recFreq any
	if t.RecurrenceFreq != nil {
		recFreq = string(*t.RecurrenceFreq)
	}
	var recByMonth any
	if t.RecurrenceByMonthDay != nil {
		recByMonth = *t.RecurrenceByMonthDay
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET
 title=?, description=?, priority=?, kind=?, due_at=?, duration_minutes=?, completed_at=?,
 recurrence_freq=?, recurrence_interval=?, recurrence_by_day=?, recurrence_by_month_day=?,
 recurrence_end_at=?, snoozed_until=?, updated_at=? WHERE id=? AND user_id=?`,
		t.Title, t.Description, t.Priority, t.Kind, formatTime(t.DueAt), t.DurationMinutes, formatTime(ptrTime(t.CompletedAt)),
		recFreq, t.RecurrenceInterval, nullableStr(t.RecurrenceByDay), recByMonth,
		formatTime(ptrTime(t.RecurrenceEndAt)), formatTime(ptrTime(t.SnoozedUntil)), formatTime(t.UpdatedAt), t.ID, t.UserID)
	return err
}

func (s *Tasks) SetLastNotified(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET last_notified_at=? WHERE id=?`,
		formatTime(at), id)
	return err
}

func (s *Tasks) Complete(ctx context.Context, userID, id string, now time.Time, advance func(*models.Task, time.Time) (*models.TaskCompletion, bool, error)) (*models.Task, *models.TaskCompletion, bool, error) {
	t, err := s.Get(ctx, userID, id)
	if err != nil {
		return nil, nil, false, err
	}
	if t.Completed() {
		return t, nil, false, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var completion *models.TaskCompletion
	finished := false

	if t.Recurring() {
		compl := &models.TaskCompletion{
			ID:          newUUID(),
			TaskID:      t.ID,
			UserID:      t.UserID,
			Title:       t.Title,
			Priority:    t.Priority,
			DueAt:       t.DueAt,
			CompletedAt: now,
			CreatedAt:   now,
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_completions
(id, task_id, user_id, title, priority, due_at, completed_at, created_at) VALUES (?,?,?,?,?,?,?,?)`,
			compl.ID, compl.TaskID, compl.UserID, compl.Title, compl.Priority, formatTime(compl.DueAt), formatTime(compl.CompletedAt), formatTime(compl.CreatedAt)); err != nil {
			return nil, nil, false, err
		}
		completion = compl

		if advance != nil {
			c, fin, err := advance(t, now)
			if err != nil {
				return nil, nil, false, err
			}
			_ = c
			finished = fin
		}
	} else {
		t.CompletedAt = &now
	}

	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET completed_at=?, updated_at=?, due_at=?,
 recurrence_freq=?, recurrence_interval=?, recurrence_by_day=?, recurrence_by_month_day=?, recurrence_end_at=?, last_notified_at=NULL, kind=? WHERE id=? AND user_id=?`,
		formatTime(now), formatTime(now.UTC()), formatTime(t.DueAt),
		nullableFreq(t.RecurrenceFreq), t.RecurrenceInterval, nullableStr(t.RecurrenceByDay), freqByMonthVal(t.RecurrenceByMonthDay),
		formatTime(ptrTime(t.RecurrenceEndAt)), t.Kind, t.ID, t.UserID); err != nil {
		return nil, nil, false, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, false, err
	}
	return t, completion, finished, nil
}

func (s *Tasks) Delete(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.ErrNotFound
	}
	return nil
}
