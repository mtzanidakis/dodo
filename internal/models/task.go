package models

import "time"

type Task struct {
	ID                   string
	UserID               string
	Title                string
	Description          string
	Priority             Priority
	Kind                 TaskKind
	DueAt                time.Time
	DurationMinutes      int
	CompletedAt          *time.Time
	RecurrenceFreq       *RecurrenceFreq
	RecurrenceInterval   int
	RecurrenceByDay      string
	RecurrenceByMonthDay *int
	RecurrenceEndAt      *time.Time
	LastNotifiedAt       *time.Time
	SnoozedUntil         *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (t *Task) Recurring() bool {
	return t.RecurrenceFreq != nil
}

func (t *Task) Completed() bool {
	return t.CompletedAt != nil
}

func (t *Task) Snoozed(now time.Time) bool {
	return t.SnoozedUntil != nil && t.SnoozedUntil.After(now)
}

type TaskCompletion struct {
	ID          string
	TaskID      string
	UserID      string
	Title       string
	Priority    Priority
	DueAt       time.Time
	CompletedAt time.Time
	CreatedAt   time.Time
}

// PeriodBounds returns the inclusive [start, end] window for a time-period
// token ("today", "week", "month"), computed in now's location with the week
// starting Monday. Any other value (including "all" or "") returns nil bounds,
// meaning no time restriction. This axis is independent of the pending/
// completed/all status axis, so callers can combine them (e.g. completed this
// week).
func PeriodBounds(period string, now time.Time) (from, to *time.Time) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var start, end time.Time
	switch period {
	case "today":
		start = day
		end = day.AddDate(0, 0, 1).Add(-time.Second)
	case "week":
		offset := (int(now.Weekday()) + 6) % 7 // Monday = 0
		start = day.AddDate(0, 0, -offset)
		end = start.AddDate(0, 0, 7).Add(-time.Second)
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 1, 0).Add(-time.Second)
	default:
		return nil, nil
	}
	return &start, &end
}

type TaskFilter struct {
	View   string
	Filter string
	From   *time.Time
	To     *time.Time
	// CompletedFrom/CompletedTo bound completed_at (used for "completed" +
	// a time period, e.g. "what did I finish this week").
	CompletedFrom *time.Time
	CompletedTo   *time.Time
	Priority      *Priority
	Month         *time.Time
	Limit         int
	Cursor        string
}

// ApplyPeriod restricts the filter to a time period, independently of its
// status. For "completed" the window bounds completed_at ("finished during the
// period"); otherwise it upper-bounds due_at so pending views still include
// overdue tasks up to the end of the period. An empty/"all" period is a no-op.
func (f *TaskFilter) ApplyPeriod(period string, now time.Time) {
	from, to := PeriodBounds(period, now)
	if to == nil {
		return
	}
	if f.Filter == "completed" {
		f.CompletedFrom = from
		f.CompletedTo = to
	} else {
		f.To = to
	}
}
