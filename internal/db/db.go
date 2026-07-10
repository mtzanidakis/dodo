package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/mtzanidakis/dodo/internal/migrations"
)

func Open(path string) (*sql.DB, error) {
	dsn := path
	dsn += "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite %s: %w", path, err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging sqlite %s: %w", path, err)
	}

	if err := migrations.Apply(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("applying migrations: %w", err)
	}

	return db, nil
}

func OpenWithoutMigrations(path string) (*sql.DB, error) {
	dsn := path
	dsn += "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging sqlite %s: %w", path, err)
	}
	return db, nil
}
