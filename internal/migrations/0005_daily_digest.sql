ALTER TABLE users ADD COLUMN daily_digest_enabled INTEGER NOT NULL DEFAULT 0 CHECK (daily_digest_enabled IN (0,1));
ALTER TABLE users ADD COLUMN daily_digest_time TEXT NOT NULL DEFAULT '08:00';
ALTER TABLE users ADD COLUMN daily_digest_last_sent_date TEXT;
