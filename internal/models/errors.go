package models

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrConflict     = errors.New("conflict")
	ErrValidation   = errors.New("validation error")
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

func (p Priority) Valid() bool {
	return p == PriorityLow || p == PriorityNormal || p == PriorityHigh
}

func (p Priority) Icon() string {
	switch p {
	case PriorityHigh:
		return "!!!"
	case PriorityLow:
		return "~"
	default:
		return "!"
	}
}

func PriorityReminderInterval(p Priority) time.Duration {
	switch p {
	case PriorityHigh:
		return 20 * time.Minute
	case PriorityLow:
		return 2 * time.Hour
	default:
		return time.Hour
	}
}

type TaskKind string

const (
	KindOneoff    TaskKind = "oneoff"
	KindRecurring TaskKind = "recurring"
)

type RecurrenceFreq string

const (
	FreqDaily   RecurrenceFreq = "daily"
	FreqWeekly  RecurrenceFreq = "weekly"
	FreqMonthly RecurrenceFreq = "monthly"
	FreqYearly  RecurrenceFreq = "yearly"
)

func (f RecurrenceFreq) Valid() bool {
	return f == FreqDaily || f == FreqWeekly || f == FreqMonthly || f == FreqYearly
}

type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

func (t Theme) Valid() bool {
	return t == ThemeSystem || t == ThemeLight || t == ThemeDark
}

type Locale string

const (
	LocaleEn Locale = "en"
	LocaleEl Locale = "el"
)

func (l Locale) Valid() bool {
	return l == LocaleEn || l == LocaleEl
}

func ParsePriority(s string) (Priority, error) {
	p := Priority(s)
	if !p.Valid() {
		return "", fmt.Errorf("%w: invalid priority %q", ErrValidation, s)
	}
	return p, nil
}

func ParseRecurrenceFreq(s string) (RecurrenceFreq, error) {
	f := RecurrenceFreq(s)
	if !f.Valid() {
		return "", fmt.Errorf("%w: invalid recurrence freq %q", ErrValidation, s)
	}
	return f, nil
}
