package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/recurrence"
	"github.com/mtzanidakis/dodo/internal/store"
)

type testStore struct {
	*store.Store
}

func newTestStore(t *testing.T) (*store.Store, *models.User) {
	t.Helper()
	t.Parallel()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	s := store.New(d)
	ctx := context.Background()
	u := &models.User{Email: "user@example.com", PasswordHash: "$argon2id$...", Timezone: "Europe/Athens", Locale: models.LocaleEn}
	if err := s.Users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return s, u
}

func TestUserCreateGetByEmail(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	got, err := s.Users.GetByEmail(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}
	if got.ID != u.ID {
		t.Fatalf("id mismatch")
	}
	if got.Email != "user@example.com" {
		t.Fatalf("email mismatch: %q", got.Email)
	}
}

func TestUserUniqueEmailConflict(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	dup := &models.User{Email: "user@example.com", PasswordHash: "x"}
	err := s.Users.Create(ctx, dup)
	if err == nil {
		t.Fatalf("expected conflict on duplicate email")
	}
	u.Email = "user@example.com"
	_ = u
}

func TestUserSoftDelete(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	if err := s.Users.SoftDelete(ctx, u.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := s.Users.GetByEmail(ctx, "user@example.com"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after soft delete, got %v", err)
	}
}

func TestUserSetTelegramConfig(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	if err := s.Users.SetTelegramConfig(ctx, u.ID, "encrypted-token", "111,222"); err != nil {
		t.Fatalf("set telegram config: %v", err)
	}
	token, allowed, chatID, chatUserID, _, err := s.Users.GetTelegramConfig(ctx, u.ID)
	if err != nil {
		t.Fatalf("get telegram config: %v", err)
	}
	if token != "encrypted-token" {
		t.Fatalf("bot token mismatch: %q", token)
	}
	if allowed != "111,222" {
		t.Fatalf("allowed ids mismatch: %q", allowed)
	}
	if chatID != "" || chatUserID != "" {
		t.Fatalf("chat ids should be empty")
	}
	if err := s.Users.SetTelegramChatID(ctx, u.ID, "chat-123", "111"); err != nil {
		t.Fatalf("set chat id: %v", err)
	}
	_, _, chatID, chatUserID, _, _ = s.Users.GetTelegramConfig(ctx, u.ID)
	if chatID != "chat-123" || chatUserID != "111" {
		t.Fatalf("chat ids mismatch: %q %q", chatID, chatUserID)
	}
	if err := s.Users.ClearTelegramConfig(ctx, u.ID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	token, _, _, _, _, _ = s.Users.GetTelegramConfig(ctx, u.ID)
	if token != "" {
		t.Fatalf("expected cleared token, got %q", token)
	}
}

func TestUserListTelegramEnabled(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	if err := s.Users.SetTelegramConfig(ctx, u.ID, "tok", "1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	users, err := s.Users.ListTelegramEnabled(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(users) != 1 || users[0].ID != u.ID {
		t.Fatalf("expected 1 telegram-enabled user, got %d", len(users))
	}
}

func TestTokensCreateListRevoke(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	tok, err := s.Tokens.Create(ctx, u.ID, "agent", "dodo_abcd", "hash123")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	got, err := s.Tokens.LookupByHash(ctx, "hash123")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.ID != tok.ID {
		t.Fatalf("id mismatch")
	}
	if err := s.Tokens.Touch(ctx, tok.ID); err != nil {
		t.Fatalf("touch: %v", err)
	}
	list, err := s.Tokens.List(ctx, u.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 token, got %d", len(list))
	}
	if err := s.Tokens.Revoke(ctx, u.ID, tok.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := s.Tokens.Revoke(ctx, u.ID, tok.ID); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("revoke twice should be NotFound, got %v", err)
	}
}

func TestSessionsCreateLookupExpire(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	ses, err := s.Sessions.Create(ctx, u.ID, "sessionhash", "curl/8", 30*24*time.Hour)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	got, err := s.Sessions.Lookup(ctx, "sessionhash")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.ID != ses.ID {
		t.Fatalf("id mismatch")
	}
	if !got.ExpiresAt.After(got.CreatedAt) {
		t.Fatalf("expires_at should be after created_at")
	}
	if err := s.Sessions.Expire(ctx, ses.ID); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if _, err := s.Sessions.Lookup(ctx, "sessionhash"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected NotFound after expire, got %v", err)
	}
	if err := s.Sessions.DeleteExpired(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("delete expired: %v", err)
	}
}

func TestTasksListFiltersAndCursor(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	due := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	for i := range 3 {
		tk := &models.Task{UserID: u.ID, Title: "task", DueAt: due.Add(time.Duration(i) * time.Hour), Priority: models.PriorityNormal}
		if err := s.Tasks.Create(ctx, tk); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	high := models.PriorityHigh
	list, _, err := s.Tasks.List(ctx, u.ID, models.TaskFilter{Filter: "pending"})
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 pending, got %d", len(list))
	}
	list, _, err = s.Tasks.List(ctx, u.ID, models.TaskFilter{Filter: "pending", Priority: &high})
	if err != nil {
		t.Fatalf("list priority: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 high-priority, got %d", len(list))
	}
	from := due.Add(30 * time.Minute)
	list, _, err = s.Tasks.List(ctx, u.ID, models.TaskFilter{Filter: "pending", From: &from})
	if err != nil {
		t.Fatalf("list from: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 tasks after from, got %d", len(list))
	}
}

func TestTasksCursorPagination(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	for i := range 5 {
		tk := &models.Task{UserID: u.ID, Title: "task", DueAt: base.Add(time.Duration(i) * time.Hour), Priority: models.PriorityNormal}
		if err := s.Tasks.Create(ctx, tk); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	list, cursor, err := s.Tasks.List(ctx, u.ID, models.TaskFilter{Limit: 2})
	if err != nil {
		t.Fatalf("list page1: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("page1 expected 2, got %d", len(list))
	}
	if cursor == "" {
		t.Fatalf("expected cursor")
	}
	list2, cursor2, err := s.Tasks.List(ctx, u.ID, models.TaskFilter{Limit: 2, Cursor: cursor})
	if err != nil {
		t.Fatalf("list page2: %v", err)
	}
	if len(list2) != 2 {
		t.Fatalf("page2 expected 2, got %d", len(list2))
	}
	if list2[0].ID == list[1].ID {
		t.Fatalf("page2 overlaps page1")
	}
	list3, _, err := s.Tasks.List(ctx, u.ID, models.TaskFilter{Limit: 2, Cursor: cursor2})
	if err != nil {
		t.Fatalf("list page3: %v", err)
	}
	if len(list3) != 1 {
		t.Fatalf("page3 expected 1, got %d", len(list3))
	}
}

func TestTasksListDue(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	past := &models.Task{UserID: u.ID, Title: "past", DueAt: now.Add(-2 * time.Hour), Priority: models.PriorityNormal}
	future := &models.Task{UserID: u.ID, Title: "future", DueAt: now.Add(2 * time.Hour), Priority: models.PriorityNormal}
	snoozed := &models.Task{UserID: u.ID, Title: "snoozed", DueAt: now.Add(-1 * time.Hour), SnoozedUntil: ptrTime(now.Add(time.Hour)), Priority: models.PriorityNormal}
	for _, tk := range []*models.Task{past, future, snoozed} {
		if err := s.Tasks.Create(ctx, tk); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	due, err := s.Tasks.ListDue(ctx, now, 100)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 1 || due[0].ID != past.ID {
		t.Fatalf("expected only past due, got %d tasks", len(due))
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestTasksCompleteOneoff(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	tk := &models.Task{UserID: u.ID, Title: "oneoff", DueAt: time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC), Priority: models.PriorityNormal}
	s.Tasks.Create(ctx, tk)
	now := time.Date(2026, 7, 11, 11, 0, 0, 0, time.UTC)
	updated, compl, finished, err := s.Tasks.Complete(ctx, u.ID, tk.ID, now, nil)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if !updated.Completed() {
		t.Fatalf("should be completed")
	}
	if compl != nil {
		t.Fatalf("oneoff should not produce completion")
	}
	if finished {
		t.Fatalf("oneoff not finished flag")
	}
}

func TestTasksCompleteRecurringAdvances(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	loc, _ := time.LoadLocation("Europe/Athens")
	base := time.Date(2026, 7, 11, 10, 0, 0, 0, loc)
	freq := models.FreqDaily
	tk := &models.Task{
		UserID:             u.ID,
		Title:              "daily",
		DueAt:              base,
		Priority:           models.PriorityNormal,
		RecurrenceFreq:     &freq,
		RecurrenceInterval: 1,
	}
	s.Tasks.Create(ctx, tk)
	now := base.Add(2 * time.Hour)

	updated, compl, finished, err := s.Tasks.Complete(ctx, u.ID, tk.ID, now, func(t *models.Task, n time.Time) (*models.TaskCompletion, bool, error) {
		rule := recurrence.Rule{Freq: *t.RecurrenceFreq, Interval: t.RecurrenceInterval}
		next := recurrence.NextOccurrence(rule, t.DueAt, t.DueAt, loc)
		if next.IsZero() {
			t.CompletedAt = &n
			t.RecurrenceFreq = nil
			t.Kind = models.KindOneoff
			return nil, true, nil
		}
		t.DueAt = next
		t.LastNotifiedAt = nil
		return nil, false, nil
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if compl == nil {
		t.Fatalf("recurring should produce completion row")
	}
	if finished {
		t.Fatalf("should not be finished")
	}
	if !updated.DueAt.Equal(base.Add(24 * time.Hour)) {
		t.Fatalf("due should advance to next day, got %v", updated.DueAt)
	}
	// The persisted series must stay OPEN so future occurrences remain:
	// only the current occurrence is recorded in task_completions.
	got, err := s.Tasks.Get(ctx, u.ID, tk.ID)
	if err != nil {
		t.Fatalf("get after complete: %v", err)
	}
	if got.Completed() {
		t.Fatalf("recurring task must stay open after completing an occurrence, completed_at=%v", got.CompletedAt)
	}
	if !got.DueAt.Equal(base.Add(24 * time.Hour)) {
		t.Fatalf("persisted due should be next day, got %v", got.DueAt)
	}
	pending, _, _ := s.Tasks.List(ctx, u.ID, models.TaskFilter{Filter: "pending", Limit: 10})
	if len(pending) != 1 {
		t.Fatalf("advanced recurring task should still be pending, got %d", len(pending))
	}
}

func TestTasksCompleteRecurringEndAtFinishes(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	loc, _ := time.LoadLocation("Europe/Athens")
	base := time.Date(2026, 7, 11, 10, 0, 0, 0, loc)
	end := base.Add(12 * time.Hour)
	freq := models.FreqDaily
	tk := &models.Task{
		UserID:             u.ID,
		Title:              "daily",
		DueAt:              base,
		Priority:           models.PriorityNormal,
		RecurrenceFreq:     &freq,
		RecurrenceInterval: 1,
		RecurrenceEndAt:    &end,
	}
	s.Tasks.Create(ctx, tk)
	now := base.Add(2 * time.Hour)

	_, compl, finished, err := s.Tasks.Complete(ctx, u.ID, tk.ID, now, func(t *models.Task, n time.Time) (*models.TaskCompletion, bool, error) {
		rule := recurrence.Rule{Freq: *t.RecurrenceFreq, Interval: t.RecurrenceInterval, EndAt: dbOrZero(t.RecurrenceEndAt)}
		next := recurrence.NextOccurrence(rule, t.DueAt, t.DueAt, loc)
		if next.IsZero() {
			t.CompletedAt = &n
			t.RecurrenceFreq = nil
			t.RecurrenceByDay = ""
			t.RecurrenceByMonthDay = nil
			t.RecurrenceEndAt = nil
			t.Kind = models.KindOneoff
			return nil, true, nil
		}
		t.DueAt = next
		t.LastNotifiedAt = nil
		return nil, false, nil
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if compl == nil {
		t.Fatalf("should produce completion")
	}
	if !finished {
		t.Fatalf("should be finished after end_at")
	}
}

func dbOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func TestTasksDeleteNotFoundForOtherUser(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	tk := &models.Task{UserID: u.ID, Title: "x", DueAt: time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC), Priority: models.PriorityNormal}
	s.Tasks.Create(ctx, tk)
	if err := s.Tasks.Delete(ctx, "other-user", tk.ID); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected NotFound for other user, got %v", err)
	}
	if err := s.Tasks.Delete(ctx, u.ID, tk.ID); err != nil {
		t.Fatalf("delete own: %v", err)
	}
}

func TestTasksGetReturnsNotFoundForOtherUser(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	tk := &models.Task{UserID: u.ID, Title: "x", DueAt: time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC), Priority: models.PriorityNormal}
	s.Tasks.Create(ctx, tk)
	if _, err := s.Tasks.Get(ctx, "other-user", tk.ID); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestCompletionsList(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	loc, _ := time.LoadLocation("Europe/Athens")
	base := time.Date(2026, 7, 11, 10, 0, 0, 0, loc)
	freq := models.FreqDaily
	tk := &models.Task{UserID: u.ID, Title: "daily", DueAt: base, Priority: models.PriorityHigh, RecurrenceFreq: &freq, RecurrenceInterval: 1}
	s.Tasks.Create(ctx, tk)
	now := base.Add(time.Hour)
	s.Tasks.Complete(ctx, u.ID, tk.ID, now, func(t *models.Task, n time.Time) (*models.TaskCompletion, bool, error) {
		rule := recurrence.Rule{Freq: *t.RecurrenceFreq, Interval: t.RecurrenceInterval}
		next := recurrence.NextOccurrence(rule, t.DueAt, t.DueAt, loc)
		t.DueAt = next
		t.LastNotifiedAt = nil
		return nil, false, nil
	})
	from := now.Add(-time.Hour)
	to := now.Add(time.Hour)
	list, err := s.Completions.List(ctx, u.ID, &from, &to)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(list))
	}
	if list[0].Priority != models.PriorityHigh {
		t.Fatalf("priority mismatch: %q", list[0].Priority)
	}
}

func TestAuditLog(t *testing.T) {
	s, u := newTestStore(t)
	ctx := context.Background()
	if err := s.Audit.Log(ctx, u.ID, "login", "user", u.ID, map[string]any{"ip": "1.2.3.4"}); err != nil {
		t.Fatalf("log: %v", err)
	}
	entries, err := s.Audit.List(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "login" {
		t.Fatalf("action mismatch: %q", entries[0].Action)
	}
	if entries[0].Meta == nil || entries[0].Meta["ip"] != "1.2.3.4" {
		t.Fatalf("meta mismatch: %v", entries[0].Meta)
	}
}
