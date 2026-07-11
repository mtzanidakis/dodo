package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/models"
)

// TimelineKind distinguishes the three row sources a task timeline mixes.
type TimelineKind string

const (
	TimelinePending    TimelineKind = "pending"        // an open task
	TimelineCompleted  TimelineKind = "completed_task" // a finished one-off task
	TimelineOccurrence TimelineKind = "occurrence"     // a recorded completion of a recurring task
)

// TimelineItem is one row of a unified task timeline.
type TimelineItem struct {
	Kind               TimelineKind
	ID                 string
	TaskID             string
	Title              string
	Priority           models.Priority
	DueAt              time.Time
	CompletedAt        *time.Time
	RecurrenceFreq     *models.RecurrenceFreq
	RecurrenceInterval int
	SnoozedUntil       *time.Time
	Description        string
}

// timelineCols is the projected column list, in scan order (sort_key is used
// only for ORDER BY, so it is not scanned).
const timelineCols = "kind, id, task_id, title, priority, due_at, completed_at, " +
	"recurrence_freq, recurrence_interval, snoozed_until, description"

// Timeline returns a user's task timeline for the given filter
// (pending|completed|all) with cursor pagination. pending/all are ordered by
// due date ascending; completed by completion time descending. It unifies open
// tasks, finished one-off tasks and recurring-task completion occurrences into
// one ordered, paginated stream, returning the page and an opaque next cursor
// ("" on the last page). from/to window the sort axis when non-nil.
func (s *Store) Timeline(ctx context.Context, userID, filter string, from, to *time.Time, limit int, cursor string) ([]*TimelineItem, string, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	// Completed view is a history ordered by completion time (newest first);
	// pending/all are a forward-looking due-date timeline.
	desc := filter == "completed"
	sortCol := "due_at"
	if desc {
		sortCol = "completed_at"
	}

	var curTime time.Time
	var curID string
	hasCursor := false
	if cursor != "" {
		if t, id, ok := decodeCursor(cursor); ok {
			curTime, curID, hasCursor = t, id, true
		}
	}
	seek := func(col string, args *[]any) string {
		if !hasCursor {
			return ""
		}
		op := ">"
		if desc {
			op = "<"
		}
		*args = append(*args, formatTime(curTime), curID)
		return " AND (" + col + ", id) " + op + " (?, ?)"
	}
	window := func(col string, args *[]any) string {
		clause := ""
		if from != nil {
			clause += " AND " + col + " >= ?"
			*args = append(*args, formatTime(*from))
		}
		if to != nil {
			clause += " AND " + col + " <= ?"
			*args = append(*args, formatTime(*to))
		}
		return clause
	}

	var arms []string
	var args []any

	if filter == "pending" || filter == "all" {
		a := "SELECT 'pending' AS kind, id, id AS task_id, title, priority, due_at, completed_at, " +
			"recurrence_freq, recurrence_interval, snoozed_until, description, due_at AS sort_key " +
			"FROM tasks WHERE user_id = ? AND completed_at IS NULL"
		args = append(args, userID)
		a += window("due_at", &args)
		a += seek("due_at", &args)
		arms = append(arms, a)
	}
	if filter == "completed" || filter == "all" {
		a := "SELECT 'occurrence' AS kind, id, task_id, title, priority, due_at, completed_at, " +
			"NULL AS recurrence_freq, 0 AS recurrence_interval, NULL AS snoozed_until, '' AS description, " +
			sortCol + " AS sort_key FROM task_completions WHERE user_id = ?"
		args = append(args, userID)
		a += window(sortCol, &args)
		a += seek(sortCol, &args)
		arms = append(arms, a)

		// Finished one-off tasks; recurring parents and tasks already recorded
		// in task_completions are excluded so they don't double up.
		b := "SELECT 'completed_task' AS kind, id, id AS task_id, title, priority, due_at, completed_at, " +
			"recurrence_freq, recurrence_interval, snoozed_until, description, " + sortCol + " AS sort_key " +
			"FROM tasks WHERE user_id = ? AND completed_at IS NOT NULL AND recurrence_freq IS NULL " +
			"AND id NOT IN (SELECT task_id FROM task_completions WHERE user_id = ?)"
		args = append(args, userID, userID)
		b += window(sortCol, &args)
		b += seek(sortCol, &args)
		arms = append(arms, b)
	}
	if len(arms) == 0 {
		return nil, "", nil
	}

	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	query := "SELECT " + timelineCols + " FROM (" + strings.Join(arms, " UNION ALL ") + ") " +
		"ORDER BY sort_key " + dir + ", id " + dir + " LIMIT ?"
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = rows.Close() }()

	var out []*TimelineItem
	for rows.Next() {
		it, err := scanTimelineItem(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	next := ""
	if len(out) > limit {
		last := out[limit-1]
		key := last.DueAt
		if desc && last.CompletedAt != nil {
			key = *last.CompletedAt
		}
		next = encodeCursor(key, last.ID)
		out = out[:limit]
	}
	return out, next, nil
}

func scanTimelineItem(rows *sql.Rows) (*TimelineItem, error) {
	var (
		kind        string
		dueAt       sql.NullString
		completedAt sql.NullString
		recFreq     sql.NullString
		recInterval sql.NullInt64
		snoozed     sql.NullString
		desc        sql.NullString
	)
	it := &TimelineItem{}
	if err := rows.Scan(&kind, &it.ID, &it.TaskID, &it.Title, &it.Priority, &dueAt, &completedAt,
		&recFreq, &recInterval, &snoozed, &desc); err != nil {
		return nil, err
	}
	it.Kind = TimelineKind(kind)
	it.DueAt = parseTime(dueAt)
	if completedAt.Valid && completedAt.String != "" {
		t := parseTime(completedAt)
		it.CompletedAt = &t
	}
	if recFreq.Valid && recFreq.String != "" {
		f := models.RecurrenceFreq(recFreq.String)
		it.RecurrenceFreq = &f
	}
	it.RecurrenceInterval = int(recInterval.Int64)
	if snoozed.Valid && snoozed.String != "" {
		t := parseTime(snoozed)
		it.SnoozedUntil = &t
	}
	it.Description = desc.String
	return it, nil
}
