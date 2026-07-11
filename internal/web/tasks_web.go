package web

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/i18n"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/recurrence"
)

type taskView struct {
	ID              string
	Title           string
	Description     string
	Priority        models.Priority
	DueAt           string // RFC3339 UTC, for data attributes
	DueLocal        string // "15:04"
	DueDateLocal    string // "Mon 2 Jan"
	Recurring       bool
	RecurrenceLabel string
	Completed       bool
	Snoozed         bool
	SnoozedUntil    string
}

type dayGroup struct {
	Label string
	Tasks []taskView
}

type freqOption struct {
	Value    string
	Label    string
	Selected bool
}

func freqOptions(lang, selected string) []freqOption {
	opts := []struct{ v, k string }{
		{"", "recurrence.none"},
		{"daily", "recurrence.daily"},
		{"weekly", "recurrence.weekly"},
		{"monthly", "recurrence.monthly"},
		{"yearly", "recurrence.yearly"},
	}
	out := make([]freqOption, 0, len(opts))
	for _, o := range opts {
		out = append(out, freqOption{Value: o.v, Label: i18n.T(o.k, lang), Selected: o.v == selected})
	}
	return out
}

func recurrenceLabel(t *models.Task, lang string) string {
	if t.RecurrenceFreq == nil {
		return ""
	}
	key := "recurrence." + string(*t.RecurrenceFreq)
	label := i18n.T(key, lang)
	if t.RecurrenceInterval > 1 {
		return label + " ×" + strconv.Itoa(t.RecurrenceInterval)
	}
	return label
}

func (h *Handler) toTaskView(t *models.Task, loc *time.Location, lang string, now time.Time) taskView {
	local := t.DueAt.In(loc)
	tv := taskView{
		ID:              t.ID,
		Title:           t.Title,
		Description:     t.Description,
		Priority:        t.Priority,
		DueAt:           t.DueAt.UTC().Format(time.RFC3339),
		DueLocal:        local.Format("15:04"),
		DueDateLocal:    local.Format("Mon 2 Jan"),
		Recurring:       t.Recurring(),
		RecurrenceLabel: recurrenceLabel(t, lang),
		Completed:       t.Completed(),
	}
	if t.SnoozedUntil != nil && t.SnoozedUntil.After(now) {
		tv.Snoozed = true
		tv.SnoozedUntil = t.SnoozedUntil.In(loc).Format("Mon 2 Jan 15:04")
	}
	return tv
}

func dayLabel(local, now time.Time, lang string) string {
	y1, m1, d1 := local.Date()
	y2, m2, d2 := now.Date()
	today := time.Date(y2, m2, d2, 0, 0, 0, 0, now.Location())
	this := time.Date(y1, m1, d1, 0, 0, 0, 0, local.Location())
	switch diff := int(this.Sub(today).Hours() / 24); diff {
	case 0:
		return i18n.T("day.today", lang)
	case 1:
		return i18n.T("day.tomorrow", lang)
	case -1:
		return i18n.T("day.yesterday", lang)
	default:
		return local.Format("Mon 2 Jan 2006")
	}
}

func (h *Handler) buildGroups(tasks []*models.Task, loc *time.Location, lang string, now time.Time) []dayGroup {
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].DueAt.Before(tasks[j].DueAt) })
	var groups []dayGroup
	var curKey string
	for _, t := range tasks {
		local := t.DueAt.In(loc)
		key := local.Format("2006-01-02")
		if key != curKey || len(groups) == 0 {
			groups = append(groups, dayGroup{Label: dayLabel(local, now, lang)})
			curKey = key
		}
		g := &groups[len(groups)-1]
		g.Tasks = append(g.Tasks, h.toTaskView(t, loc, lang, now))
	}
	return groups
}

func (h *Handler) handleHome(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	loc := loadLoc(u.Timezone)
	now := time.Now().In(loc)
	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "pending"
	}
	view := r.URL.Query().Get("view")

	pd := h.base(w, r, u, i18n.T("nav.tasks", string(u.Locale)), "tasks")
	pd.Filter = filter
	pd.View = view
	pd.Freqs = freqOptions(string(u.Locale), "")

	if view == "calendar" {
		pd.Calendar = h.buildCalendar(r, u, loc, now)
		h.render(w, "tasks/index.html", pd)
		return
	}

	tasks, _, _ := h.deps.Store.Tasks.List(r.Context(), u.ID, models.TaskFilter{Filter: filter, Limit: 200})
	pd.Groups = h.buildGroups(tasks, loc, string(u.Locale), now)
	h.render(w, "tasks/index.html", pd)
}

// ---- calendar -------------------------------------------------------------

type calEvent struct {
	Title    string
	Priority models.Priority
	Done     bool
}

func ruleFromTask(t *models.Task) recurrence.Rule {
	rule := recurrence.Rule{
		Freq:       *t.RecurrenceFreq,
		Interval:   t.RecurrenceInterval,
		ByDay:      parseByDay(t.RecurrenceByDay),
		ByMonthDay: intOrZero(t.RecurrenceByMonthDay),
	}
	if t.RecurrenceEndAt != nil {
		rule.EndAt = *t.RecurrenceEndAt
	}
	return rule
}

type calDay struct {
	Day     int
	InMonth bool
	Today   bool
	Events  []calEvent
}

type calendarView struct {
	MonthLabel string
	Prev       string
	Next       string
	DOW        []string
	Weeks      [][]calDay
}

func (h *Handler) buildCalendar(r *http.Request, u *models.User, loc *time.Location, now time.Time) *calendarView {
	monthStr := r.URL.Query().Get("month")
	var anchor time.Time
	if t, err := time.ParseInLocation("2006-01", monthStr, loc); err == nil {
		anchor = t
	} else {
		anchor = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	}
	first := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, loc)
	next := first.AddDate(0, 1, 0)

	byDay := map[int][]calEvent{}
	add := func(local time.Time, title string, prio models.Priority, done bool) {
		if local.Year() == first.Year() && local.Month() == first.Month() {
			byDay[local.Day()] = append(byDay[local.Day()], calEvent{Title: title, Priority: prio, Done: done})
		}
	}

	// Pending tasks: one-offs land on their due day; recurring tasks are
	// expanded to every occurrence that falls within the visible month.
	winFrom := first.Add(-time.Second).UTC()
	winTo := next.UTC()
	pending, _, _ := h.deps.Store.Tasks.List(r.Context(), u.ID, models.TaskFilter{Filter: "pending", Limit: 500})
	for _, t := range pending {
		if t.Recurring() {
			for _, occ := range recurrence.Occurrences(ruleFromTask(t), t.DueAt, winFrom, winTo, loc) {
				add(occ.In(loc), t.Title, t.Priority, false)
			}
		} else {
			add(t.DueAt.In(loc), t.Title, t.Priority, false)
		}
	}
	// Completed one-off tasks in this month.
	completed, _, _ := h.deps.Store.Tasks.List(r.Context(), u.ID, models.TaskFilter{Filter: "completed", From: ptrT(first.UTC()), To: ptrT(next.UTC()), Limit: 500})
	for _, t := range completed {
		add(t.DueAt.In(loc), t.Title, t.Priority, true)
	}
	// Completed occurrences of recurring tasks (history) in this month.
	if compls, err := h.deps.Store.Completions.List(r.Context(), u.ID, ptrT(first.UTC()), ptrT(next.UTC())); err == nil {
		for _, c := range compls {
			add(c.DueAt.In(loc), c.Title, c.Priority, true)
		}
	}

	// Monday-first grid.
	offset := (int(first.Weekday()) + 6) % 7
	gridStart := first.AddDate(0, 0, -offset)
	var weeks [][]calDay
	cur := gridStart
	for wk := 0; wk < 6; wk++ {
		var week []calDay
		for d := 0; d < 7; d++ {
			inMonth := cur.Month() == first.Month()
			cd := calDay{Day: cur.Day(), InMonth: inMonth, Today: sameDay(cur, now)}
			if inMonth {
				cd.Events = byDay[cur.Day()]
			}
			week = append(week, cd)
			cur = cur.AddDate(0, 0, 1)
		}
		weeks = append(weeks, week)
		if cur.After(next) && cur.Weekday() == time.Monday {
			break
		}
	}

	lang := string(u.Locale)
	return &calendarView{
		MonthLabel: first.Format("January 2006"),
		Prev:       first.AddDate(0, -1, 0).Format("2006-01"),
		Next:       next.Format("2006-01"),
		DOW: []string{
			i18n.T("dow.mon", lang), i18n.T("dow.tue", lang), i18n.T("dow.wed", lang),
			i18n.T("dow.thu", lang), i18n.T("dow.fri", lang), i18n.T("dow.sat", lang), i18n.T("dow.sun", lang),
		},
		Weeks: weeks,
	}
}

func sameDay(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func ptrT(t time.Time) *time.Time { return &t }

// ---- task mutations -------------------------------------------------------

func parseWebDue(s string, loc *time.Location) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func (h *Handler) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	loc := loadLoc(u.Timezone)
	due := parseWebDue(r.FormValue("due_at"), loc)
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" || due.IsZero() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	priority := models.PriorityNormal
	if p, err := models.ParsePriority(r.FormValue("priority")); err == nil {
		priority = p
	}
	t := &models.Task{
		UserID:      u.ID,
		Title:       title,
		Description: strings.TrimSpace(r.FormValue("description")),
		Priority:    priority,
		DueAt:       due,
	}
	if freq := r.FormValue("recurrence_freq"); freq != "" {
		if f, err := models.ParseRecurrenceFreq(freq); err == nil {
			t.RecurrenceFreq = &f
			t.Kind = models.KindRecurring
			t.RecurrenceInterval = 1
			if n, err := strconv.Atoi(r.FormValue("recurrence_interval")); err == nil && n >= 1 {
				t.RecurrenceInterval = n
			}
		}
	}
	if err := h.deps.Store.Tasks.Create(r.Context(), t); err == nil {
		h.deps.Hub.Publish(u.ID, "task.created", map[string]any{"id": t.ID, "title": t.Title})
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) handleEditTaskPage(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	loc := loadLoc(u.Timezone)
	now := time.Now().In(loc)
	t, err := h.deps.Store.Tasks.Get(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	pd := h.base(w, r, u, i18n.T("action.edit", string(u.Locale)), "tasks")
	selected := ""
	if t.RecurrenceFreq != nil {
		selected = string(*t.RecurrenceFreq)
	}
	pd.Freqs = freqOptions(string(u.Locale), selected)
	tv := h.toTaskView(t, loc, string(u.Locale), now)
	tv.DueLocal = t.DueAt.In(loc).Format("2006-01-02T15:04")
	pd.Groups = []dayGroup{{Tasks: []taskView{tv}}}
	h.render(w, "tasks/edit.html", pd)
}

func (h *Handler) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	loc := loadLoc(u.Timezone)
	t, err := h.deps.Store.Tasks.Get(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if title := strings.TrimSpace(r.FormValue("title")); title != "" {
		t.Title = title
	}
	t.Description = strings.TrimSpace(r.FormValue("description"))
	if p, err := models.ParsePriority(r.FormValue("priority")); err == nil {
		t.Priority = p
	}
	if due := parseWebDue(r.FormValue("due_at"), loc); !due.IsZero() {
		t.DueAt = due
	}
	if freq := r.FormValue("recurrence_freq"); freq != "" {
		if f, err := models.ParseRecurrenceFreq(freq); err == nil {
			t.RecurrenceFreq = &f
			t.Kind = models.KindRecurring
			t.RecurrenceInterval = 1
			if n, err := strconv.Atoi(r.FormValue("recurrence_interval")); err == nil && n >= 1 {
				t.RecurrenceInterval = n
			}
		}
	} else {
		t.RecurrenceFreq = nil
		t.RecurrenceByDay = ""
		t.RecurrenceByMonthDay = nil
		t.Kind = models.KindOneoff
	}
	if err := h.deps.Store.Tasks.Update(r.Context(), t); err == nil {
		h.deps.Hub.Publish(u.ID, "task.updated", map[string]any{"id": t.ID})
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) advance(loc *time.Location) func(*models.Task, time.Time) (*models.TaskCompletion, bool, error) {
	return func(t *models.Task, n time.Time) (*models.TaskCompletion, bool, error) {
		if !t.Recurring() {
			return nil, false, nil
		}
		rule := ruleFromTask(t)
		next := recurrence.NextOccurrence(rule, t.DueAt, t.DueAt, loc)
		for !next.IsZero() && next.Before(n) {
			next = recurrence.NextOccurrence(rule, t.DueAt, next, loc)
		}
		if next.IsZero() || (t.RecurrenceEndAt != nil && next.After(*t.RecurrenceEndAt)) {
			t.CompletedAt = &n
			t.RecurrenceFreq = nil
			t.RecurrenceByDay = ""
			t.RecurrenceByMonthDay = nil
			t.Kind = models.KindOneoff
			return nil, true, nil
		}
		t.DueAt = next
		t.LastNotifiedAt = nil
		return nil, false, nil
	}
}

func (h *Handler) handleCompleteTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	loc := loadLoc(u.Timezone)
	now := time.Now().In(loc)
	id := r.PathValue("id")
	t, _, _, err := h.deps.Store.Tasks.Complete(r.Context(), u.ID, id, time.Now().UTC(), h.advance(loc))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.deps.Hub.Publish(u.ID, "task.completed", map[string]any{"id": id})
	tv := h.toTaskView(t, loc, string(u.Locale), now)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl := fragmentTemplate("tasks/_row.html")
	_ = tmpl.ExecuteTemplate(w, "row", rowCtx{Task: tv, Lang: string(u.Locale), CSRF: csrfOrNew(w, r)})
}

func (h *Handler) handleSnoozeTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	loc := loadLoc(u.Timezone)
	t, err := h.deps.Store.Tasks.Get(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	now := time.Now()
	var until time.Time
	switch r.URL.Query().Get("preset") {
	case "1h":
		until = now.Add(time.Hour)
	case "3h":
		until = now.Add(3 * time.Hour)
	case "tomorrow":
		tl := now.In(loc).AddDate(0, 0, 1)
		until = time.Date(tl.Year(), tl.Month(), tl.Day(), 9, 0, 0, 0, loc)
	default:
		until = parseWebDue(r.FormValue("until"), loc)
	}
	if until.IsZero() {
		until = now.Add(time.Hour)
	}
	utc := until.UTC()
	t.SnoozedUntil = &utc
	if err := h.deps.Store.Tasks.Update(r.Context(), t); err == nil {
		h.deps.Hub.Publish(u.ID, "task.updated", map[string]any{"id": t.ID})
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	if err := h.deps.Store.Tasks.Delete(r.Context(), u.ID, id); err == nil {
		h.deps.Hub.Publish(u.ID, "task.deleted", map[string]any{"id": id})
	}
	w.WriteHeader(http.StatusOK)
}

type rowCtx struct {
	Task taskView
	Lang string
	CSRF string
}

func parseByDay(s string) []time.Weekday {
	if s == "" {
		return nil
	}
	m := map[string]time.Weekday{"MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday, "TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday, "SU": time.Sunday}
	var out []time.Weekday
	for _, p := range strings.Split(s, ",") {
		if wd, ok := m[strings.TrimSpace(p)]; ok {
			out = append(out, wd)
		}
	}
	return out
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
