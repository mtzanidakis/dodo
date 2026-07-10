package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/models"
)

type Completions struct {
	db *sql.DB
}

func completionColumns() string {
	return `id, task_id, user_id, title, priority, due_at, completed_at, created_at`
}

func scanCompletion(row interface {
	Scan(...any) error
}, c *models.TaskCompletion) error {
	var dueAt, completedAt, createdAt sql.NullString
	if err := row.Scan(&c.ID, &c.TaskID, &c.UserID, &c.Title, &c.Priority, &dueAt, &completedAt, &createdAt); err != nil {
		return err
	}
	c.DueAt = parseTime(dueAt)
	c.CompletedAt = parseTime(completedAt)
	c.CreatedAt = parseTime(createdAt)
	return nil
}

func (s *Completions) List(ctx context.Context, userID string, from, to *time.Time) ([]*models.TaskCompletion, error) {
	var (
		conds []string
		args  []any
	)
	conds = append(conds, "user_id = ?")
	args = append(args, userID)
	if from != nil {
		conds = append(conds, "completed_at >= ?")
		args = append(args, formatTime(*from))
	}
	if to != nil {
		conds = append(conds, "completed_at <= ?")
		args = append(args, formatTime(*to))
	}
	query := "SELECT " + completionColumns() + " FROM task_completions WHERE " +
		strings.Join(conds, " AND ") + " ORDER BY completed_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*models.TaskCompletion
	for rows.Next() {
		c := &models.TaskCompletion{}
		if err := scanCompletion(rows, c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
