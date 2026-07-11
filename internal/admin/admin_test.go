package admin_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mtzanidakis/dodo/internal/admin"
	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/store"
)

func newDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/dodo.sqlite"
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = d.Close() }()
	if err := d.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return path
}

func runAdmin(t *testing.T, dbPath string, args []string) (int, string, string) {
	t.Helper()
	t.Setenv("DODO_DATABASE_PATH", dbPath)
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr
	code := admin.Run(args, "test", "abc")
	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)
	return code, string(outBytes), string(errBytes)
}

func TestAdminBootstrapFirstUser(t *testing.T) {
	code, out, _ := runAdmin(t, newDB(t), []string{"user", "create", "--email", "admin@example.com", "--password", "pass12345"})
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	bs := out
	var u map[string]any
	if err := json.Unmarshal([]byte(bs), &u); err != nil {
		t.Fatalf("decode (out=%q): %v", bs, err)
	}
	if u["role"] != "admin" {
		t.Fatalf("first user should default to admin, got %v", u["role"])
	}
}

func TestAdminPasswordValidation(t *testing.T) {
	code, _, errStr := runAdmin(t, newDB(t), []string{"user", "create", "--email", "x@y.com", "--password", "short"})
	if code == 0 {
		t.Fatalf("short password should fail")
	}
	if !strings.Contains(errStr, "8 characters") {
		t.Fatalf("unexpected error: %s", errStr)
	}
}

func TestAdminUserListNoHashLeak(t *testing.T) {
	dbPath := newDB(t)
	runAdmin(t, dbPath, []string{"user", "create", "--email", "admin@example.com", "--password", "pass12345"})
	_, out, _ := runAdmin(t, dbPath, []string{"user", "list"})
	if strings.Contains(out, "argon2") {
		t.Fatalf("password hash leaked in list output: %s", out)
	}
	if !strings.Contains(out, "admin@example.com") {
		t.Fatalf("user missing from list: %s", out)
	}
}

func TestAdminTokenCreateAndList(t *testing.T) {
	dbPath := newDB(t)
	runAdmin(t, dbPath, []string{"user", "create", "--email", "admin@example.com", "--password", "pass12345"})
	code, out, _ := runAdmin(t, dbPath, []string{"token", "create", "--email", "admin@example.com", "--name", "agent"})
	if code != 0 {
		t.Fatalf("token create exit %d", code)
	}
	var tok map[string]any
	json.Unmarshal([]byte(out), &tok)
	if fullToken, _ := tok["token"].(string); !strings.HasPrefix(fullToken, "dodo_") {
		t.Fatalf("expected full token, got %v", tok["token"])
	}
	_, out2, _ := runAdmin(t, dbPath, []string{"token", "list", "--email", "admin@example.com"})
	if strings.Contains(out2, "token_hash") {
		t.Fatalf("token hash leaked: %s", out2)
	}
	if !strings.Contains(out2, "agent") {
		t.Fatalf("token not in list: %s", out2)
	}
}

func TestAdminUserGetAndResetPassword(t *testing.T) {
	dbPath := newDB(t)
	runAdmin(t, dbPath, []string{"user", "create", "--email", "bob@example.com", "--password", "pass12345"})
	_, out, _ := runAdmin(t, dbPath, []string{"user", "get", "--email", "bob@example.com"})
	var u map[string]any
	json.Unmarshal([]byte(out), &u)
	if u["email"] != "bob@example.com" {
		t.Fatalf("get mismatch: %s", out)
	}
	code, _, _ := runAdmin(t, dbPath, []string{"user", "reset-password", "--email", "bob@example.com", "--password", "newpass12"})
	if code != 0 {
		t.Fatalf("reset-password exit %d", code)
	}
}

func TestAdminMigrateAndVersion(t *testing.T) {
	code, _, _ := runAdmin(t, newDB(t), []string{"migrate"})
	if code != 0 {
		t.Fatalf("migrate exit %d", code)
	}
	code, out, _ := runAdmin(t, newDB(t), []string{"version"})
	if code != 0 || !strings.Contains(out, "test") {
		t.Fatalf("version: code %d out %s", code, out)
	}
}

func TestAdminDirectStoreReusable(t *testing.T) {
	dbPath := newDB(t)
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = d.Close() }()
	st := store.New(d)
	if _, err := st.Users.List(context.Background()); err != nil {
		t.Fatalf("list: %v", err)
	}
}
