package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/recurrence"
)

type createTaskRequest struct {
	Title                string  `json:"title"`
	Description          string  `json:"description"`
	Priority             string  `json:"priority"`
	DueAt                string  `json:"due_at"`
	DurationMinutes      int     `json:"duration_minutes"`
	RecurrenceFreq       *string `json:"recurrence_freq"`
	RecurrenceInterval   int     `json:"recurrence_interval"`
	RecurrenceByDay      *string `json:"recurrence_by_day"`
	RecurrenceByMonthDay *int    `json:"recurrence_by_month_day"`
	RecurrenceEndAt      *string `json:"recurrence_end_at"`
}

func (s *Server) advanceRecurring(t *models.Task, now time.Time, loc *time.Location) (*models.TaskCompletion, bool, error) {
	rule := recurrence.Rule{
		Freq:       *t.RecurrenceFreq,
		Interval:   t.RecurrenceInterval,
		ByDay:      parseByDay(t.RecurrenceByDay),
		ByMonthDay: orZero(t.RecurrenceByMonthDay),
	}
	if t.RecurrenceEndAt != nil {
		rule.EndAt = *t.RecurrenceEndAt
	}
	next := recurrence.NextOccurrence(rule, t.DueAt, t.DueAt, loc)
	if next.IsZero() {
		finishRecurring(t, now)
		return nil, true, nil
	}
	for next.Before(now) && !next.IsZero() {
		fastNext := recurrence.NextOccurrence(rule, t.DueAt, next, loc)
		if fastNext.IsZero() {
			next = fastNext
			break
		}
		next = fastNext
	}
	if next.IsZero() || (t.RecurrenceEndAt != nil && next.After(*t.RecurrenceEndAt)) {
		finishRecurring(t, now)
		return nil, true, nil
	}
	t.DueAt = next
	t.LastNotifiedAt = nil
	return nil, false, nil
}

func finishRecurring(t *models.Task, now time.Time) {
	t.CompletedAt = &now
	t.RecurrenceFreq = nil
	t.RecurrenceByDay = ""
	t.RecurrenceByMonthDay = nil
	t.RecurrenceEndAt = nil
	t.Kind = models.KindOneoff
}

func orZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func parseByDay(s string) []time.Weekday {
	if s == "" {
		return nil
	}
	var out []time.Weekday
	for _, part := range strings.Split(s, ",") {
		switch strings.TrimSpace(part) {
		case "MO":
			out = append(out, time.Monday)
		case "TU":
			out = append(out, time.Tuesday)
		case "WE":
			out = append(out, time.Wednesday)
		case "TH":
			out = append(out, time.Thursday)
		case "FR":
			out = append(out, time.Friday)
		case "SA":
			out = append(out, time.Saturday)
		case "SU":
			out = append(out, time.Sunday)
		}
	}
	return out
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	loc := userLoc(u)
	filter := models.TaskFilter{
		View:   r.URL.Query().Get("view"),
		Filter: r.URL.Query().Get("filter"),
	}
	if from := r.URL.Query().Get("from"); from != "" {
		filter.From = ptrTime(parseDueAt(from, loc))
	}
	if to := r.URL.Query().Get("to"); to != "" {
		filter.To = ptrTime(parseDueAt(to, loc))
	}
	if p := r.URL.Query().Get("priority"); p != "" {
		pr := models.Priority(p)
		filter.Priority = &pr
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			filter.Limit = n
		}
	}
	filter.Cursor = r.URL.Query().Get("cursor")

	tasks, cursor, err := s.store.Tasks.List(r.Context(), u.ID, filter)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]taskDTO, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, toTaskDTO(t))
	}
	resp := listEnvelope[taskDTO]{Items: items}
	if cursor != "" {
		c := cursor
		resp.Cursor = &c
	}
	writeJSON(w, http.StatusOK, resp)
}

func userLoc(u *models.User) *time.Location {
	if u == nil {
		return time.UTC
	}
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

var _ = errors.New

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var req createTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Title == "" {
		writeError(w, errors.Join(models.ErrValidation, errors.New("title required")))
		return
	}
	priority := models.PriorityNormal
	if req.Priority != "" {
		p, err := models.ParsePriority(req.Priority)
		if err != nil {
			writeError(w, err)
			return
		}
		priority = p
	}
	if req.DueAt == "" {
		writeError(w, errors.Join(models.ErrValidation, errors.New("due_at required")))
		return
	}
	loc, _ := time.LoadLocation(u.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	dueAt := parseDueAt(req.DueAt, loc)

	t := &models.Task{
		UserID:          u.ID,
		Title:           req.Title,
		Description:     req.Description,
		Priority:        priority,
		DueAt:           dueAt,
		DurationMinutes: req.DurationMinutes,
	}
	if req.RecurrenceFreq != nil && *req.RecurrenceFreq != "" {
		f, err := models.ParseRecurrenceFreq(*req.RecurrenceFreq)
		if err != nil {
			writeError(w, err)
			return
		}
		t.RecurrenceFreq = &f
		t.RecurrenceInterval = req.RecurrenceInterval
		if t.RecurrenceInterval < 1 {
			t.RecurrenceInterval = 1
		}
		if req.RecurrenceByDay != nil {
			t.RecurrenceByDay = *req.RecurrenceByDay
		}
		if req.RecurrenceByMonthDay != nil {
			t.RecurrenceByMonthDay = req.RecurrenceByMonthDay
		}
		if req.RecurrenceEndAt != nil && *req.RecurrenceEndAt != "" {
			end, err := parseDueAtSafe(*req.RecurrenceEndAt)
			if err != nil {
				writeError(w, err)
				return
			}
			t.RecurrenceEndAt = ptrTime(end)
		}
	}

	if err := s.store.Tasks.Create(r.Context(), t); err != nil {
		writeError(w, err)
		return
	}
	s.hub.Publish(u.ID, "task.created", toTaskDTO(t))
	s.audit(r, "task.create", "task", t.ID, nil)
	writeJSON(w, http.StatusCreated, toTaskDTO(t))
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	t, err := s.store.Tasks.Get(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTaskDTO(t))
}

type updateTaskRequest struct {
	Title                *string `json:"title"`
	Description          *string `json:"description"`
	Priority             *string `json:"priority"`
	DueAt                *string `json:"due_at"`
	DurationMinutes      *int    `json:"duration_minutes"`
	RecurrenceFreq       *string `json:"recurrence_freq"`
	RecurrenceInterval   *int    `json:"recurrence_interval"`
	RecurrenceByDay      *string `json:"recurrence_by_day"`
	RecurrenceByMonthDay *int    `json:"recurrence_by_month_day"`
	RecurrenceEndAt      *string `json:"recurrence_end_at"`
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	t, err := s.store.Tasks.Get(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if t.Completed() {
		writeError(w, errors.Join(models.ErrValidation, errors.New("cannot update completed task")))
		return
	}
	var req updateTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	loc, _ := time.LoadLocation(u.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	recalc := r.URL.Query().Get("recalculate") == "1"

	if req.Title != nil {
		t.Title = *req.Title
	}
	if req.Description != nil {
		t.Description = *req.Description
	}
	if req.Priority != nil {
		p, err := models.ParsePriority(*req.Priority)
		if err != nil {
			writeError(w, err)
			return
		}
		t.Priority = p
	}
	if req.DueAt != nil {
		t.DueAt = parseDueAt(*req.DueAt, loc)
	}
	if req.DurationMinutes != nil {
		t.DurationMinutes = *req.DurationMinutes
	}
	if req.RecurrenceFreq != nil {
		if *req.RecurrenceFreq == "" {
			t.RecurrenceFreq = nil
			t.RecurrenceByDay = ""
			t.RecurrenceByMonthDay = nil
			t.RecurrenceEndAt = nil
			t.Kind = models.KindOneoff
		} else {
			f, err := models.ParseRecurrenceFreq(*req.RecurrenceFreq)
			if err != nil {
				writeError(w, err)
				return
			}
			t.RecurrenceFreq = &f
			t.Kind = models.KindRecurring
		}
	}
	if req.RecurrenceInterval != nil {
		t.RecurrenceInterval = *req.RecurrenceInterval
		if t.RecurrenceInterval < 1 {
			t.RecurrenceInterval = 1
		}
	}
	if req.RecurrenceByDay != nil {
		t.RecurrenceByDay = *req.RecurrenceByDay
	}
	if req.RecurrenceByMonthDay != nil {
		t.RecurrenceByMonthDay = req.RecurrenceByMonthDay
	}
	if req.RecurrenceEndAt != nil {
		if *req.RecurrenceEndAt == "" {
			t.RecurrenceEndAt = nil
		} else {
			end, err := parseDueAtSafe(*req.RecurrenceEndAt)
			if err != nil {
				writeError(w, err)
				return
			}
			t.RecurrenceEndAt = ptrTime(end)
		}
	}
	if recalc && t.Recurring() {
		rule := recurrence.Rule{Freq: *t.RecurrenceFreq, Interval: t.RecurrenceInterval, ByDay: parseByDay(t.RecurrenceByDay), ByMonthDay: orZero(t.RecurrenceByMonthDay)}
		if t.RecurrenceEndAt != nil {
			rule.EndAt = *t.RecurrenceEndAt
		}
		next := recurrence.NextOccurrence(rule, t.DueAt, t.DueAt, loc)
		if !next.IsZero() {
			t.DueAt = next
		}
	}

	if err := s.store.Tasks.Update(r.Context(), t); err != nil {
		writeError(w, err)
		return
	}
	s.hub.Publish(u.ID, "task.updated", toTaskDTO(t))
	s.audit(r, "task.update", "task", t.ID, nil)
	writeJSON(w, http.StatusOK, toTaskDTO(t))
}

func (s *Server) handleCompleteTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	loc := userLoc(u)
	advance := func(t *models.Task, now time.Time) (*models.TaskCompletion, bool, error) {
		return s.advanceRecurring(t, now, loc)
	}
	t, compl, _, err := s.store.Tasks.Complete(r.Context(), u.ID, id, s.Now(), advance)
	if err != nil {
		writeError(w, err)
		return
	}
	resp := map[string]any{"task": toTaskDTO(t)}
	if compl != nil {
		resp["completion"] = toCompletionDTO(compl)
	}
	s.hub.Publish(u.ID, "task.completed", toTaskDTO(t))
	s.audit(r, "task.complete", "task", id, nil)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSnoozeTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	t, err := s.store.Tasks.Get(r.Context(), u.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		Until string `json:"until"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	loc, _ := time.LoadLocation(u.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	until := parseDueAt(req.Until, loc)
	t.SnoozedUntil = &until
	if err := s.store.Tasks.Update(r.Context(), t); err != nil {
		writeError(w, err)
		return
	}
	s.hub.Publish(u.ID, "task.updated", toTaskDTO(t))
	writeJSON(w, http.StatusOK, toTaskDTO(t))
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	if err := s.store.Tasks.Delete(r.Context(), u.ID, id); err != nil {
		writeError(w, err)
		return
	}
	s.hub.Publish(u.ID, "task.deleted", map[string]any{"id": id})
	s.audit(r, "task.delete", "task", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func parseDueAt(s string, loc *time.Location) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.ParseInLocation(time.RFC3339Nano, s, loc); err == nil {
		return t.UTC()
	}
	if t, err := time.ParseInLocation(time.RFC3339, s, loc); err == nil {
		return t.UTC()
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", s, loc); err == nil {
		return t.UTC()
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, loc); err == nil {
		return t.UTC()
	}
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func parseDueAtSafe(s string) (time.Time, error) {
	t := parseDueAt(s, time.UTC)
	if t.IsZero() {
		return time.Time{}, errors.Join(models.ErrValidation, errors.New("invalid datetime"))
	}
	return t, nil
}

var _ = strconv.Atoi
