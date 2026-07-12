package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mtzanidakis/dodo/internal/api"
	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/cli"
	"github.com/mtzanidakis/dodo/internal/clientconfig"
	"github.com/mtzanidakis/dodo/internal/config"
	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/ws"
)

type cliEnv struct {
	server *httptest.Server
	token  string
	outBuf *bytes.Buffer
}

func newCLIEnv(t *testing.T) *cliEnv {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	st := store.New(d)
	ctx := context.Background()
	hash, _ := auth.HashPassword("pass1234")
	u := &models.User{Email: "agent@example.com", PasswordHash: hash, Timezone: "Europe/Athens", Locale: models.LocaleEn}
	if err := st.Users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	gen, _ := auth.GenerateAPIToken()
	if _, err := st.Tokens.Create(ctx, u.ID, "agent", gen.Prefix, gen.Hash); err != nil {
		t.Fatalf("create token: %v", err)
	}
	hub := ws.NewHub(slog.Default())
	srv, err := api.NewServer(testConfig(), st, hub, stubTG{}, slog.Default(), "test")
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	httpSrv := httptest.NewServer(srv.Handler())
	t.Cleanup(httpSrv.Close)

	outBuf := &bytes.Buffer{}
	return &cliEnv{server: httpSrv, token: gen.Full, outBuf: outBuf}
}

func testConfig() config.Config {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i)
	}
	return config.Config{EncryptionKey: b}
}

func (e *cliEnv) run(t *testing.T, args ...string) int {
	t.Helper()
	cfg := clientconfig.ClientConfig{URL: e.server.URL, Token: e.token, LogLevel: "info"}
	app := cli.New(cfg, false)
	app.Out = e.outBuf
	return app.Run(args)
}

type stubTG struct{}

func (stubTG) ValidateToken(context.Context, string) (string, error)  { return "bot", nil }
func (stubTG) SendTest(context.Context, string, string, string) error { return nil }
func (stubTG) SendReminder(context.Context, string, string, string, string, string) error {
	return nil
}
func (stubTG) StartForUser(context.Context, string) error { return nil }
func (stubTG) StopForUser(string) error                   { return nil }
func (stubTG) StartAll(context.Context) error             { return nil }
func (stubTG) StopAll()                                   {}

func TestCLIMe(t *testing.T) {
	e := newCLIEnv(t)
	code := e.run(t, "me")
	if code != 0 {
		t.Fatalf("me exit %d", code)
	}
	var u map[string]any
	json.Unmarshal(e.outBuf.Bytes(), &u)
	if u["email"] != "agent@example.com" {
		t.Fatalf("me output: %s", e.outBuf.String())
	}
}

func TestCLITasksLifecycle(t *testing.T) {
	e := newCLIEnv(t)
	due := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	code := e.run(t, "tasks", "create", "--title", "Pay bill", "--due", due, "--priority", "high")
	if code != 0 {
		t.Fatalf("create exit %d", code)
	}
	var created map[string]any
	json.Unmarshal(e.outBuf.Bytes(), &created)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("create output: %s", e.outBuf.String())
	}

	e.outBuf.Reset()
	code = e.run(t, "tasks", "list", "--filter", "pending")
	if code != 0 {
		t.Fatalf("list exit %d", code)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(e.outBuf.Bytes(), &list)
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 task, got %d", len(list.Items))
	}

	e.outBuf.Reset()
	code = e.run(t, "tasks", "complete", id)
	if code != 0 {
		t.Fatalf("complete exit %d", code)
	}
	if !strings.Contains(e.outBuf.String(), id) {
		t.Fatalf("complete output: %s", e.outBuf.String())
	}
}

func TestCLITasksListLocalizesTimezone(t *testing.T) {
	e := newCLIEnv(t)
	// A fixed UTC instant so the expected local rendering is deterministic.
	dueUTC := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	code := e.run(t, "tasks", "create", "--title", "TZ", "--due", dueUTC.Format(time.RFC3339))
	if code != 0 {
		t.Fatalf("create exit %d", code)
	}

	e.outBuf.Reset()
	code = e.run(t, "tasks", "list", "--filter", "pending")
	if code != 0 {
		t.Fatalf("list exit %d", code)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(e.outBuf.Bytes(), &list)
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 task, got %d: %s", len(list.Items), e.outBuf.String())
	}

	got, _ := list.Items[0]["due_at"].(string)
	// The user's profile timezone is Europe/Athens, so due_at should render in
	// that zone (a +HH:MM offset, not a "Z" UTC suffix) while denoting the same
	// instant.
	loc, err := time.LoadLocation("Europe/Athens")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	if want := dueUTC.In(loc).Format(time.RFC3339); got != want {
		t.Fatalf("due_at = %q, want %q", got, want)
	}
	if strings.HasSuffix(got, "Z") {
		t.Fatalf("due_at still UTC: %q", got)
	}
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil || !parsed.Equal(dueUTC) {
		t.Fatalf("due_at not the same instant: %q (err %v)", got, err)
	}
}

func TestCLITimezoneConfigOverride(t *testing.T) {
	e := newCLIEnv(t)
	dueUTC := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	if code := e.run(t, "tasks", "create", "--title", "TZ", "--due", dueUTC.Format(time.RFC3339)); code != 0 {
		t.Fatalf("create exit %d", code)
	}

	// An explicit config timezone wins over the profile (Europe/Athens).
	cfg := clientconfig.ClientConfig{URL: e.server.URL, Token: e.token, LogLevel: "info", Timezone: "UTC"}
	app := cli.New(cfg, false)
	buf := &bytes.Buffer{}
	app.Out = buf
	if code := app.Run([]string{"tasks", "list", "--filter", "pending"}); code != 0 {
		t.Fatalf("list exit %d", code)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(buf.Bytes(), &list)
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 task, got %d: %s", len(list.Items), buf.String())
	}
	if got, _ := list.Items[0]["due_at"].(string); got != dueUTC.Format(time.RFC3339) {
		t.Fatalf("due_at = %q, want UTC %q", got, dueUTC.Format(time.RFC3339))
	}
}

func TestCLITokensCreate(t *testing.T) {
	e := newCLIEnv(t)
	e.outBuf.Reset()
	code := e.run(t, "tokens", "create", "--name", "ci")
	if code != 0 {
		t.Fatalf("tokens create exit %d", code)
	}
	var tok map[string]any
	json.Unmarshal(e.outBuf.Bytes(), &tok)
	if strings.TrimSpace(tok["token"].(string)) == "" {
		t.Fatalf("expected token in output: %s", e.outBuf.String())
	}
}

func TestCLIMissingAuth(t *testing.T) {
	cfg := clientconfig.ClientConfig{URL: "http://localhost", LogLevel: "info"}
	app := cli.New(cfg, false)
	app.Out = &bytes.Buffer{}
	app.Err = &bytes.Buffer{}
	code := app.Run([]string{"tasks", "list"})
	if code != cli.ExitAuth {
		t.Fatalf("expected exit %d, got %d", cli.ExitAuth, code)
	}
}

var _ io.Reader = strings.NewReader("")
