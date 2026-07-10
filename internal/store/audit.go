package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type Audit struct {
	db *sql.DB
}

type AuditEntry struct {
	ID         int64
	UserID     string
	Action     string
	TargetType string
	TargetID   string
	Meta       map[string]any
	CreatedAt  time.Time
}

func (s *Audit) Log(ctx context.Context, userID, action, targetType, targetID string, meta map[string]any) error {
	var metaJSON any
	if meta != nil {
		b, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		metaJSON = string(b)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_log (user_id, action, target_type, target_id, meta, created_at)
 VALUES (?,?,?,?,?,?)`,
		nullableStr(userID), action, nullableStr(targetType), nullableStr(targetID), metaJSON, formatTime(time.Now().UTC()))
	return err
}

func (s *Audit) List(ctx context.Context, limit int) ([]*AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, action, target_type, target_id, meta, created_at FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		var (
			uid     sql.NullString
			ttype   sql.NullString
			tid     sql.NullString
			metaStr sql.NullString
		)
		var createdAt sql.NullString
		if err := rows.Scan(&e.ID, &uid, &e.Action, &ttype, &tid, &metaStr, &createdAt); err != nil {
			return nil, err
		}
		e.UserID = uid.String
		e.TargetType = ttype.String
		e.TargetID = tid.String
		e.CreatedAt = parseTime(createdAt)
		if metaStr.Valid && metaStr.String != "" {
			_ = json.Unmarshal([]byte(metaStr.String), &e.Meta)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
