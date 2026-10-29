package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/ws"
)

func TestSchedulerPublishesTaskDue(t *testing.T) {
	t.Parallel()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	st := store.New(d)
	ctx := context.Background()
	u := &models.User{Email: "s@example.com", PasswordHash: "x", Timezone: "UTC", Locale: models.LocaleEn}
	if err := st.Users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Date(2026, 10, 29, 17, 0, 0, 0, time.UTC)
	task := &models.Task{UserID: u.ID, Title: "Pay bill", Priority: models.PriorityNormal, DueAt: now.Add(-time.Minute)}
	if err := st.Tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	hub := ws.NewHub(slog.Default())
	subscribed := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		cl := hub.Subscribe(u.ID, c)
		defer hub.Unsubscribe(cl)
		go cl.SendLoop()
		close(subscribed)
		for {
			if _, _, err := c.Read(r.Context()); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(rctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.CloseNow() }()
	<-subscribed

	sc := newScheduler(time.Minute, st, hub, nil, slog.Default())
	sc.now = func() time.Time { return now }
	sc.tick(ctx)

	_, data, err := c.Read(rctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var evt struct {
		Type    string            `json:"type"`
		Payload map[string]string `json:"payload"`
	}
	if err := json.Unmarshal(data, &evt); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	want := map[string]string{"id": task.ID, "title": "Pay bill", "notified_at": "2026-10-29T17:00:00Z"}
	if evt.Type != "task.due" || len(evt.Payload) != len(want) {
		t.Fatalf("event: %s", data)
	}
	for k, v := range want {
		if evt.Payload[k] != v {
			t.Fatalf("payload %s = %q, want %q (%s)", k, evt.Payload[k], v, data)
		}
	}
}
