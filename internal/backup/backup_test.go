package backup

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestBackup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.sqlite")

	d, err := sql.Open("sqlite", src)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	if _, err := d.Exec("CREATE TABLE t(x)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := d.Exec("INSERT INTO t VALUES (1),(2),(3)"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_ = d.Close()

	t.Setenv("DODO_DATABASE_PATH", src)
	dest := filepath.Join(dir, "backup.sqlite")

	if code := Run([]string{"-dump", dest}); code != 0 {
		t.Fatalf("backup exit %d", code)
	}

	b, err := sql.Open("sqlite", dest)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer func() { _ = b.Close() }()
	var n int
	if err := b.QueryRow("SELECT count(*) FROM t").Scan(&n); err != nil {
		t.Fatalf("query backup: %v", err)
	}
	if n != 3 {
		t.Fatalf("backup row count = %d, want 3", n)
	}

	if code := Run(nil); code != 2 {
		t.Fatalf("missing -dump: exit %d, want 2", code)
	}
	if code := Run([]string{"-dump", dest}); code != 1 {
		t.Fatalf("existing dest: exit %d, want 1", code)
	}
}
