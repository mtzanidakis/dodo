package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/i18n"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/recurrence"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/ws"
)

type Service struct {
	pollers *Pollers
	store   *store.Store
	hub     *ws.Hub
	logger  *slog.Logger
	now     func() time.Time
}

func NewService(pollers *Pollers, st *store.Store, hub *ws.Hub, logger *slog.Logger) *Service {
	s := &Service{pollers: pollers, store: st, hub: hub, logger: logger, now: time.Now}
	pollers.onUpdate = s.handleUpdate
	return s
}

func (s *Service) ValidateToken(ctx context.Context, botToken string) (string, error) {
	c := New(botToken)
	if s.pollers.apiBase != "" {
		c = c.WithAPIBase(s.pollers.apiBase)
	}
	bot, err := c.GetMe(ctx)
	if err != nil {
		return "", err
	}
	return bot.Username, nil
}

func (s *Service) SendTest(ctx context.Context, userID, chatID, text string) error {
	client, err := s.pollers.registry.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}
	return client.SendMessage(ctx, chatID, text)
}

// SendReminder sends a due-task reminder carrying an inline "Complete" button
// whose callback data completes the task straight from Telegram.
func (s *Service) SendReminder(ctx context.Context, userID, chatID, text, taskID, buttonLabel string) error {
	client, err := s.pollers.registry.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}
	return client.SendMessageWithMarkup(ctx, chatID, text, completeKeyboard(taskID, buttonLabel))
}

func completeKeyboard(taskID, label string) *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{{
			{Text: label, CallbackData: "complete:" + taskID},
		}},
	}
}

func (s *Service) StartForUser(ctx context.Context, userID string) error {
	return s.pollers.StartForUser(ctx, userID)
}
func (s *Service) StopForUser(userID string) error    { return s.pollers.StopForUser(userID) }
func (s *Service) StartAll(ctx context.Context) error { return s.pollers.StartAll(ctx) }
func (s *Service) StopAll()                           { s.pollers.StopAll() }

func (s *Service) handleUpdate(userID string, u Update) {
	ctx := context.Background()
	encToken, allowed, _, _, _, err := s.store.Users.GetTelegramConfig(ctx, userID)
	if err != nil || encToken == "" {
		return
	}
	lang := s.userLang(userID)
	if u.Message != nil && u.Message.From != nil {
		fromID := strconv.FormatInt(u.Message.From.ID, 10)
		if !isAllowed(fromID, allowed) {
			s.reply(ctx, userID, u.Message.Chat.IDString(), i18n.T("telegram.unauthorized", lang))
			return
		}
		text := strings.TrimSpace(u.Message.Text)
		switch text {
		case "/start", "":
			chatID := u.Message.Chat.IDString()
			chatUserID := fromID
			_ = s.store.Users.SetTelegramChatID(ctx, userID, chatID, chatUserID)
			s.reply(ctx, userID, chatID, i18n.T("telegram.linked", lang))
			s.hub.Publish(userID, "telegram.linked", map[string]any{"chat_id": chatID})
		default:
			s.reply(ctx, userID, u.Message.Chat.IDString(), i18n.T("telegram.help", lang))
		}
		return
	}
	if u.CallbackQuery != nil {
		s.handleCallback(ctx, userID, u.CallbackQuery)
	}
}

func (s *Service) userLang(userID string) string {
	u, err := s.store.Users.GetByID(context.Background(), userID)
	if err != nil || u == nil {
		return "en"
	}
	return string(u.Locale)
}

func (s *Service) handleCallback(ctx context.Context, userID string, cq *CallbackQuery) {
	if !strings.HasPrefix(cq.Data, "complete:") {
		_ = s.answer(ctx, userID, cq.ID, "Unknown action")
		return
	}
	taskID := strings.TrimPrefix(cq.Data, "complete:")
	fromID := strconv.FormatInt(cq.From.ID, 10)
	_, allowed, _, _, _, _ := s.store.Users.GetTelegramConfig(ctx, userID)
	if !isAllowed(fromID, allowed) {
		_ = s.answer(ctx, userID, cq.ID, "Not authorized")
		return
	}
	loc := s.userLocUnsafe(userID)
	now := s.now().UTC()
	t, _, _, err := s.store.Tasks.Complete(ctx, userID, taskID, now, func(t *models.Task, n time.Time) (*models.TaskCompletion, bool, error) {
		if !t.Recurring() {
			return nil, false, nil
		}
		rule := recurrence.Rule{Freq: *t.RecurrenceFreq, Interval: t.RecurrenceInterval, ByDay: parseByDay(t.RecurrenceByDay), ByMonthDay: orZero(t.RecurrenceByMonthDay)}
		if t.RecurrenceEndAt != nil {
			rule.EndAt = *t.RecurrenceEndAt
		}
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
		if errors.Is(err, models.ErrNotFound) {
			_ = s.answer(ctx, userID, cq.ID, i18n.T("telegram.not_found_toast", s.userLang(userID)))
			return
		}
		_ = s.answer(ctx, userID, cq.ID, "Error completing task")
		return
	}
	_ = s.answer(ctx, userID, cq.ID, i18n.T("telegram.completed_toast", s.userLang(userID)))
	s.hub.Publish(userID, "task.completed", map[string]any{"id": taskID})
	chatID := cq.Message.Chat.IDString()
	_ = s.editMessage(ctx, userID, chatID, cq.Message.MessageID, fmt.Sprintf("%s (completed)", t.Title))
}

func (s *Service) reply(ctx context.Context, userID, chatID, text string) {
	client, err := s.pollers.registry.GetOrCreate(ctx, userID)
	if err != nil {
		return
	}
	_ = client.SendMessage(ctx, chatID, text)
}

func (s *Service) answer(ctx context.Context, userID, id, text string) error {
	client, err := s.pollers.registry.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}
	return client.AnswerCallbackQuery(ctx, id, text)
}

func (s *Service) editMessage(ctx context.Context, userID, chatID string, messageID int64, text string) error {
	client, err := s.pollers.registry.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}
	return client.EditMessageText(ctx, chatID, messageID, text)
}

func (s *Service) userLocUnsafe(userID string) *time.Location {
	u, err := s.store.Users.GetByID(context.Background(), userID)
	if err != nil || u == nil {
		return time.UTC
	}
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func isAllowed(fromID, allowed string) bool {
	if allowed == "" {
		return false
	}
	for _, p := range strings.Split(allowed, ",") {
		if strings.TrimSpace(p) == fromID {
			return true
		}
	}
	return false
}

func (c Chat) IDString() string { return strconv.FormatInt(c.ID, 10) }

func parseByDay(s string) []time.Weekday {
	var out []time.Weekday
	for _, p := range strings.Split(s, ",") {
		switch strings.TrimSpace(p) {
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

func orZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
