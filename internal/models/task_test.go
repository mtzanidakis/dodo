package models

import (
	"testing"
	"time"
)

func TestPeriodBounds(t *testing.T) {
	// Wednesday 2026-07-15 14:30 local.
	now := time.Date(2026, 7, 15, 14, 30, 0, 0, time.UTC)

	t.Run("none", func(t *testing.T) {
		for _, p := range []string{"all", "weird", ""} {
			from, to := PeriodBounds(p, now)
			if from != nil || to != nil {
				t.Fatalf("%q: want nil bounds, got %v..%v", p, from, to)
			}
		}
	})

	cases := []struct {
		period     string
		start, end time.Time
	}{
		{"today", time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 15, 23, 59, 59, 0, time.UTC)},
		// Monday-first week containing Wed 15th: Mon 13 .. Sun 19.
		{"week", time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 19, 23, 59, 59, 0, time.UTC)},
		{"month", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 23, 59, 59, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.period, func(t *testing.T) {
			from, to := PeriodBounds(c.period, now)
			if from == nil || to == nil {
				t.Fatalf("%s: nil bounds", c.period)
			}
			if !from.Equal(c.start) || !to.Equal(c.end) {
				t.Fatalf("%s: got %v..%v, want %v..%v", c.period, from, to, c.start, c.end)
			}
		})
	}
}

func TestApplyPeriod(t *testing.T) {
	now := time.Date(2026, 7, 15, 14, 30, 0, 0, time.UTC)

	// pending + week -> upper-bounds due_at (includes overdue), no completed bound.
	pf := TaskFilter{Filter: "pending"}
	pf.ApplyPeriod("week", now)
	if pf.To == nil || pf.From != nil || pf.CompletedFrom != nil || pf.CompletedTo != nil {
		t.Fatalf("pending+week: %+v", pf)
	}

	// completed + week -> windows completed_at, leaves due_at untouched.
	cf := TaskFilter{Filter: "completed"}
	cf.ApplyPeriod("week", now)
	if cf.CompletedFrom == nil || cf.CompletedTo == nil || cf.To != nil {
		t.Fatalf("completed+week: %+v", cf)
	}

	// all period -> no-op.
	nf := TaskFilter{Filter: "completed"}
	nf.ApplyPeriod("all", now)
	if nf.To != nil || nf.CompletedFrom != nil || nf.CompletedTo != nil {
		t.Fatalf("all: %+v", nf)
	}
}
