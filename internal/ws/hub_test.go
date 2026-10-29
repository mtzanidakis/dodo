package ws

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
)

func newTestHub(t *testing.T, interval time.Duration) (*Hub, string) {
	t.Helper()
	h := NewHub(slog.Default())
	h.pingInterval = interval
	h.timeout = 500 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		cl := h.Subscribe("u1", c)
		defer h.Unsubscribe(cl)
		go cl.SendLoop()
		for {
			if _, _, err := c.Read(r.Context()); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return h, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func waitClients(t *testing.T, h *Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.mu.RLock()
		n := len(h.clients["u1"])
		h.mu.RUnlock()
		if n == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("subscribed clients: got %d, want %d", n, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func readEvent(t *testing.T, ctx context.Context, c *websocket.Conn) Event {
	t.Helper()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var evt Event
	if err := json.Unmarshal(data, &evt); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	return evt
}

func TestSendLoopSendsHeartbeat(t *testing.T) {
	t.Parallel()
	_, url := newTestHub(t, 20*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.CloseNow() }()
	// The client must keep reading for the protocol pings to be answered.
	if evt := readEvent(t, ctx, c); evt.Type != "ping" {
		t.Fatalf("first message: %+v", evt)
	}
	if evt := readEvent(t, ctx, c); evt.Type != "ping" {
		t.Fatalf("second message: %+v", evt)
	}
}

func TestPublishReachesSubscriber(t *testing.T) {
	t.Parallel()
	h, url := newTestHub(t, time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.CloseNow() }()
	waitClients(t, h, 1)
	h.Publish("u1", "task.created", map[string]string{"id": "t1"})
	if evt := readEvent(t, ctx, c); evt.Type != "task.created" {
		t.Fatalf("event: %+v", evt)
	}
}

func TestDeadClientIsUnsubscribed(t *testing.T) {
	t.Parallel()
	h, url := newTestHub(t, 20*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// Never reading means pings go unanswered, like a client that vanished.
	defer func() { _ = c.CloseNow() }()
	waitClients(t, h, 1)
	waitClients(t, h, 0)
}
