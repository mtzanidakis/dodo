package tui_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/mtzanidakis/dodo/internal/api"
	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/clientconfig"
	"github.com/mtzanidakis/dodo/internal/config"
	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/tui"
	"github.com/mtzanidakis/dodo/internal/ws"
)

type stubTG struct{}

func (stubTG) ValidateToken(context.Context, string) (string, error)  { return "b", nil }
func (stubTG) SendTest(context.Context, string, string, string) error { return nil }
func (stubTG) SendReminder(context.Context, string, string, string, string, string) error {
	return nil
}
func (stubTG) StartForUser(context.Context, string) error { return nil }
func (stubTG) StopForUser(string) error                   { return nil }
func (stubTG) StartAll(context.Context) error             { return nil }
func (stubTG) StopAll()                                   {}

func newEnv(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	st := store.New(d)
	ctx := context.Background()
	hash, _ := auth.HashPassword("pass1234")
	u := &models.User{Email: "t@x.com", PasswordHash: hash, Timezone: "Europe/Athens"}
	if err := st.Users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	gen, _ := auth.GenerateAPIToken()
	if _, err := st.Tokens.Create(ctx, u.ID, "n", gen.Prefix, gen.Hash); err != nil {
		t.Fatalf("create token: %v", err)
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	srv, _ := api.NewServer(config.Config{EncryptionKey: key}, st, ws.NewHub(slog.Default()), stubTG{}, slog.Default(), "t")
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)
	return hs, gen.Full
}

func TestTUIClientListCreateComplete(t *testing.T) {
	hs, tok := newEnv(t)
	c := tui.NewClient(clientconfig.ClientConfig{URL: hs.URL, Token: tok})
	if err := c.Create("Pay rent", "2026-07-11T11:00:00Z", "high", "monthly bill"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := c.Create("Buy milk", "2026-07-11T08:00:00Z", "normal", ""); err != nil {
		t.Fatalf("create2: %v", err)
	}
	items, err := c.ListTasks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(items))
	}
	if err := c.Complete(items[0].ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	rest, _ := c.ListTasks()
	if len(rest) != 1 {
		t.Fatalf("expected 1 pending after complete, got %d", len(rest))
	}
	if err := c.Snooze(rest[0].ID, "2026-07-12T09:00:00Z"); err != nil {
		t.Fatalf("snooze: %v", err)
	}
	if err := c.Delete(rest[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	after, _ := c.ListTasks()
	if len(after) != 0 {
		t.Fatalf("expected 0 pending after delete, got %d", len(after))
	}
	if email, _ := c.Me(); email != "t@x.com" {
		t.Fatalf("me: %s", email)
	}
}

func TestTUIClientListFilter(t *testing.T) {
	hs, tok := newEnv(t)
	c := tui.NewClient(clientconfig.ClientConfig{URL: hs.URL, Token: tok})
	if err := c.Create("Task A", "2026-07-11T11:00:00Z", "high", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	items, err := c.ListTasks()
	if err != nil || len(items) != 1 {
		t.Fatalf("list pending: %v len=%d", err, len(items))
	}
	if err := c.Complete(items[0].ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	completed, err := c.ListTasksFilter("completed")
	if err != nil || len(completed) != 1 {
		t.Fatalf("list completed: %v len=%d", err, len(completed))
	}
	all, err := c.ListTasksFilter("all")
	if err != nil || len(all) != 1 {
		t.Fatalf("list all: %v len=%d", err, len(all))
	}
}
