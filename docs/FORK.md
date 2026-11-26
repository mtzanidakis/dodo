# Dodo custom fork — multi-user administration

This fork implements role-based administration, public email-verified signups,
account activation and deletion, and per-user task defaults. The original
SQLite database is migrated automatically on first startup with the updated
binary (migrations 0003 and 0004). Existing users and tasks are retained.

## Configuration

Copy the SMTP settings from `smtp.env.example` into the same `.env`
file that holds `DODO_ENCRYPTION_KEY`:

- `DODO_PUBLIC_URL`: canonical public HTTPS origin, e.g. `https://dodo.example.org`
- `DODO_SMTP_HOST`, `DODO_SMTP_PORT`: SMTP endpoint
- `DODO_SMTP_USER`, `DODO_SMTP_PASSWORD`: SMTP credentials
- `DODO_SMTP_FROM`: envelope/from email address allowed by the SMTP server
- `DODO_SMTP_MODE`: `starttls` (typically port 587) or `tls` (typically port 465)

Public registration is unavailable until the SMTP endpoint, sender, and HTTPS
public origin have been configured. Verification tokens are one-time and valid
for 24 hours. Passwords are hashed with the existing Argon2id implementation.
Accounts created through the administrator panel are active immediately.

## Deploying the local build

**Do not replace your existing Dodo installation without a consistent SQLite
backup and a successful test build.** Keep the existing encryption key and
persistent volume mapping. From the fork checkout:

```sh
docker compose build --no-cache
docker compose up -d
docker compose logs --tail=100 dodo
```

The Dockerfile executes frontend tests, `go vet`, and Go tests before
producing the runtime image. A failed test stops the build.

For an installation with an existing `dodo` container, first create a
consistent database dump with the original binary's backup command, then
store a copy outside the Docker volume:

```sh
docker exec dodo dodo backup -dump /data/dodo-pre-upgrade.sqlite
docker cp dodo:/data/dodo-pre-upgrade.sqlite ./dodo-pre-upgrade.sqlite
```

Review your existing Compose file before switching: the fork's sample
`compose.yaml` builds `dodo-custom:latest` locally and uses the named
volume `dodo-data`. Do not switch volumes without migrating the data.

## Administration

The oldest nondeleted user is promoted to administrator on migration; ties
are broken by user ID. In a fresh database, the first administratively created
user (CLI or web) is promoted. The first publicly registered and verified
user also becomes administrator if no active administrator exists.

Administrators can list accounts, create users with chosen passwords, disable
or re-enable accounts, permanently delete them, and change roles. SQLite
triggers prevent deleting, disabling or demoting the final active admin,
including through paths outside the web UI.

The account screen exposes personal defaults between profile and password:
due offset, priority, recurrence, and recurrence interval (1–365). New-task
defaults are fetched when the editor is opened, using the profile timezone.

All new labels are translated in English, Italian and Greek.

## Daily Telegram summary

Every account has an optional daily summary under **Settings**. The checkbox is
off by default and the initial delivery time is 08:00. Times use the IANA
timezone configured in the account profile, including daylight-saving changes.

For a summary to be delivered, Telegram must be configured and linked for that
user. Once per local calendar date, when the scheduler reaches or passes the
chosen time, the service sends one plain-text Telegram message listing pending
tasks whose due date falls on that day. Recurrences are expanded for the local
day. If no task is due that day, no message is sent. A successful send is
persisted so a restart does not resend the same day's summary. On transmission
failure, the scheduler retries at its next tick.

Settings are stored in SQLite migration `0005_daily_digest.sql`. The
application remains compatible with existing users; daily summaries are off
until enabled individually. The site footer and Settings navigation point to
this personal fork.
