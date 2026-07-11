package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mtzanidakis/dodo/internal/models"
)

type Store struct {
	Users       *Users
	Tokens      *Tokens
	Sessions    *Sessions
	Tasks       *Tasks
	Completions *Completions
	Audit       *Audit

	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{
		Users:       &Users{db: db},
		Tokens:      &Tokens{db: db},
		Sessions:    &Sessions{db: db},
		Tasks:       &Tasks{db: db},
		Completions: &Completions{db: db},
		Audit:       &Audit{db: db},
		db:          db,
	}
}

func newUUID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("uuid.NewV7: %v", err))
	}
	return id.String()
}

func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "UNIQUE constraint failed") ||
		strings.Contains(s, "UNIQUE") || strings.Contains(s, "constraint failed: UNIQUE")
}

func NotFound(err error) error {
	if isNotFound(err) {
		return fmt.Errorf("%w", models.ErrNotFound)
	}
	return err
}
