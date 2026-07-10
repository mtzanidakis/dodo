package recurrence_test

import (
	"testing"
	"time"

	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/recurrence"
)

func loc(t *testing.T) *time.Location {
	t.Helper()
	l, err := time.LoadLocation("Europe/Athens")
	if err != nil {
		t.Skipf("timezone unavailable: %v", err)
	}
	return l
}

func TestDailyInterval(t *testing.T) {
	t.Parallel()
	l := loc(t)
	base := time.Date(2026, 7, 11, 10, 0, 0, 0, l)
	r := recurrence.Rule{Freq: models.FreqDaily, Interval: 1}
	next := recurrence.NextOccurrence(r, base, base, l)
	want := base.Add(24 * time.Hour)
	if !next.Equal(want) {
		t.Fatalf("daily next: got %v want %v", next, want)
	}
	r2 := recurrence.Rule{Freq: models.FreqDaily, Interval: 3}
	next2 := recurrence.NextOccurrence(r2, base, base, l)
	if !next2.Equal(base.Add(3 * 24 * time.Hour)) {
		t.Fatalf("daily interval 3: got %v", next2)
	}
}

func TestWeeklyMultiDay(t *testing.T) {
	t.Parallel()
	l := loc(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, l) // Monday
	r := recurrence.Rule{Freq: models.FreqWeekly, Interval: 1, ByDay: []time.Weekday{time.Tuesday, time.Thursday}}
	next := recurrence.NextOccurrence(r, base, base, l) // first after base is Tue 7/7
	want := time.Date(2026, 7, 7, 9, 0, 0, 0, l)
	if !next.Equal(want) {
		t.Fatalf("weekly multi-day: got %v want %v", next, want)
	}
	next2 := recurrence.NextOccurrence(r, base, next, l) // next after Tue is Thu 7/9
	want2 := time.Date(2026, 7, 9, 9, 0, 0, 0, l)
	if !next2.Equal(want2) {
		t.Fatalf("weekly 2nd: got %v want %v", next2, want2)
	}
}

func TestWeeklyInterval2(t *testing.T) {
	t.Parallel()
	l := loc(t)
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, l) // Monday
	r := recurrence.Rule{Freq: models.FreqWeekly, Interval: 2, ByDay: []time.Weekday{time.Monday}}
	next := recurrence.NextOccurrence(r, base, base, l)
	want := base.Add(14 * 24 * time.Hour)
	if !next.Equal(want) {
		t.Fatalf("weekly interval 2: got %v want %v", next, want)
	}
}

func TestMonthlyEndOfMonthClamp(t *testing.T) {
	t.Parallel()
	l := loc(t)
	base := time.Date(2026, 1, 31, 10, 0, 0, 0, l)
	r := recurrence.Rule{Freq: models.FreqMonthly, Interval: 1, ByMonthDay: 31}
	// Jan 31 -> Feb has no 31, skip -> Mar 31
	next := recurrence.NextOccurrence(r, base, base, l)
	want := time.Date(2026, 3, 31, 10, 0, 0, 0, l)
	if !next.Equal(want) {
		t.Fatalf("monthly clamp skip: got %v want %v", next, want)
	}
}

func TestMonthlyUsesBaseDay(t *testing.T) {
	t.Parallel()
	l := loc(t)
	base := time.Date(2026, 7, 15, 10, 0, 0, 0, l)
	r := recurrence.Rule{Freq: models.FreqMonthly, Interval: 1}
	next := recurrence.NextOccurrence(r, base, base, l)
	want := time.Date(2026, 8, 15, 10, 0, 0, 0, l)
	if !next.Equal(want) {
		t.Fatalf("monthly base day: got %v want %v", next, want)
	}
}

func TestYearlyAcrossLeap(t *testing.T) {
	t.Parallel()
	l := loc(t)
	base := time.Date(2024, 2, 29, 10, 0, 0, 0, l) // leap day
	r := recurrence.Rule{Freq: models.FreqYearly, Interval: 1}
	// 2025 Feb 29 invalid -> skip to 2026? Actually day 29, 2025 Feb has 28; skip to 2028 leap
	next := recurrence.NextOccurrence(r, base, base, l)
	want := time.Date(2028, 2, 29, 10, 0, 0, 0, l)
	if !next.Equal(want) {
		t.Fatalf("yearly leap: got %v want %v", next, want)
	}
}

func TestDSTFold(t *testing.T) {
	t.Parallel()
	l := loc(t)
	// Europe/Athens DST spring forward end of March 2026 (Mar 29 03:00 -> 04:00)
	base := time.Date(2026, 3, 28, 2, 30, 0, 0, l)
	r := recurrence.Rule{Freq: models.FreqDaily, Interval: 1}
	next := recurrence.NextOccurrence(r, base, base, l)
	// 02:30 on Mar 29 doesn't exist; time.Date normalises to 03:30 (or stays). Just check the date.
	if next.Month() != time.March || next.Day() != 29 {
		t.Fatalf("DST next date: got %v", next)
	}
}

func TestEndAtCutoff(t *testing.T) {
	t.Parallel()
	l := loc(t)
	base := time.Date(2026, 7, 11, 10, 0, 0, 0, l)
	end := base.Add(36 * time.Hour)
	r := recurrence.Rule{Freq: models.FreqDaily, Interval: 1, EndAt: end}
	next := recurrence.NextOccurrence(r, base, base, l) // +24h within end
	if !next.Equal(base.Add(24*time.Hour)) {
		t.Fatalf("within end: got %v", next)
	}
	next2 := recurrence.NextOccurrence(r, next, next, l) // +48h > end(36h) → zero
	if !next2.IsZero() {
		t.Fatalf("past end should be zero, got %v", next2)
	}
}

func TestOccurrencesWindow(t *testing.T) {
	t.Parallel()
	l := loc(t)
	base := time.Date(2026, 7, 1, 10, 0, 0, 0, l)
	r := recurrence.Rule{Freq: models.FreqDaily, Interval: 1}
	from := base
	to := base.Add(48 * time.Hour)
	occs := recurrence.Occurrences(r, base, from, to, l)
	if len(occs) != 2 {
		t.Fatalf("expected 2 occurrences in 48h window (after base), got %d", len(occs))
	}
}