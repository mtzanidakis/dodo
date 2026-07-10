package recurrence

import (
	"time"

	"github.com/mtzanidakis/dodo/internal/models"
)

type Rule struct {
	Freq       models.RecurrenceFreq
	Interval   int
	ByDay      []time.Weekday
	ByMonthDay int
	EndAt      time.Time
}

func normalize(r Rule) Rule {
	if r.Interval < 1 {
		r.Interval = 1
	}
	return r
}

func NextOccurrence(r Rule, base, after time.Time, loc *time.Location) time.Time {
	if r.Freq == "" {
		return time.Time{}
	}
	r = normalize(r)
	if loc == nil {
		loc = time.UTC
	}

	var next time.Time
	switch r.Freq {
	case models.FreqDaily:
		next = advanceDaily(r, base, after, loc)
	case models.FreqWeekly:
		next = advanceWeekly(r, base, after, loc)
	case models.FreqMonthly:
		next = advanceMonthly(r, base, after, loc)
	case models.FreqYearly:
		next = advanceYearly(r, base, after, loc)
	}

	if next.IsZero() || !withinEnd(r, next) {
		return time.Time{}
	}
	return next.UTC()
}

func withinEnd(r Rule, t time.Time) bool {
	if r.EndAt.IsZero() {
		return true
	}
	return !t.After(r.EndAt)
}

func advanceDaily(r Rule, base, after time.Time, loc *time.Location) time.Time {
	b := base.In(loc)
	candidate := time.Date(b.Year(), b.Month(), b.Day(), b.Hour(), b.Minute(), b.Second(), 0, loc)
	for i := 0; i < 366*100; i++ {
		if candidate.After(after) {
			return candidate
		}
		candidate = candidate.AddDate(0, 0, r.Interval)
	}
	return time.Time{}
}

func advanceWeekly(r Rule, base, after time.Time, loc *time.Location) time.Time {
	b := base.In(loc)
	days := r.ByDay
	if len(days) == 0 {
		days = []time.Weekday{b.Weekday()}
	}
	set := make(map[time.Weekday]bool)
	for _, d := range days {
		set[d] = true
	}

	start := time.Date(b.Year(), b.Month(), b.Day(), b.Hour(), b.Minute(), b.Second(), 0, loc)
	offset := int(b.Weekday() - days[0])
	if offset < 0 {
		offset += 7
	}
	start = start.AddDate(0, 0, -offset)

	for weeks := 0; weeks < 52*100; weeks++ {
		for d := 0; d < 7; d++ {
			candidate := start.AddDate(0, 0, d)
			wd := candidate.Weekday()
			if set[wd] && candidate.After(after) {
				return candidate
			}
		}
		start = start.AddDate(0, 0, 7*r.Interval)
	}
	return time.Time{}
}

func lastDayOfMonth(year, month int) int {
	return time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
}

func advanceMonthly(r Rule, base, after time.Time, loc *time.Location) time.Time {
	b := base.In(loc)
	day := r.ByMonthDay
	if day == 0 {
		day = b.Day()
	}

	year, month := b.Year(), int(b.Month())
	for i := 0; i < 12*120; i++ {
		ld := lastDayOfMonth(year, month)
		if day > ld {
			month += r.Interval
			continue
		}
		candidate := time.Date(year, time.Month(month), day, b.Hour(), b.Minute(), b.Second(), 0, loc)
		if candidate.After(after) {
			return candidate
		}
		month += r.Interval
	}
	return time.Time{}
}

func advanceYearly(r Rule, base, after time.Time, loc *time.Location) time.Time {
	b := base.In(loc)
	year, month, day := b.Year(), int(b.Month()), b.Day()
	for i := 0; i < 200; i++ {
		ld := lastDayOfMonth(year, month)
		if day > ld {
			year += r.Interval
			continue
		}
		candidate := time.Date(year, time.Month(month), day, b.Hour(), b.Minute(), b.Second(), 0, loc)
		if candidate.After(after) {
			return candidate
		}
		year += r.Interval
	}
	return time.Time{}
}

func Occurrences(r Rule, base, from, to time.Time, loc *time.Location) []time.Time {
	r = normalize(r)
	if loc == nil {
		loc = time.UTC
	}
	var out []time.Time
	after := from
	for i := 0; i < 10000; i++ {
		next := NextOccurrence(r, base, after, loc)
		if next.IsZero() || next.After(to) {
			break
		}
		out = append(out, next)
		after = next
	}
	return out
}
