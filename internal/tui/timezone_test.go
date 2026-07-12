package tui

import (
	"context"
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
)

// tzEnv spins up an API server whose sole user has the Europe/Athens profile
// timezone and returns a bearer token for it.
func tzEnv(t *testing.T) (string, string) {
	t.Helper()
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
	return hs.URL, gen.Full
}

func TestModelUsesProfileTimezone(t *testing.T) {
	url, tok := tzEnv(t)
	m := initialModel(NewClient(clientconfig.ClientConfig{URL: url, Token: tok}))

	want, err := time.LoadLocation("Europe/Athens")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	if m.loc == nil || m.loc.String() != want.String() {
		t.Fatalf("loc = %v, want %v", m.loc, want)
	}
	// A fixed UTC instant renders in the profile zone, not host local/UTC.
	dueUTC := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	got := m.fmtLocal(dueUTC.Format(time.RFC3339))
	if exp := dueUTC.In(want).Format("2006-01-02 15:04"); got != exp {
		t.Fatalf("fmtLocal = %q, want %q", got, exp)
	}
}

// TestFormValidateUsesZone verifies a bare "2006-01-02 15:04" due entered in
// the TUI is interpreted in the display zone, not host local/UTC.
func TestFormValidateUsesZone(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Athens")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	f := newTaskForm()
	f.title.setValue("x")
	f.due.setValue("2026-07-11 09:00")
	_, due, _, _, err := f.validate(loc)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	got, err := time.Parse(time.RFC3339, due)
	if err != nil {
		t.Fatalf("parse due: %v", err)
	}
	if want := time.Date(2026, 7, 11, 9, 0, 0, 0, loc).UTC(); !got.Equal(want) {
		t.Fatalf("due = %v, want %v", got.UTC(), want)
	}
}

func TestModelTimezoneConfigOverride(t *testing.T) {
	url, tok := tzEnv(t)
	// An explicit config timezone wins over the profile (Europe/Athens).
	m := initialModel(NewClient(clientconfig.ClientConfig{URL: url, Token: tok, Timezone: "UTC"}))
	if m.loc == nil || m.loc.String() != "UTC" {
		t.Fatalf("loc = %v, want UTC", m.loc)
	}
	dueUTC := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	if got := m.fmtLocal(dueUTC.Format(time.RFC3339)); got != "2026-07-12 09:00" {
		t.Fatalf("fmtLocal = %q, want UTC rendering", got)
	}
}
