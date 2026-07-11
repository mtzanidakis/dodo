package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/mtzanidakis/dodo/internal/i18n"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/ws"
)

type Scheduler struct {
	interval time.Duration
	store    *store.Store
	hub      *ws.Hub
	telegram TelegramService
	logger   *slog.Logger
	now      func() time.Time
	stop     chan struct{}
}

func newScheduler(interval time.Duration, st *store.Store, hub *ws.Hub, tg TelegramService, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		interval: interval, store: st, hub: hub, telegram: tg, logger: logger,
		now: time.Now, stop: make(chan struct{}),
	}
}

func (sc *Scheduler) Start(ctx context.Context) error {
	go sc.loop(ctx)
	return nil
}

func (sc *Scheduler) Stop() {
	select {
	case <-sc.stop:
	default:
		close(sc.stop)
	}
}

func (sc *Scheduler) loop(ctx context.Context) {
	ticker := time.NewTicker(sc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sc.stop:
			return
		case <-ticker.C:
			sc.tick(ctx)
		}
	}
}

func (sc *Scheduler) tick(ctx context.Context) {
	now := sc.now().UTC()
	start := time.Now()
	tasks, err := sc.store.Tasks.ListDue(ctx, now, 500)
	if err != nil {
		sc.logger.Warn("scheduler list-due error", "error", err)
		return
	}
	for _, t := range tasks {
		if !shouldNotify(t, now) {
			continue
		}
		sc.dispatch(ctx, t, now)
	}
	_ = sc.store.Sessions.DeleteExpired(ctx, now)
	sc.logger.Debug("scheduler tick", "duration_ms", time.Since(start).Milliseconds(), "due", len(tasks))
}

func shouldNotify(t *models.Task, now time.Time) bool {
	if t.LastNotifiedAt == nil {
		return true
	}
	interval := models.PriorityReminderInterval(t.Priority)
	return now.Sub(*t.LastNotifiedAt) >= interval
}

func (sc *Scheduler) dispatch(ctx context.Context, t *models.Task, now time.Time) {
	u, err := sc.store.Users.GetByID(ctx, t.UserID)
	if err != nil || u == nil {
		return
	}
	sc.hub.Publish(t.UserID, "task.due", map[string]any{"id": t.ID, "title": t.Title})
	if u.TelegramEnabled() && u.TelegramChatID != "" {
		text := renderReminder(u, t, now)
		label := i18n.T("action.complete", string(u.Locale))
		if err := sc.telegram.SendReminder(ctx, u.ID, u.TelegramChatID, text, t.ID, label); err != nil {
			sc.logger.Warn("telegram reminder send failed", "user_id", t.UserID, "task_id", t.ID, "error", err)
		}
	}
	_ = sc.store.Tasks.SetLastNotified(ctx, t.ID, now)
}

func renderReminder(u *models.User, t *models.Task, now time.Time) string {
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil {
		loc = time.UTC
	}
	due := t.DueAt.In(loc).Format("15:04")
	prio := i18n.T("priority."+string(t.Priority), string(u.Locale))
	return i18n.T("reminder.summary", string(u.Locale), t.Title, due, prio)
}

var _ TelegramService
