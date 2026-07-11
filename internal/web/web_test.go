package web

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/ws"
)

func newWebEnv(t *testing.T) (*http.ServeMux, *store.Store, *models.User, string) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	st := store.New(d)
	ctx := context.Background()
	hash, _ := auth.HashPassword("pass1234")
	u := &models.User{Email: "w@example.com", PasswordHash: hash, Timezone: "Europe/Athens", Locale: models.LocaleEn, Theme: models.ThemeSystem}
	if err := st.Users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	gen, _ := auth.GenerateSession()
	if _, err := st.Sessions.Create(ctx, u.ID, gen.Hash, "test", 24*time.Hour); err != nil {
		t.Fatalf("session: %v", err)
	}
	h := NewHandler(Deps{Store: st, AuthMW: &auth.Middleware{Store: st}, Hub: ws.NewHub(slog.Default()), Version: "test"})
	mux := http.NewServeMux()
	h.Mount(mux)
	return mux, st, u, gen.Full
}

func withSession(req *http.Request, session string) {
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: session})
}

func TestHomeRendersTasks(t *testing.T) {
	mux, st, u, session := newWebEnv(t)
	due := time.Now().UTC().Add(time.Hour)
	if err := st.Tasks.Create(context.Background(), &models.Task{UserID: u.ID, Title: "Buy milk", Priority: models.PriorityHigh, DueAt: due}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	withSession(req, session)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("home: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Buy milk") {
		t.Fatalf("home missing task: %s", body)
	}
	if !strings.Contains(body, "task-list") || !strings.Contains(body, "New task") {
		t.Fatalf("home missing chrome")
	}
}

func TestPeriodFilterScopesPendingToToday(t *testing.T) {
	mux, st, u, session := newWebEnv(t)
	loc, _ := time.LoadLocation(u.Timezone)
	now := time.Now().In(loc)
	todayDue := time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, loc)
	nextWeekDue := todayDue.AddDate(0, 0, 8)
	for _, tk := range []*models.Task{
		{UserID: u.ID, Title: "DueToday", Priority: models.PriorityNormal, DueAt: todayDue.UTC()},
		{UserID: u.ID, Title: "DueNextWeek", Priority: models.PriorityNormal, DueAt: nextWeekDue.UTC()},
	} {
		if err := st.Tasks.Create(context.Background(), tk); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/?filter=pending&period=today", nil)
	withSession(req, session)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "DueToday") {
		t.Fatalf("today period should include today's pending task")
	}
	if strings.Contains(body, "DueNextWeek") {
		t.Fatalf("today period should exclude next week's task")
	}
}

func TestCompletedThisWeekCombinesAxes(t *testing.T) {
	mux, st, u, session := newWebEnv(t)
	loc, _ := time.LoadLocation(u.Timezone)
	now := time.Now().In(loc)
	// Two tasks due long ago; complete one now (this week), leave one pending.
	old := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, loc).AddDate(0, 0, -40)
	doneTask := &models.Task{UserID: u.ID, Title: "FinishedThisWeek", Priority: models.PriorityNormal, DueAt: old.UTC()}
	pendingTask := &models.Task{UserID: u.ID, Title: "StillPending", Priority: models.PriorityNormal, DueAt: old.UTC()}
	for _, tk := range []*models.Task{doneTask, pendingTask} {
		if err := st.Tasks.Create(context.Background(), tk); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	if _, _, _, err := st.Tasks.Complete(context.Background(), u.ID, doneTask.ID, time.Now().UTC(), nil); err != nil {
		t.Fatalf("complete: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/?filter=completed&period=week", nil)
	withSession(req, session)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "FinishedThisWeek") {
		t.Fatalf("completed+week should show a task completed this week")
	}
	if strings.Contains(body, "StillPending") {
		t.Fatalf("completed+week should not show a pending task")
	}
}

func TestCalendarRenders(t *testing.T) {
	mux, _, _, session := newWebEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/?view=calendar", nil)
	withSession(req, session)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("calendar: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cal-grid") {
		t.Fatalf("calendar grid missing")
	}
}

func TestCalendarExpandsRecurringOccurrences(t *testing.T) {
	mux, st, u, session := newWebEnv(t)
	loc, _ := time.LoadLocation(u.Timezone)
	now := time.Now().In(loc)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	// A daily task starting on the 2nd of the visible month.
	freq := models.FreqDaily
	start := time.Date(month.Year(), month.Month(), 2, 9, 0, 0, 0, loc)
	if err := st.Tasks.Create(context.Background(), &models.Task{
		UserID: u.ID, Title: "Standup", Priority: models.PriorityNormal,
		DueAt: start.UTC(), RecurrenceFreq: &freq, RecurrenceInterval: 1, Kind: models.KindRecurring,
	}); err != nil {
		t.Fatalf("create recurring: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/?view=calendar&month="+month.Format("2006-01"), nil)
	withSession(req, session)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("calendar: %d", rec.Code)
	}
	// A daily task must appear many times, not once.
	occurrences := strings.Count(rec.Body.String(), ">Standup<")
	daysInMonth := month.AddDate(0, 1, -1).Day()
	if occurrences < daysInMonth-2 {
		t.Fatalf("recurring task should appear on ~every day of the month, got %d of %d", occurrences, daysInMonth)
	}
}

func TestAccountAndTokensRender(t *testing.T) {
	mux, _, _, session := newWebEnv(t)
	for _, path := range []string{"/account", "/tokens"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		withSession(req, session)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
}

func TestCreateTaskFormAndComplete(t *testing.T) {
	mux, st, u, session := newWebEnv(t)
	csrf := "tok-csrf-value"

	form := url.Values{"title": {"Walk dog"}, "due_at": {time.Now().Add(time.Hour).Format("2006-01-02T15:04")}, "priority": {"normal"}, "_csrf": {csrf}}
	req := httptest.NewRequest(http.MethodPost, "/ui/tasks", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", csrf)
	withSession(req, session)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookie, Value: csrf})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create task: %d", rec.Code)
	}

	tasks, _, _ := st.Tasks.List(context.Background(), u.ID, models.TaskFilter{Filter: "pending", Limit: 10})
	if len(tasks) != 1 || tasks[0].Title != "Walk dog" {
		t.Fatalf("task not created: %+v", tasks)
	}

	// Complete via the htmx fragment endpoint.
	req = httptest.NewRequest(http.MethodPost, "/ui/tasks/"+tasks[0].ID+"/complete", nil)
	req.Header.Set("X-CSRF-Token", csrf)
	withSession(req, session)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookie, Value: csrf})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "done") {
		t.Fatalf("complete fragment missing done marker: %s", rec.Body.String())
	}
}

func TestCSRFRejectsMissingToken(t *testing.T) {
	mux, _, _, session := newWebEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/ui/tasks", strings.NewReader("title=x&due_at=2026-01-01T10:00"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	withSession(req, session)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookie, Value: "abc"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without csrf token, got %d", rec.Code)
	}
}
