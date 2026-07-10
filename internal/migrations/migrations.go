package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed *.sql
var migrationFS embed.FS

type Migration struct {
	Version int
	Source  string
	SQL     string
}

func Load() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return nil, fmt.Errorf("reading embedded migrations: %w", err)
	}
	var ms []Migration
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("migration %q: expected <version>_<name>.sql", name)
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("migration %q: invalid version prefix %q", name, parts[0])
		}
		data, err := migrationFS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("reading migration %s: %w", name, err)
		}
		ms = append(ms, Migration{Version: v, Source: name, SQL: string(data)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].Version < ms[j].Version })
	return ms, nil
}

func Apply(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
)`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	migrations, err := Load()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if err := applyOne(ctx, db, m); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Source, err)
		}
	}
	return nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("reading schema_migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	m := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scanning schema_migrations: %w", err)
		}
		m[v] = true
	}
	return m, rows.Err()
}

func applyOne(ctx context.Context, db *sql.DB, m Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("exec sql: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		m.Version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("recording migration: %w", err)
	}
	return tx.Commit()
}
