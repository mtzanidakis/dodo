package store

import (
	"database/sql"
	"time"

	"github.com/mtzanidakis/dodo/internal/db"
)

func formatTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(nullStr sql.NullString) time.Time {
	if !nullStr.Valid || nullStr.String == "" {
		return time.Time{}
	}
	return db.ParseUTC(nullStr.String)
}

func parseNullableTime(nullStr sql.NullString) *time.Time {
	if !nullStr.Valid || nullStr.String == "" {
		return nil
	}
	t := db.ParseUTC(nullStr.String)
	if t.IsZero() {
		return nil
	}
	return &t
}

func parseIntPtr(nullInt sql.NullInt64) *int {
	if !nullInt.Valid {
		return nil
	}
	v := int(nullInt.Int64)
	return &v
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
