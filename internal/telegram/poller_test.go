package telegram

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mtzanidakis/dodo/internal/crypto"
	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
)

func TestClientTimeoutExceedsLongPoll(t *testing.T) {
	c := New("x")
	if c.httpClient.Timeout <= LongPollSeconds*time.Second {
		t.Fatalf("client timeout %v must exceed the %ds long-poll, else idle polls abort", c.httpClient.Timeout, LongPollSeconds)
	}
}

// A poller started from an API request must keep running after that request's
// context is cancelled (which happens the moment the response is sent).
func TestPollerRunsDespiteCancelledCallerContext(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getUpdates") && calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":1,"message":{"message_id":1,"from":{"id":42},"chat":{"id":42,"type":"private"},"text":"/start"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer srv.Close()

	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = d.Close() }()
	st := store.New(d)
	c, err := crypto.New(make([]byte, 32))
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	ctx := context.Background()
	u := &models.User{Email: "x@y.com", PasswordHash: "h"}
	if err := st.Users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	enc, _ := c.Encrypt("bot-token")
	if err := st.Users.SetTelegramConfig(ctx, u.ID, enc, "42"); err != nil {
		t.Fatalf("set telegram: %v", err)
	}

	reg := NewRegistry(st, c).WithAPIBase(srv.URL)
	pollers := NewPollers(reg, st, slog.Default(), nil)
	got := make(chan Update, 1)
	pollers.onUpdate = func(_ string, upd Update) {
		select {
		case got <- upd:
		default:
		}
	}

	// Cancel BEFORE starting: the old bug bound the goroutine to this context,
	// so it would exit immediately and never poll.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_ = pollers.StartForUser(cancelled, u.ID)
	defer func() { _ = pollers.StopForUser(u.ID) }()

	select {
	case upd := <-got:
		if upd.Message == nil || upd.Message.Text != "/start" {
			t.Fatalf("unexpected update: %+v", upd)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("poller processed no updates; it is bound to the cancelled caller context")
	}
}
