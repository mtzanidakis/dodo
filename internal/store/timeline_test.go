package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/recurrence"
	"github.com/mtzanidakis/dodo/internal/store"
)

// pageAll walks the timeline via the cursor and returns every item, asserting
// each non-final page is full and no id repeats across pages.
func pageAll(t *testing.T, s *store.Store, userID, filter string, pageSize int) []*store.TimelineItem {
	t.Helper()
	ctx := context.Background()
	var got []*store.TimelineItem
	seen := map[string]bool{}
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 100 {
			t.Fatal("timeline pagination did not terminate")
		}
		page, next, err := s.Timeline(ctx, userID, filter, nil, nil, pageSize, cursor)
		if err != nil {
			t.Fatalf("timeline %s: %v", filter, err)
		}
		for _, it := range page {
			if seen[it.ID] {
				t.Fatalf("duplicate id %s across pages (%s)", it.ID, filter)
			}
			seen[it.ID] = true
		}
		got = append(got, page...)
		if next == "" {
			break
		}
		if len(page) != pageSize {
			t.Fatalf("non-final %s page should be full, got %d", filter, len(page))
		}
		cursor = next
	}
	return got
}

func TestTimelinePaginationAndOrdering(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	loc, _ := time.LoadLocation("Europe/Athens")
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, loc)

	// 7 pending one-off tasks on consecutive days.
	for i := 0; i < 7; i++ {
		tk := &models.Task{UserID: u.ID, Title: "pending", DueAt: base.AddDate(0, 0, i), Priority: models.PriorityNormal}
		if err := s.Tasks.Create(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}
	// 3 finished one-off tasks (created then completed).
	for i := 0; i < 3; i++ {
		tk := &models.Task{UserID: u.ID, Title: "done-oneoff", DueAt: base.AddDate(0, 0, -1-i), Priority: models.PriorityLow}
		if err := s.Tasks.Create(ctx, tk); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := s.Tasks.Complete(ctx, u.ID, tk.ID, base.Add(time.Duration(i)*time.Minute), nil); err != nil {
			t.Fatal(err)
		}
	}
	// 1 recurring task completed 4 times -> 4 occurrences, parent stays pending.
	freq := models.FreqDaily
	rec := &models.Task{UserID: u.ID, Title: "daily", DueAt: base, Priority: models.PriorityHigh, RecurrenceFreq: &freq, RecurrenceInterval: 1}
	if err := s.Tasks.Create(ctx, rec); err != nil {
		t.Fatal(err)
	}
	advance := func(tk *models.Task, _ time.Time) (*models.TaskCompletion, bool, error) {
		r := recurrence.Rule{Freq: *tk.RecurrenceFreq, Interval: tk.RecurrenceInterval}
		tk.DueAt = recurrence.NextOccurrence(r, tk.DueAt, tk.DueAt, loc)
		return nil, false, nil
	}
	for i := 0; i < 4; i++ {
		if _, _, _, err := s.Tasks.Complete(ctx, u.ID, rec.ID, base.Add(time.Duration(i)*time.Hour), advance); err != nil {
			t.Fatal(err)
		}
	}

	// pending: 7 one-offs + the still-open recurring parent = 8, due ASC.
	pending := pageAll(t, s, u.ID, "pending", 3)
	if len(pending) != 8 {
		t.Fatalf("pending: want 8, got %d", len(pending))
	}
	for i := 1; i < len(pending); i++ {
		if pending[i].DueAt.Before(pending[i-1].DueAt) {
			t.Fatalf("pending not due-ascending at %d", i)
		}
	}

	// completed: 3 finished one-offs + 4 occurrences = 7, completed DESC.
	completed := pageAll(t, s, u.ID, "completed", 3)
	if len(completed) != 7 {
		t.Fatalf("completed: want 7, got %d", len(completed))
	}
	for i := 1; i < len(completed); i++ {
		a, b := completed[i-1].CompletedAt, completed[i].CompletedAt
		if a == nil || b == nil {
			t.Fatal("completed item missing CompletedAt")
		}
		if a.Before(*b) {
			t.Fatalf("completed not completion-descending at %d", i)
		}
	}
	// No recurring parent should appear as a completed one-off.
	for _, it := range completed {
		if it.Kind == store.TimelineCompleted && it.ID == rec.ID {
			t.Fatal("recurring parent leaked into completed as a one-off")
		}
	}

	// all: pending(8) + occurrences(4) + finished one-offs(3) = 15, due ASC.
	all := pageAll(t, s, u.ID, "all", 4)
	if len(all) != 15 {
		t.Fatalf("all: want 15, got %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].DueAt.Before(all[i-1].DueAt) {
			t.Fatalf("all not due-ascending at %d", i)
		}
	}
}
