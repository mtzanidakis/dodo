ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin'));
ALTER TABLE users ADD COLUMN disabled_at TEXT;
ALTER TABLE users ADD COLUMN email_verified_at TEXT;
ALTER TABLE users ADD COLUMN default_due TEXT NOT NULL DEFAULT '24h' CHECK (default_due IN ('now', '24h', 'week', 'month'));
ALTER TABLE users ADD COLUMN default_priority TEXT NOT NULL DEFAULT 'normal' CHECK (default_priority IN ('low', 'normal', 'high'));
ALTER TABLE users ADD COLUMN default_recurrence TEXT NOT NULL DEFAULT 'once' CHECK (default_recurrence IN ('once', 'daily', 'weekly', 'monthly', 'yearly'));
ALTER TABLE users ADD COLUMN default_recurrence_interval INTEGER NOT NULL DEFAULT 1 CHECK (default_recurrence_interval BETWEEN 1 AND 365);

UPDATE users SET email_verified_at = created_at WHERE deleted_at IS NULL;
UPDATE users SET role = 'admin'
 WHERE id = (
   SELECT id FROM users
   WHERE deleted_at IS NULL
   ORDER BY created_at ASC, id ASC LIMIT 1
 );

CREATE TABLE email_verification_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT
);
CREATE INDEX idx_email_verification_tokens_user ON email_verification_tokens(user_id);

CREATE INDEX idx_users_admin_active ON users(role, disabled_at)
 WHERE deleted_at IS NULL;
