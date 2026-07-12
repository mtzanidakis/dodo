# AGENTS.md

Guidance for AI agents working on this repository.

## Project

`dodo` is a self-hosted todo/reminders service. Three Go binaries:

| Binary     | Subcommands        | Purpose                                                      |
|------------|--------------------|-------------------------------------------------------------|
| `dodo`     | `serve`, `admin`   | HTTP API + web UI + scheduler + telegram pollers; user/token admin (direct DB). Shipped in the container. |
| `dodo-cli` | (top-level)        | AI-agent CLI client (JSON stdout, `--pretty` for humans).    |
| `dodo-tui` | (top-level)        | Terminal UI client (API + token auth).                      |

## Tooling

- [mise](https://mise.jdx.dev/) installs Go 1.26.5, golangci-lint 2.12.2, Node 24.
- `mise trust` once per clone.

## Commands

```
mise run build-all     # build dodo, dodo-cli, dodo-tui
mise run build-server  # build only ./cmd/dodo
mise run test          # go test -race -covermode=atomic ./...
mise run lint          # golangci-lint run ./...
mise run web:build     # build web assets into internal/web/dist
mise run tidy          # go mod tidy
```

Always run **lint** and **test** before declaring a task done:

```
mise run lint && mise run test
```

Also `go vet ./...` and `gofmt -l .` (must be empty).

## Conventions

- Go 1.26.5, `internal/` package boundary.
- Logging: `log/slog` structured. Server/admin read `DODO_LOG_LEVEL`; clients read `log_level` from their config.
- Errors: sentinel errors in `internal/models/errors.go` (`ErrNotFound`, `ErrUnauthorized`, `ErrConflict`, `ErrValidation`). Wrap with `%w`. HTTP layer maps them to status codes.
- `context.Context` is the first param of every store/handler method.
- JSON: snake_case struct tags.
- Datetimes: stored in the DB as `TEXT` RFC3339 **UTC**. Convert to/from the user's timezone only at the API/render edge.
- IDs: UUIDv7 strings (`uuid.NewV7()`), `TEXT PRIMARY KEY`.
- Passwords: minimum 8 characters, enforced everywhere.
- Per-user data scoping: every store query touching `tasks`, `task_completions`, `api_tokens` (and the `me/*` routes) takes a `userID` and constrains `WHERE user_id = ?`. Body `user_id` fields are ignored. Cross-user `GET/PATCH/DELETE /tasks/{id}` returns **404** (not 403) to avoid leaking existence. All users are equal; there are no roles.
- No code comments unless genuinely needed. No emojis in code or UI.

## Server env vars (`dodo` binary only)

| Env                       | Default          | Notes                                            |
|---------------------------|------------------|--------------------------------------------------|
| `DODO_DATABASE_PATH`       | `/data/dodo.sqlite` | Used by `serve` and `admin`.                  |
| `DODO_LISTEN`             | `:8080`          | HTTP bind address.                               |
| `DODO_ENCRYPTION_KEY`     | (required)       | 32-byte base64 key for AES-256-GCM (telegram bot tokens). Generate with `openssl rand -base64 32`. Server refuses to start without it. |
| `DODO_DEFAULT_LOCALE`     | `en`             | `en` or `el`.                                    |
| `DODO_DEFAULT_TIMEZONE`   | `Europe/Athens`  | IANA name.                                       |
| `DODO_SCHEDULER_INTERVAL` | `1m`             | Reminder scan cadence (min `1m`).                |
| `DODO_LOG_LEVEL`          | `info`           | debug/info/warn/error.                           |

## Client config (`dodo-cli` / `dodo-tui`)

`~/.config/dodo/config.json` (overridable with `--config`), parsed with stdlib `encoding/json`. **No env vars.**

```json
{
  "url": "http://localhost:8080",
  "token": "dodo_xxxxxxxxxxxx",
  "log_level": "info",
  "timezone": "Europe/Athens"
}
```

`--url` and `--token` flags override the config file. Missing `url`/`token` when an API call is needed -> exit 5.

`timezone` (optional IANA name) is the display zone for rendering timestamps, resolved config -> profile (`/api/v1/me`) -> host local. The CLI rewrites timestamp fields in its JSON output to this zone (still valid RFC3339) and the TUI renders/parses input in it. `dodo-cli init` accepts `--timezone`.

## Quickstart (local)

```
mise install
mise run build-all
export DODO_ENCRYPTION_KEY=$(openssl rand -base64 32)
export DODO_DATABASE_PATH=/tmp/dodo.sqlite
printf 'supersecret\n' | ./dodo admin user create --email admin@example.com  # password via stdin, not argv
./dodo serve &            # listens on :8080
TOK=$(./dodo admin token create --email admin@example.com --name agent | jq -r .token)
./dodo-cli --url http://localhost:8080 --token "$TOK" init --url http://localhost:8080 --token "$TOK"
./dodo-cli tasks create --title "Pay bill" --due 2026-07-11T17:00:00Z --priority high
./dodo-cli tasks list
```

## Adding a migration

Create `internal/migrations/NNNN_name.sql` (numeric prefix), embed is automatic via `//go:embed *.sql`. The runner sorts by prefix and applies each in a transaction, recording the version in `schema_migrations`.

## Commits

Conventional Commits, enforced in CI by commitlint (`.commitlintrc.yml`):

```
<type>(<scope>): <subject>

<body, wrap at 72>

<footer>
```

Types: `feat fix refactor test chore docs ci build perf style`. Scopes: `db auth api recurrence scheduler notify telegram web i18n tui cli admin docker ci crypto config`. Subject: imperative, lowercase, no trailing period, max 72 chars.

## Branch protection (main)

Require `ci` (lint, test, build) to pass; require conventional commits; linear history; no force-push to `main`.

## Tests

- In-memory SQLite (`:memory:`) for store tests; `httptest` for api/cli tests; table-driven; `t.Parallel()` where safe (but not in tests that use `t.Setenv`).
- Aim for >= 85% coverage in `internal/store`, `internal/recurrence`, `internal/auth`, `internal/api`, `internal/scheduler`, `internal/notify`.