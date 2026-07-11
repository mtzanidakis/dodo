package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mtzanidakis/dodo/internal/api"
	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/config"
	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/ws"
)

type stubTelegram struct{}

func (stubTelegram) ValidateToken(context.Context, string) (string, error)  { return "botname", nil }
func (stubTelegram) SendTest(context.Context, string, string, string) error { return nil }
func (stubTelegram) SendReminder(context.Context, string, string, string, string, string) error {
	return nil
}
func (stubTelegram) StartForUser(context.Context, string) error { return nil }
func (stubTelegram) StopForUser(string) error                   { return nil }
func (stubTelegram) StartAll(context.Context) error             { return nil }
func (stubTelegram) StopAll()                                   {}

func testConfig() config.Config {
	return config.Config{EncryptionKey: testKey()}
}

func testKey() []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

type testServer struct {
	*api.Server
	store   *store.Store
	tokenA  string
	userA   *models.User
	tokenB  string
	userB   *models.User
	cookieA string
	hub     *ws.Hub
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	t.Parallel()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	st := store.New(d)
	hub := ws.NewHub(slog.Default())
	tg := &stubTelegram{}
	srv, err := api.NewServer(testConfig(), st, hub, tg, slog.Default(), "test")
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	ts := &testServer{Server: srv, store: st, hub: hub}

	ts.userA = makeUser(t, st, "a@example.com", "pass1234")
	ts.userB = makeUser(t, st, "b@example.com", "pass1234")
	ts.tokenA = makeToken(t, st, ts.userA.ID, "agentA")
	ts.tokenB = makeToken(t, st, ts.userB.ID, "agentB")
	ts.cookieA = loginSession(t, st, ts.userA.ID)
	return ts
}

func (ts *testServer) handler() http.Handler { return ts.Server.Handler() }

func makeUser(t *testing.T, st *store.Store, email, pw string) *models.User {
	t.Helper()
	hash, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := &models.User{Email: email, PasswordHash: hash, Timezone: "Europe/Athens", Locale: models.LocaleEn}
	if err := st.Users.Create(context.Background(), u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func makeToken(t *testing.T, st *store.Store, userID, name string) string {
	t.Helper()
	gen, _ := auth.GenerateAPIToken()
	if _, err := st.Tokens.Create(context.Background(), userID, name, gen.Prefix, gen.Hash); err != nil {
		t.Fatalf("create token: %v", err)
	}
	return gen.Full
}

func loginSession(t *testing.T, st *store.Store, userID string) string {
	t.Helper()
	gen, _ := auth.GenerateSession()
	if _, err := st.Sessions.Create(context.Background(), userID, gen.Hash, "test", 24*time.Hour); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return gen.Full
}

func (ts *testServer) do(t *testing.T, method, path, token, cookie string, body any) (*httptest.ResponseRecorder, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: cookie})
	}
	rec := httptest.NewRecorder()
	ts.handler().ServeHTTP(rec, req)
	return rec, rec.Body.Bytes()
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	if len(body) == 0 {
		return v
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode: %v body=%s", err, body)
	}
	return v
}

func TestCreateGetListComplete(t *testing.T) {
	ts := newTestServer(t)
	due := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	body := map[string]any{"title": "Pay bill", "due_at": due, "priority": "normal"}
	rec, b := ts.do(t, http.MethodPost, "/api/v1/tasks", ts.tokenA, "", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, b)
	}
	created := decode[taskDTO](t, b)
	if created.Title != "Pay bill" {
		t.Fatalf("title: %v", created)
	}

	rec, b = ts.do(t, http.MethodGet, "/api/v1/tasks/"+created.ID, ts.tokenA, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d", rec.Code)
	}

	rec, b = ts.do(t, http.MethodGet, "/api/v1/tasks?filter=pending", ts.tokenA, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	list := decode[listResp](t, b)
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 task, got %d", len(list.Items))
	}

	rec, b = ts.do(t, http.MethodPost, "/api/v1/tasks/"+created.ID+"/complete", ts.tokenA, "", map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: %d %s", rec.Code, b)
	}
}

type taskDTO struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type listResp struct {
	Items []taskDTO `json:"items"`
}

func TestCreateRecurringAdvances(t *testing.T) {
	ts := newTestServer(t)
	due := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	freq := "daily"
	body := map[string]any{"title": "Daily", "due_at": due, "recurrence_freq": freq, "recurrence_interval": 1}
	rec, b := ts.do(t, http.MethodPost, "/api/v1/tasks", ts.tokenA, "", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create recurring: %d %s", rec.Code, b)
	}
	created := decode[taskDTO](t, b)
	rec, b = ts.do(t, http.MethodPost, "/api/v1/tasks/"+created.ID+"/complete", ts.tokenA, "", map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("complete recurring: %d %s", rec.Code, b)
	}
	var resp struct {
		Task       taskDTO `json:"task"`
		Completion struct {
			TaskID string `json:"task_id"`
		} `json:"completion"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Completion.TaskID == "" {
		t.Fatalf("recurring completion row expected")
	}
	if !strings.Contains(resp.Task.ID, "") || resp.Task.ID == "" {
		t.Fatalf("task id missing")
	}
	got, _ := ts.store.Tasks.Get(context.Background(), ts.userA.ID, created.ID)
	nextDay := got.DueAt
	if !nextDay.Equal(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("due should advance +1d, got %v", nextDay)
	}
}

func TestCrossUserIsolation(t *testing.T) {
	ts := newTestServer(t)
	due := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	body := map[string]any{"title": "A's task", "due_at": due}
	rec, b := ts.do(t, http.MethodPost, "/api/v1/tasks", ts.tokenA, "", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}
	created := decode[taskDTO](t, b)

	rec, _ = ts.do(t, http.MethodGet, "/api/v1/tasks/"+created.ID, ts.tokenB, "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("user B reading A's task should be 404, got %d", rec.Code)
	}

	rec, b = ts.do(t, http.MethodGet, "/api/v1/tasks", ts.tokenB, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	list := decode[listResp](t, b)
	if len(list.Items) != 0 {
		t.Fatalf("user B should see 0 tasks, got %d", len(list.Items))
	}
	_ = created
}

func TestTokenCreateRevocation(t *testing.T) {
	ts := newTestServer(t)
	rec, b := ts.do(t, http.MethodPost, "/api/v1/tokens", ts.tokenA, "", map[string]any{"name": "new"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", rec.Code, b)
	}
	var tok struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(b, &tok)
	if tok.Token == "" {
		t.Fatalf("token returned once")
	}
	rec, _ = ts.do(t, http.MethodDelete, "/api/v1/tokens/"+tok.ID, ts.tokenA, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d", rec.Code)
	}
	rec, _ = ts.do(t, http.MethodDelete, "/api/v1/tokens/"+tok.ID, ts.tokenA, "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("revoke twice: %d", rec.Code)
	}
}

func TestProfileUpdateAndPasswordChange(t *testing.T) {
	ts := newTestServer(t)
	rec, b := ts.do(t, http.MethodPatch, "/api/v1/me", ts.tokenA, "", map[string]any{"display_name": "Alice", "theme": "dark"})
	if rec.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", rec.Code, b)
	}
	prof := decode[struct {
		DisplayName string `json:"display_name"`
		Theme       string `json:"theme"`
	}](t, b)
	if prof.DisplayName != "Alice" || prof.Theme != "dark" {
		t.Fatalf("profile mismatch: %+v", prof)
	}
	rec, _ = ts.do(t, http.MethodPost, "/api/v1/me/password", ts.tokenA, "", map[string]any{"current_password": "pass1234", "new_password": "newpass12"})
	if rec.Code != http.StatusOK {
		t.Fatalf("change pw: %d", rec.Code)
	}
	rec, _ = ts.do(t, http.MethodPost, "/api/v1/me/password", ts.tokenA, "", map[string]any{"current_password": "pass1234", "new_password": "newpass12"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current pw: %d", rec.Code)
	}
}

func TestLoginSession(t *testing.T) {
	ts := newTestServer(t)
	rec, _ := ts.do(t, http.MethodPost, "/api/v1/auth/login", "", "", map[string]any{"email": "a@example.com", "password": "pass1234"})
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d", rec.Code)
	}
	var setCookie string
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.SessionCookie {
			setCookie = c.Value
		}
	}
	if setCookie == "" {
		t.Fatalf("session cookie not set")
	}
	rec, _ = ts.do(t, http.MethodGet, "/api/v1/auth/me", "", setCookie, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me via session: %d", rec.Code)
	}
	rec, _ = ts.do(t, http.MethodPost, "/api/v1/auth/login", "", "", map[string]any{"email": "a@example.com", "password": "wrong"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login: %d", rec.Code)
	}
}
