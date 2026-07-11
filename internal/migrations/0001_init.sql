CREATE TABLE users (
    id                   TEXT PRIMARY KEY,
    email                TEXT NOT NULL,
    password_hash        TEXT NOT NULL,
    display_name         TEXT NOT NULL DEFAULT '',
    timezone             TEXT NOT NULL DEFAULT 'Europe/Athens',
    locale               TEXT NOT NULL DEFAULT 'en',
    theme                TEXT NOT NULL DEFAULT 'system',
    telegram_bot_token    TEXT,
    telegram_allowed_user_ids TEXT,
    telegram_chat_id      TEXT,
    telegram_chat_user_id TEXT,
    telegram_configured_at TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    deleted_at           TEXT
);

CREATE UNIQUE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;

CREATE TABLE api_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_prefix TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    last_used_at TEXT,
    expires_at   TEXT,
    created_at   TEXT NOT NULL,
    revoked_at   TEXT
);

CREATE TABLE sessions (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash    TEXT NOT NULL UNIQUE,
    user_agent    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    expires_at    TEXT NOT NULL
);

CREATE TABLE tasks (
    id                    TEXT PRIMARY KEY,
    user_id               TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title                 TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    priority              TEXT NOT NULL DEFAULT 'normal',
    kind                  TEXT NOT NULL DEFAULT 'oneoff',
    due_at                TEXT NOT NULL,
    duration_minutes      INTEGER NOT NULL DEFAULT 0,
    completed_at          TEXT,
    recurrence_freq      TEXT,
    recurrence_interval  INTEGER NOT NULL DEFAULT 1,
    recurrence_by_day    TEXT,
    recurrence_by_month_day INTEGER,
    recurrence_end_at    TEXT,
    last_notified_at     TEXT,
    snoozed_until        TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

CREATE INDEX idx_tasks_user_due ON tasks(user_id, due_at) WHERE completed_at IS NULL;
CREATE INDEX idx_tasks_user_completed ON tasks(user_id, completed_at);

CREATE TABLE task_completions (
    id            TEXT PRIMARY KEY,
    task_id       TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    priority      TEXT NOT NULL,
    due_at        TEXT NOT NULL,
    completed_at  TEXT NOT NULL,
    created_at    TEXT NOT NULL
);

CREATE INDEX idx_completions_user ON task_completions(user_id, completed_at);

CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     TEXT,
    action      TEXT NOT NULL,
    target_type TEXT,
    target_id   TEXT,
    meta        TEXT,
    created_at  TEXT NOT NULL
);