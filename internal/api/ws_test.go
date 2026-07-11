package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/mtzanidakis/dodo/internal/auth"
)

// The /ws endpoint must upgrade through the logging middleware's ResponseWriter
// wrapper; without Unwrap the hijack fails and Accept returns 501.
func TestWebSocketUpgradesAndDelivers(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": {auth.SessionCookie + "=" + ts.cookieA}},
	})
	if err != nil {
		t.Fatalf("ws dial failed (upgrade rejected): %v", err)
	}
	defer func() { _ = c.Close(websocket.StatusNormalClosure, "") }()

	// A published event reaches the connected client.
	go func() {
		time.Sleep(100 * time.Millisecond)
		ts.hub.Publish(ts.userA.ID, "task.created", map[string]any{"id": "abc"})
	}()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	if !strings.Contains(string(data), "task.created") {
		t.Fatalf("unexpected ws payload: %s", data)
	}
}
