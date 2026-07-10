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

type TaskFilter struct {
	View     string
	Filter   string
	From     *time.Time
	To       *time.Time
	Priority *Priority
	Month    *time.Time
	Limit    int
	Cursor   string
}
