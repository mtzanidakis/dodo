package tui

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mtzanidakis/dodo/internal/api"
	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/clientconfig"
	"github.com/mtzanidakis/dodo/internal/config"
	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/ws"

	tea "github.com/charmbracelet/bubbletea"
)

type noTG struct{}

func (noTG) ValidateToken(context.Context, string) (string, error)                      { return "b", nil }
func (noTG) SendTest(context.Context, string, string, string) error                     { return nil }
func (noTG) SendReminder(context.Context, string, string, string, string, string) error { return nil }
func (noTG) StartForUser(context.Context, string) error                                 { return nil }
func (noTG) StopForUser(string) error                                                   { return nil }
func (noTG) StartAll(context.Context) error                                             { return nil }
func (noTG) StopAll()                                                                   {}

// TestModelAutoLoadMore verifies that scrolling to the bottom of the list
// pulls and appends the next page until the timeline is exhausted.
func TestModelAutoLoadMore(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	st := store.New(d)
	ctx := context.Background()
	hash, _ := auth.HashPassword("pass1234")
	u := &models.User{Email: "t@x.com", PasswordHash: hash, Timezone: "Europe/Athens"}
	if err := st.Users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	gen, _ := auth.GenerateAPIToken()
	if _, err := st.Tokens.Create(ctx, u.ID, "n", gen.Prefix, gen.Hash); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	srv, _ := api.NewServer(config.Config{EncryptionKey: key}, st, ws.NewHub(slog.Default()), noTG{}, slog.Default(), "t")
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)

	c := NewClient(clientconfig.ClientConfig{URL: hs.URL, Token: gen.Full})

	// 120 pending tasks -> 3 pages of 50/50/20.
	const total = 120
	base := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	for i := 0; i < total; i++ {
		due := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		if err := c.Create(fmt.Sprintf("task %03d", i), due, "normal", ""); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	m := initialModel(c)
	if len(m.items) != 25 || m.nextCursor == "" {
		t.Fatalf("first page: items=%d cursor=%q", len(m.items), m.nextCursor)
	}

	// Drive "down" repeatedly; each time the cursor hits the last row a new
	// page must be appended, until everything is loaded.
	var mdl tea.Model = m
	for i := 0; i < total+10; i++ {
		mdl, _ = mdl.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	got := mdl.(model)
	if len(got.items) != total {
		t.Fatalf("after scrolling: want %d items, got %d", total, len(got.items))
	}
	if got.nextCursor != "" {
		t.Fatalf("cursor should be empty once exhausted, got %q", got.nextCursor)
	}
	// No duplicates across the appended pages.
	seen := map[string]bool{}
	for _, it := range got.items {
		if seen[it.ID] {
			t.Fatalf("duplicate task %s after paging", it.ID)
		}
		seen[it.ID] = true
	}
}
