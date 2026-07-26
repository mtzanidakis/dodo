package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestCLIAcceptsConfiguredDateFormat(t *testing.T) {
	e := newCLIEnv(t)
	// The config pattern is what the user types; the API contract on the wire
	// stays RFC3339.
	cfg := clientconfig.ClientConfig{
		URL: e.server.URL, Token: e.token, LogLevel: "info",
		Timezone: "Europe/Athens", DateFormat: "DD/MM/YYYY",
	}
	app := cli.New(cfg, false)
	buf := &bytes.Buffer{}
	app.Out = buf
	if code := app.Run([]string{"tasks", "create", "--title", "Typed", "--due", "12/07/2026 09:00"}); code != 0 {
		t.Fatalf("create exit %d: %s", code, buf.String())
	}

	var created map[string]any
	if err := json.Unmarshal(buf.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v (%s)", err, buf.String())
	}
	got, _ := created["due_at"].(string)
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("due_at %q is not RFC3339: %v", got, err)
	}
	loc, err := time.LoadLocation("Europe/Athens")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	if want := time.Date(2026, 7, 12, 9, 0, 0, 0, loc); !parsed.Equal(want) {
		t.Fatalf("due_at = %v, want %v", parsed, want)
	}
}

func TestCLIInitRejectsInvalidDateFormat(t *testing.T) {
	e := newCLIEnv(t)
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := clientconfig.ClientConfig{URL: e.server.URL, Token: e.token, LogLevel: "info"}
	app := cli.New(cfg, false)
	errBuf := &bytes.Buffer{}
	app.Err = errBuf
	app.Out = &bytes.Buffer{}
	if code := app.Run([]string{"init", "--date-format", "nonsense", "--config", path}); code != cli.ExitUsage {
		t.Fatalf("init exit %d, want %d", code, cli.ExitUsage)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("config must not be written for an invalid pattern")
	}
}

// prettyRun runs a command with --pretty enabled and the given date pattern,
// returning stdout and stderr separately.
func prettyRun(t *testing.T, e *cliEnv, dateFormat string, args ...string) (string, string, int) {
	t.Helper()
	cfg := clientconfig.ClientConfig{
		URL: e.server.URL, Token: e.token, LogLevel: "info",
		Timezone: "Europe/Athens", DateFormat: dateFormat,
	}
	app := cli.New(cfg, true)
	out, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
	app.Out, app.Err = out, errBuf
	code := app.Run(args)
	return out.String(), errBuf.String(), code
}

func TestPrettyTaskListUsesDateFormatAndColumns(t *testing.T) {
	e := newCLIEnv(t)
	cfg := clientconfig.ClientConfig{URL: e.server.URL, Token: e.token, LogLevel: "info", Timezone: "Europe/Athens"}
	app := cli.New(cfg, false)
	app.Out = &bytes.Buffer{}
	if code := app.Run([]string{"tasks", "create", "--title", "Pay rent", "--due", "2026-08-05T17:30:00+03:00", "--priority", "high"}); code != 0 {
		t.Fatalf("create exit %d", code)
	}

	out, _, code := prettyRun(t, e, "DD/MM/YYYY", "tasks", "list", "--filter", "pending")
	if code != 0 {
		t.Fatalf("list exit %d: %s", code, out)
	}
	for _, want := range []string{"DUE", "STATUS", "PRIORITY", "TITLE", "05/08/2026 17:30", "Pay rent", "high"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty list missing %q:\n%s", want, out)
		}
	}
	// The layout is for humans; it must not be JSON.
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("pretty list still emitted JSON:\n%s", out)
	}
}

func TestPrettyDoesNotChangeJSONOutput(t *testing.T) {
	e := newCLIEnv(t)
	cfg := clientconfig.ClientConfig{URL: e.server.URL, Token: e.token, LogLevel: "info", Timezone: "Europe/Athens"}
	app := cli.New(cfg, false)
	buf := &bytes.Buffer{}
	app.Out = buf
	if code := app.Run([]string{"tasks", "create", "--title", "Agent task", "--due", "2026-08-05T17:30:00+03:00"}); code != 0 {
		t.Fatalf("create exit %d", code)
	}
	buf.Reset()
	if code := app.Run([]string{"tasks", "list", "--filter", "pending"}); code != 0 {
		t.Fatalf("list exit %d", code)
	}
	// Without --pretty the agent contract is unchanged: parseable JSON with
	// RFC3339 timestamps.
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &list); err != nil {
		t.Fatalf("default output must stay JSON: %v (%s)", err, buf.String())
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 task, got %d", len(list.Items))
	}
	if _, err := time.Parse(time.RFC3339, list.Items[0]["due_at"].(string)); err != nil {
		t.Fatalf("due_at must stay RFC3339: %v", err)
	}
}

func TestPrettySingleTaskAndEmptyList(t *testing.T) {
	e := newCLIEnv(t)
	out, _, code := prettyRun(t, e, "YYYY-MM-DD", "tasks", "create",
		"--title", "One off", "--due", "2026-08-05T17:30:00+03:00", "--priority", "low", "--desc", "with a note")
	if code != 0 {
		t.Fatalf("create exit %d: %s", code, out)
	}
	for _, want := range []string{"Title", "One off", "Due", "2026-08-05 17:30", "Priority", "low", "with a note"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty task missing %q:\n%s", want, out)
		}
	}

	out, _, code = prettyRun(t, e, "YYYY-MM-DD", "completions", "list")
	if code != 0 {
		t.Fatalf("completions exit %d", code)
	}
	if !strings.Contains(out, "No results.") {
		t.Fatalf("empty list should say so, got:\n%s", out)
	}
}

func TestPrettyErrorsAreOneReadableLine(t *testing.T) {
	e := newCLIEnv(t)
	out, errOut, code := prettyRun(t, e, "DD/MM/YYYY", "tasks", "get", "missing-id")
	if code != cli.ExitNotFound {
		t.Fatalf("exit %d, want %d", code, cli.ExitNotFound)
	}
	if out != "" {
		t.Fatalf("failures must not write to stdout, got %q", out)
	}
	lines := strings.Split(strings.TrimSpace(errOut), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected a single error line, got %d:\n%s", len(lines), errOut)
	}
	if !strings.HasPrefix(lines[0], "error: ") || strings.Contains(lines[0], "{") {
		t.Fatalf("error line should be plain text, got %q", lines[0])
	}
}

func TestErrorsStayJSONWithoutPretty(t *testing.T) {
	e := newCLIEnv(t)
	cfg := clientconfig.ClientConfig{URL: e.server.URL, Token: e.token, LogLevel: "info"}
	app := cli.New(cfg, false)
	errBuf := &bytes.Buffer{}
	app.Out, app.Err = &bytes.Buffer{}, errBuf
	if code := app.Run([]string{"tasks", "get", "missing-id"}); code != cli.ExitNotFound {
		t.Fatalf("exit %d", code)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(errBuf.String())), &env); err != nil {
		t.Fatalf("agent error output must stay JSON: %v (%q)", err, errBuf.String())
	}
	if _, ok := env["error"]; !ok {
		t.Fatalf("expected an error envelope, got %v", env)
	}
}
