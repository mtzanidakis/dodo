package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/migrations"
)

func TestApplyInOrder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	var name string
	err = d.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='users'").Scan(&name)
	if err != nil {
		t.Fatalf("users table missing: %v", err)
	}
	if name != "users" {
		t.Fatalf("expected users, got %q", name)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, err := db.OpenWithoutMigrations(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	if err := migrations.Apply(ctx, d); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := migrations.Apply(ctx, d); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	var count int
	if err := d.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 applied migration after re-apply, got %d", count)
	}
}

func TestReopenRunsNoMigrations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "dodo.sqlite")

	d1, err := db.Open(path)
	if err != nil {
		t.Fatalf("open d1: %v", err)
	}
	var before int
	if err := d1.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&before); err != nil {
		t.Fatalf("d1 before: %v", err)
	}
	d1.Close()

	d2, err := db.Open(path)
	if err != nil {
		t.Fatalf("reopen d2: %v", err)
	}
	defer d2.Close()
	var after int
	if err := d2.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&after); err != nil {
		t.Fatalf("d2 after: %v", err)
	}
	if after != before {
		t.Fatalf("reopen changed migration count: before %d after %d", before, after)
	}
}
