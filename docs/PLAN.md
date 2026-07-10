# dodo — Implementation Plan

Detailed plan derived from `docs/SPECS.md` and the decisions captured during planning.
Designed to be handed to an implementing agent. Work through phases sequentially unless noted; each phase lists **Files**, **Tasks**, **Done when**.

---

## 0. Conventions & global decisions

### 0.1 Three binaries
Build produces three separate Go binaries:

| Binary        | Subcommands            | Purpose                                                        | Where it runs        |
|---------------|------------------------|----------------------------------------------------------------|----------------------|
| `dodo`        | `serve`, `admin`       | HTTP API + embedded web UI + websocket + scheduler + telegram; user/token management (direct DB access) | In the container     |
| `dodo-cli`    | (top-level)            | AI-agent CLI client (JSON output, `--pretty` for humans)       | End-user workstation |
| `dodo-tui`    | (top-level)            | Terminal UI client (API + token auth)                          | End-user workstation |

Entry wiring lives in `cmd/dodo/main.go` (dispatches `serve`/`admin`), `cmd/dodo-cli/main.go`, and `cmd/dodo-tui/main.go`; implementations live in `internal/<area>`. The container bundles only the `dodo` binary (serve + admin). goreleaser cross-compiles `dodo-cli` and `dodo-tui` for end-user distribution.

### 0.2 External dependencies (kept minimal)
Required:
- `modernc.org/sqlite` — pure-Go SQLite driver.
- `github.com/coder/websocket` — websocket support (stdlib has none).
- `golang.org/x/crypto/argon2` — `argon2id` password hashing.
- `github.com/google/uuid` — IDs (UUIDv7; time-ordered, sortable, DB-friendly).
- `github.com/charmbracelet/bubbletea`, `bubbles`, `lipgloss` — TUI.
- `github.com/pelletier/go-toml/v2` — TOML config file parser for CLI/TUI (`encoding/json` handles all JSON; stdlib has no TOML).

Everything else uses the Go standard library (`net/http` with `ServeMux` method patterns for routing, `log/slog` for logging, `encoding/json`, `html/template`, `time`, `crypto/rand`, `os`).

Do NOT add: chi/gin/echo, viper, cobra (use a tiny stdlib dispatcher), golang-migrate (write a 50-line runner), telego or any other Telegram library (hand-rolled Bot API client in `internal/telegram` — see Phase 8; only `getMe`, `sendMessage`, `getUpdates`, `answerCallbackQuery`, `editMessageText` are needed).

### 0.3 Repo layout
```
dodo/
├── mise.toml
├── go.mod / go.sum
├── .golangci.yaml
├── .goreleaser.yml
├── Dockerfile
├── .github/workflows/{ci.yml,docker.yml,release.yml}
├── .commitlintrc.yml
├── cmd/
│   ├── dodo/main.go          # serve + admin entry
│   ├── dodo-cli/main.go      # agent CLI entry
│   └── dodo-tui/main.go       # TUI entry
├── internal/
│   ├── config/        env → typed struct (server/admin)
│   ├── clientconfig/  ~/.config/dodo/config.toml loader (cli/tui)
│   ├── db/            *sqlite.Conn, migration runner connection helper
│   ├── migrations/    *.sql embedded; migrations.go with //go:embed
│   ├── models/        domain structs (User, Task, ApiToken, Session, ...)
│   ├── store/         repository layer (one file per entity)
│   ├── auth/          argon2id, session cookie, bearer token middleware
│   ├── crypto/        AES-256-GCM encrypt/decrypt for secrets-at-rest
│   ├── api/           http handlers + ServeMux wiring + middleware
│   ├── ws/            websocket hub
│   ├── telegram/      hand-rolled Bot API client + per-user long-polling registry
│   ├── notify/        notification dispatcher (telegram + ws fan-out)
│   ├── scheduler/     reminder scan loop
│   ├── recurrence/    next/all occurrence math
│   ├── i18n/          translation loader + T()
│   ├── web/           html templates + handlers + dist/ (built assets, gitignored) + go:embed
│   ├── admin/         admin CLI commands (direct DB)
│   ├── cli/           agent CLI commands (HTTP client + JSON)
│   └── tui/           bubbletea program
├── web/               frontend sources (build → internal/web/dist)
│   ├── package.json
│   ├── tailwind.config.js
│   ├── postcss.config.js
│   └── src/
├── locales/
│   ├── en.json
│   └── el.json
└── docs/{SPECS.md,PLAN.md}
```

### 0.4 Coding conventions
- Go 1.26.4, modules, `internal/` package boundary.
- Logging: `log/slog` with structured fields. The `dodo` (server/admin) binary reads level from `DODO_LOG_LEVEL`; `dodo-cli`/`dodo-tui` read `log_level` from their TOML config (default `info`).
- Errors: define sentinel errors in `internal/models/errors.go` (`ErrNotFound`, `ErrUnauthorized`, `ErrConflict`, `ErrValidation`). Wrap with `%w`. HTTP layer maps them to status codes.
- Context: every store/handler signature takes `ctx context.Context` first.
- JSON: snake_case via struct tags (`json:"due_at"`).
- Time: **all datetimes stored in DB as `TEXT` RFC3339 UTC**. Convert to/from user timezone only at the API/render edge via `time.LoadLocation(user.Timezone)`.
- IDs: UUIDv7 strings (time-ordered, sortable), stored as `TEXT PRIMARY KEY`. Generate via `uuid.NewV7()`.
- Passwords: minimum 8 characters; enforced everywhere a password is set (API, web forms, `dodo admin`).
- Naming: `store.UserStore`, `store.TaskStore`, etc. Methods `Create/Get/Update/Delete/List` + domain verbs (`Complete`, `AdvanceRecurring`).
- Tests: table-driven, `_test.go` next to source, `t.Parallel()` where safe; in-memory SQLite (`:memory:`) for store tests; `httptest` for API tests.
- No comments in code unless genuinely needed. No emojis in code or UI.
- **Per-user data scoping**: every store query that touches `tasks`, `task_completions`, `api_tokens` (and the `me/*` routes) must take a `userID` argument and constrain `WHERE user_id = ?`. The API handler reads the authenticated user from context (set by `AuthSession` or `AuthBearer`) and passes it down; never trust a `user_id` field in request bodies for these routes. An admin role never grants cross-user read/write of task data via the HTTP API — admin powers are limited to the `dodo admin` CLI (direct DB) for user/token management. If task `{id}` belongs to a different user, return `404` (not `403`) to avoid leaking existence.
- golangci-lint v2.12.2 with `.golangci.yaml` (enable errcheck, govet, staticcheck, ineffassign, unused, gocritic, revive; keep test linters relaxed).

### 0.5 Server configuration (env vars — `dodo` binary only)

Env vars are used **exclusively** by the `dodo` binary (`serve` + `admin`). `dodo-cli` and `dodo-tui` ignore all env vars; they read `~/.config/dodo/config.toml` (see §0.6).

| Env                        | Default        | Notes                                              |
|----------------------------|----------------|----------------------------------------------------|
| `DODO_DATABASE_PATH`       | `/data/dodo.sqlite` | Used by `serve` and `admin`.                    |
| `DODO_LISTEN`              | `:8080`        | HTTP bind address.                                 |
| `DODO_ENCRYPTION_KEY`      | (required)     | 32-byte base64 key for AES-256-GCM encryption of secrets at rest (telegram bot tokens). **Required** — server refuses to start without it. Generate with `openssl rand -base64 32`. |
| `DODO_DEFAULT_LOCALE`      | `en`           | `en` or `el`.                                      |
| `DODO_DEFAULT_TIMEZONE`    | `Europe/Athens` | IANA name; fallback when browser cannot detect or admin creates a user. |
| `DODO_SCHEDULER_INTERVAL`  | `1m`           | Reminder scan cadence (Go duration; minimum `1m`).  |
| `DODO_LOG_LEVEL`           | `info`         | debug/info/warn/error (server + admin only).       |

`internal/config.Load()` returns a validated `Config` struct; unknown/invalid env values are startup errors except where defaults are documented.

### 0.6 Client configuration (`dodo-cli` / `dodo-tui` — TOML config file)

Both client binaries read a TOML config file at `~/.config/dodo/config.toml` (path overridable via `--config <path>` flag). They **do not read any environment variables** — not `DODO_API_URL`, not `DODO_*`, nothing.

File format:
```toml
url      = "http://localhost:8080"   # API base URL (required)
token    = "dodo_xxxxxxxxxxxx"       # API token (required for API calls)
log_level = "info"                    # optional: debug|info|warn|error
```

Resolution rules:
- `url` and `token` may be overridden by `--url` and `--token` flags respectively (flags > config file). This is the primary path for AI agents: `dodo-cli --url <api> --token <token> tasks list`.
- `--config` flag sets a custom config path; default is `~/.config/dodo/config.toml`.
- Missing config file is not fatal (flags can supply everything), but missing `url` or `token` when an API call is needed → user-facing error (exit 5) with a hint about how to create one (`dodo admin token create` or `POST /api/v1/tokens` via the web UI).
- For first-time setup, `dodo-cli init --url <api> --token <token>` writes a minimal `config.toml`; `dodo-tui` does the same on first launch when no config exists (prompting interactively, or erroring in non-interactive mode).

`internal/clientconfig.Load(flags)` returns a `ClientConfig` struct used by both `dodo-cli` and `dodo-tui`. The token is sent as `Authorization: Bearer <token>`; the server scopes all returned data to the token owner (see §0.5 scoping rule below).

### 0.7 Git conventions — Conventional Commits

All commits MUST follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types**: `feat`, `fix`, `refactor`, `test`, `chore`, `docs`, `ci`, `build`, `perf`, `style`.

**Scope**: the phase or component area (e.g. `db`, `auth`, `api`, `recurrence`, `scheduler`, `notify`, `telegram`, `web`, `i18n`, `tui`, `cli`, `admin`, `docker`, `ci`).

**Subject**: imperative mood, lowercase, no trailing period, max 72 chars.

**Body**: detailed description of what and why — wrap at 72 chars, explain rationale and key decisions, mention any breaking changes. Multi-paragraph is encouraged.

**Footer**: `BREAKING CHANGE: <description>` for breaking changes; `Closes #N` / `Refs #N` for issue references.

**Per-phase commit strategy**: each implementation phase produces **multiple focused commits** (not one giant commit per phase). Commit logically grouped chunks of work as they are completed within a phase. Suggested commit sequence per phase:

```
feat(db): add initial schema with migration 0001

  Create users, api_tokens, sessions, tasks, task_completions, and
  audit_log tables. UUIDv7 primary keys, RFC3339 UTC timestamps,
  per-user telegram config columns (encrypted bot token).

  All recurring task recurrence columns are nullable; one-off tasks
  leave them NULL. Partial indexes on pending and completed tasks
  speed up list queries.

feat(db): add migration runner with embedded SQL files

  Embed migrations/*.sql via go:embed, sort by numeric prefix, apply
  in a transaction, record version in schema_migrations table.
  Idempotent on re-open.

feat(store): implement user repository with telegram helpers

  CRUD plus GetByEmail, GetByTelegramWebhookSecret, SetTelegramConfig,
  SetTelegramChatID, ClearTelegramConfig. All methods take context
  and scope by user_id where applicable.
```

The `feat` type is used for new functionality, `fix` for bugs, `refactor` for non-behavioral changes, `test` for test-only additions, `chore` for tooling/config, `ci` for CI changes, `build` for build system, `docs` for documentation, `perf` for performance, `style` for formatting.

**Commit message validation**: a GitHub Actions job (in `ci.yml`, see Phase 16) lints commit messages on push and PR using a conventional-commits checker (e.g. `commit-lint` action or a small script). Failing the check blocks the PR from merging.

---

## Phase 1 — Scaffold & tooling

**Files:** `go.mod`, `mise.toml`, `.golangci.yaml`, `.gitignore`, `.editorconfig`, `.commitlintrc.yml` (conventional commit config), `cmd/dodo/main.go`, `cmd/dodo-cli/main.go`, `cmd/dodo-tui/main.go` (stubs), `internal/config/config.go`.

**Tasks:**
1. `go mod init github.com/mtzanidakis/dodo`.
2. `mise.toml`: pin `go = "1.26.4"`, `golangci-lint = "2.12.2"`, `node = "24"`. Define tasks: `build-server` (`go build ./cmd/dodo`), `build-cli` (`go build ./cmd/dodo-cli`), `build-tui` (`go build ./cmd/dodo-tui`), `build-all`, `run`, `test`, `lint`, `web:build`, `web:dev`, `tidy`.
3. `.golangci.yaml` (latest v2 schema) with the linters listed in 0.4.
4. `.commitlintrc.yml` (repo root, auto-detected by commitlint): conventional-commits config defining allowed types (`feat,fix,refactor,test,chore,docs,ci,build,perf,style`), scopes (`db,auth,api,recurrence,scheduler,notify,telegram,web,i18n,tui,cli,admin,docker,ci,crypto,config`), subject max-length 72, body max-line-length 72, require footer for breaking changes.
5. `cmd/dodo/main.go`: dispatch on `os.Args[1]` to `serve`/`admin`; print usage otherwise. `cmd/dodo-cli/main.go` and `cmd/dodo-tui/main.go`: minimal stubs printing "not implemented" and `--help` flag placeholders. Each branch/program just prints "not implemented" for now.
6. `internal/config.Load()` parsing the env table in §0.5 with `os.Getenv` + helpers for the `dodo` binary (server + admin); validate locale/timezone with `time.LoadLocation`. Separately, `internal/clientconfig.Load()` loading `~/.config/dodo/config.toml` (via `github.com/pelletier/go-toml/v2`) and merging `--url`/`--token`/`--config` flags for `dodo-cli`/`dodo-tui` (see §0.6). Client binaries never call `os.Getenv`.
7. `.gitignore`: `/internal/web/dist`, `/data`, binaries (`/dodo`, `/dodo-cli`, `/dodo-tui`), `*.sqlite`, `*.db-*`.

**Done when:** `mise run build-all` compiles all three binaries; `mise run lint` passes; `dodo serve` prints "not implemented"; `dodo-cli --help` and `dodo-tui --help` print usage.

---

## Phase 2 — Database, migrations, schema

**Files:** `internal/db/db.go`, `internal/migrations/*.sql`, `internal/migrations/migrations.go`.

**Tasks:**
1. `internal/db.Open(path)` returns `*sql.DB` using `modernc.org/sqlite`; set pragmas `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`.
2. Migration runner:
   - `schema_migrations(version INT PRIMARY KEY, applied_at TEXT NOT NULL)`.
   - Embed `internal/migrations/*.sql` via `//go:embed`, sort by filename numeric prefix, run in a transaction, record version.
   - On open, run all pending migrations.
3. Migration `0001_init.sql` creating schema (see §A below for full DDL; tables: `users`, `api_tokens`, `sessions`, `tasks`, `task_completions`, `audit_log`).
4. `internal/models/types.go`: Go structs with `db` tags matching columns (use a small `rows.Scan` helper or `sqlx`-free manual scan; prefer hand-written scans to avoid a dep).

**Done when:** opening a fresh `:memory:` DB applies all migrations idempotently; a re-open runs zero migrations; tests cover "applies in order" and "is idempotent".

---

## §A — Database schema (migration `0001_init.sql`)

```sql
CREATE TABLE users (
    id                   TEXT PRIMARY KEY,
    email                TEXT NOT NULL,
    password_hash        TEXT NOT NULL,
    role                 TEXT NOT NULL DEFAULT 'user',          -- 'admin' | 'user'
    display_name         TEXT NOT NULL DEFAULT '',
    timezone             TEXT NOT NULL DEFAULT 'Europe/Athens',  -- IANA name
    locale               TEXT NOT NULL DEFAULT 'en',           -- 'en' | 'el'
    theme                TEXT NOT NULL DEFAULT 'system',       -- 'system' | 'light' | 'dark'
    -- per-user Telegram bot config (each user runs their own bot)
    telegram_bot_token    TEXT,                                   -- AES-256-GCM encrypted bot token (nullable = telegram disabled for this user)
    telegram_allowed_user_ids TEXT,                                -- comma-separated Telegram user IDs allowed to interact with the bot
    telegram_chat_id      TEXT,                                    -- the chat_id to send notifications to (set when an allowed user sends /start)
    telegram_chat_user_id TEXT,                                    -- the Telegram user_id that linked (for verification/re-link)
    telegram_configured_at TEXT,                                   -- when bot token was last saved/validated
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    deleted_at           TEXT
);

-- allows re-using an email after a soft delete
CREATE UNIQUE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;

CREATE TABLE api_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_prefix TEXT NOT NULL,                 -- first 12 chars shown in UI
    token_hash   TEXT NOT NULL UNIQUE,          -- sha256 hex of full token
    last_used_at TEXT,
    expires_at   TEXT,                          -- optional expiry; NULL = never
    created_at   TEXT NOT NULL,
    revoked_at   TEXT
);

CREATE TABLE sessions (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash    TEXT NOT NULL UNIQUE,         -- sha256 hex of session cookie value
    user_agent    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    expires_at    TEXT NOT NULL
);

CREATE TABLE tasks (
    id                    TEXT PRIMARY KEY,
    user_id               TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title                 TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    priority              TEXT NOT NULL DEFAULT 'normal',     -- 'low' | 'normal' | 'high'
    kind                  TEXT NOT NULL DEFAULT 'oneoff',      -- 'oneoff' | 'recurring'
    due_at                TEXT NOT NULL,                       -- RFC3339 UTC
    duration_minutes      INTEGER NOT NULL DEFAULT 0,
    completed_at          TEXT,                                -- null until completed
    -- recurrence rule (NULL columns == not recurring)
    recurrence_freq      TEXT,                                 -- 'daily' | 'weekly' | 'monthly' | 'yearly'
    recurrence_interval  INTEGER NOT NULL DEFAULT 1,
    recurrence_by_day    TEXT,                                 -- comma list of weekdays 'MO,TU,WE' for weekly
    recurrence_by_month_day INTEGER,                          -- for monthly (1..31)
    recurrence_end_at    TEXT,                                 -- stop after this date (inclusive)
    last_notified_at     TEXT,                                 -- last reminder sent
    snoozed_until        TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

CREATE INDEX idx_tasks_user_due ON tasks(user_id, due_at) WHERE completed_at IS NULL;
CREATE INDEX idx_tasks_user_completed ON tasks(user_id, completed_at);

-- historical copies of recurring occurrences that were completed
CREATE TABLE task_completions (
    id            TEXT PRIMARY KEY,
    task_id       TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    priority      TEXT NOT NULL,
    due_at        TEXT NOT NULL,           -- the occurrence's due date
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
    meta        TEXT,                       -- JSON blob
    created_at  TEXT NOT NULL
);
```

---

## Phase 3 — Domain models & store layer

**Files:** `internal/models/{user,task,token,session}.go`, `internal/models/errors.go`, `internal/store/{store,users,tasks,tokens,sessions,completions}.go`, plus `*_test.go`.

**Tasks:**
1. Define structs and value enums (`Priority`, `TaskKind`, `Role`, `Theme`, `Locale`, `RecurrenceFreq`) with `String()`/`Parse()` and DB-safe values.
2. `store.Store` aggregates `*users`, `*tasks`, etc. for DI; constructor takes `*sql.DB`.
3. Implement CRUD + domain methods:
   - `Users.Create/GetByEmail/GetByID/Update/SoftDelete/List`
   - `Users.SetTelegramConfig(ctx, userID, token, allowedUserIDs)` / `GetTelegramConfig(ctx, userID)` / `ClearTelegramConfig(ctx, userID)` / `SetTelegramChatID(ctx, userID, chatID, chatUserID)` / `ListTelegramEnabled(ctx)` (users with a bot token set — used to start the long-pollers at boot)
   - `Tasks.Create/Get/List(filter)/Update/Complete/Delete`
   - `Tasks.AdvanceRecurring(ctx, id)` — see §B.
   - `Tasks.ListDue(ctx, now, maxRows)` — used by scheduler (completed_at IS NULL AND due_at <= now AND (snoozed_until IS NULL OR snoozed_until <= now) AND (last_notified_at IS NULL OR last_notified_at <= now - repeat)).
   - `Tokens.Create/Hash/List/Revoke/LookupByHash/Touch`
   - `Sessions.Create/Lookup/Expire/DeleteExpired`
4. Helper `internal/db/time.go`: `FormatUTC(t) string`, `ParseUTC(s) time.Time` using `time.RFC3339`.
5. Tests cover every method with `:memory:` DB.

**Done when:** `go test ./internal/store/...` is green and covers all methods including recurring-advance edge cases (skipped occurrences, end_at reached, by_day wrap).

---

## §B — Recurring task completion algorithm

When a recurring task `T` is completed at time `now`:

1. Insert a row into `task_completions` capturing `T.title`, `T.priority`, `T.due_at` (the occurrence that was due), `now` as completed_at, `T.user_id`.
2. Compute `next := recurrence.NextOccurrence(T.rule, T.due_at)` (strictly-after `T.due_at`).
3. If `next` is zero or `next > T.recurrence_end_at` (if set):
   - Set `T.completed_at = now`, `T.kind = 'oneoff'`, clear recurrence columns. (Series ended; row stays as a completed record.)
   - Optionally also write a `task_completions` row for this final state — done in step 1 already.
4. Else:
   - Keep `T.completed_at = NULL`.
   - Fast-forward: advance `next` to the first occurrence strictly after `now` (missed occurrences are skipped; no grace period in v1).
   - Update `T.due_at = next`, `T.last_notified_at = NULL`.
5. Return the updated task.

One-off completion: set `completed_at = now`, leave everything else. No `task_completions` row needed (the task row itself records it), but insertion is allowed for uniform history queries — keep it **only for recurring** to avoid duplication.

---

## Phase 4 — Auth, sessions, API tokens, middleware

**Files:** `internal/auth/{argon2.go,session.go,token.go,middleware.go}`, `internal/api/server.go`, `internal/api/auth_handlers.go`, tests.

**Tasks:**
1. `auth.HashPassword(pw) → hash`; `auth.VerifyPassword(pw, hash) → bool`. argon2id with sane params (memory 64 MiB, iterations 3, parallelism 2).
2. Tokens: `auth.GenerateAPIToken() (full string, prefix, hash)`; full format `dodo_<32 base62>`; prefix = first 12 chars of the full token (matches the schema's `token_prefix` comment); `hash = sha256 hex of full`. Store only prefix + hash.
3. Sessions: session cookie name `dodo_session`, value `dodo_<48 base62>`; store `sha256` in `sessions`; cookie is `HttpOnly`, `SameSite=Lax`, `Secure` when the request arrives over https (`r.TLS != nil` or `X-Forwarded-Proto: https`), path `/`, max-age 30 days.
4. Middleware:
   - `AuthSession` — load user from session cookie; sets `ctx` user (the session's owner).
   - `AuthBearer` — load user from `Authorization: Bearer dodo_…` by looking up the per-user `api_tokens` row by `token_hash`; the resolved user is the token's owner. Update `last_used_at`. The bearer user is the only user the request may act on — handlers scope all queries to it (see §0.4 "Per-user data scoping").
   - `RequireUser`, `RequireAdmin` (admin only affects admin-only routes such as future admin endpoints; it does NOT widen task visibility).
   - On overlap (both present) prefer bearer for `/api/*` JSON routes and session for browser routes; document the rule in code.
   - A revoked, expired, or nonexistent token → `401`; an expired/missing session → redirect to `/login` for browser, `401` for `/api/*`.
   - A session or token whose user is soft-deleted (`deleted_at` set) → `401`, and the session/token is invalidated.
   - `CSRF` — double-submit protection for session-authenticated non-GET requests: issue a `dodo_csrf` cookie (random 32 bytes, not HttpOnly); the layout template exposes it as `<meta name="csrf-token">` and htmx sends it as `X-CSRF-Token` via `hx-headers` on `<body>`; missing/mismatching header → `403`. Bearer-token requests are exempt (no cookie auth involved).
5. Login handler `POST /api/v1/auth/login {email,password}` → sets session cookie, returns `User` JSON. In-memory failure rate limiter keyed by (IP, email): 10 failures / 15 min → `429` + audit entry. `POST /api/v1/auth/logout`. `GET /api/v1/auth/me`.
6. Audit logging helper `internal/store/audit.go` called by sensitive ops (login, token create/revoke, user create/delete, password change, telegram config save/clear, telegram chat link).

**Done when:** tests cover login success/failure, login rate-limit lockout, bearer + session extraction, token prefix uniqueness, expired-session rejection, CSRF rejection (session-auth POST without a valid `X-CSRF-Token` → 403), rejection of soft-deleted users, and that a token owned by user A cannot read user B's tasks (returns 404 on `/api/v1/tasks/{B_task_id}` and only lists A's tasks).

---

## Phase 5 — HTTP API server implementation

**Files:** `internal/api/{server,routes,tasks,users_profile,tokens,telegram,notfound}.go`, middleware wiring.

**Tasks:** build `ServeMux` (Go 1.22+ method patterns). All routes under `/api/v1` for non-browser clients; browser pages mount under `/` (Phase 11).

Routes (JSON in/out, errors as `{"error":{"code":"...","message":"..."}}`):
```
POST   /api/v1/auth/login
POST   /api/v1/auth/logout
GET    /api/v1/auth/me

GET    /api/v1/me
PATCH  /api/v1/me                      {display_name, timezone, locale, theme}
POST   /api/v1/me/password             {current_password, new_password}
POST   /api/v1/me/telegram              {bot_token, allowed_user_ids} -> validates token via getMe, starts the long-poller, returns {bot_username, status}
GET    /api/v1/me/telegram                                            -> {bot_username, allowed_user_ids, chat_id, chat_user_id, configured_at, status}
PATCH  /api/v1/me/telegram              {bot_token?, allowed_user_ids?}  (partial update; re-validates + restarts the poller if bot_token changes)
DELETE /api/v1/me/telegram                                               (clears all telegram config; stops the poller)
POST   /api/v1/me/telegram/test         -> sends a test notification to the linked chat_id

GET    /api/v1/tokens
POST   /api/v1/tokens                   {name} -> {id,name,prefix,token,created_at}  (token returned once)
DELETE /api/v1/tokens/{id}

GET    /api/v1/tasks?view=list|calendar&filter=pending|completed|all&from=&to=&priority=
POST   /api/v1/tasks
GET    /api/v1/tasks/{id}
PATCH  /api/v1/tasks/{id}
POST   /api/v1/tasks/{id}/complete
POST   /api/v1/tasks/{id}/snooze        {until}
DELETE /api/v1/tasks/{id}

GET    /api/v1/completions?from=&to=

GET    /ws                             (websocket; session cookie or bearer token on the handshake; unauthenticated -> close 4401)
GET    /healthz                        (no auth; 200 "ok"; used by the Docker healthcheck)
```
Pagination: `?cursor=&limit=` (default 50, max 200) using `due_at`/`id` encoding.

**Tasks:**
1. `POST /tasks`: validate title non-empty, `due_at` parseable, priority in set, recurrence rule valid. On recurring, set `kind='recurring'` and compute `due_at` only if not supplied (else first occurrence is the supplied due_at).
2. `PATCH /tasks/{id}`: only mutable fields may be touched (not `completed_at` directly). For recurring tasks, changing recurrence fields recomputes `due_at` only if explicitly requested via `recalculate=1` query.
3. `POST /tasks/{id}/complete` → applies §B; returns updated task (or final one-off record) and the new `task_completions` row id.
4. List filter logic in `store` (param struct), pushed down to SQL `WHERE` with the partial indexes. **All task and completion queries are scoped `WHERE user_id = <auth_user.ID>`; ignore any `user_id` in the request body.** Listing never returns another user's tasks; `GET/PATCH/DELETE /tasks/{id}` returns 404 when the task's `user_id ≠ auth_user.ID` (do not 403 — avoids leaking existence).
5. Calendar view returns nested `days[]` with per-day `tasks[]` for a given month (server computes using available `loc`).
6. Error mapping: validation → 400; not found → 404; unauthorized → 401; forbidden (not owner/admin) → 403; conflict → 409.
7. `/api/v1/tokens` routes are scoped to the auth user: `GET` lists only their tokens, `POST` creates a token owned by them, `DELETE /tokens/{id}` returns 404 unless the token's `user_id` matches the auth user. (The `dodo admin` CLI bypasses this via direct DB — see Phase 9.)
8. Middleware wrapping every route: panic recovery (slog error with stack, 500 JSON response) and request logging (method, path, status, duration at debug level).

**Done when:** `httptest`-based tests cover create/update/complete for one-off and recurring (including end_at), list filters, token create/revoke (scoped to the auth user), profile update, password change, telegram config save/validate/clear (against a stubbed Bot API `httptest` server for `getMe`). Browser session + bearer both verified. Cross-user isolation test: user A's token returns 404 on user B's task id and never lists B's tasks.

---

## Phase 6 — Recurrence engine

**Files:** `internal/recurrence/recurrence.go`, `recurrence_test.go`.

**API:**
```go
type Rule struct {
    Freq        RecurrenceFreq   // daily|weekly|monthly|yearly
    Interval    int              // >= 1
    ByDay       []time.Weekday   // weekly only; empty == weekday of Base
    ByMonthDay  int              // monthly only; 0 == day-of-month of Base; 31 clamps to last day
    EndAt       time.Time         // zero == no end
}
func NextOccurrence(r Rule, base time.Time, after time.Time, loc *time.Location) time.Time
func Occurrences(r Rule, base time.Time, from, to time.Time, loc *time.Location) []time.Time
```
All occurrence math operates in the user's `loc`; result converted to UTC only at the DB edge. DST handled by `time.Date`.

Rules:
- daily: `base + Interval days`, advance while `<= after`.
- weekly: if `ByDay` empty then use `base.Weekday()`; iterate forward day-by-day across the `Interval`-week window; only emit days matching `ByDay`.
- monthly: `ByMonthDay` (or base day); clamp to last valid day of month; add `Interval` months; skip months where day invalid only if `ByMonthDay` was explicitly set beyond the month length is desired as "skip" (document: skip).
- yearly: same month+day + `Interval` years.

**Done when:** table-driven tests covering weekly with multi-day list, monthly end-of-month clamping, yearly across leap years, `Interval > 1`, DST fold, `EndAt` cutoff, `from>/to<` window queries.

---

## Phase 7 — Scheduler & notification dispatcher

**Files:** `internal/scheduler/scheduler.go`, `internal/notify/notify.go`, `internal/notify/telegram.go`, `internal/ws/hub.go`.

**Tasks:**
1. `ws.Hub`: per-user set of `*websocket.Conn`; `Subscribe(userID, conn)`, `Publish(userID, event)`; non-blocking sends with buffered channel; dead clients removed on send error. Events typed `{type, payload}` JSON.
2. `notify` sends telegram messages via `internal/telegram` (see Phase 8): `telegram.Registry` caches one `*telegram.Client` per user (bot token stored encrypted in `users.telegram_bot_token`, decrypted at read time via `internal/crypto`). `Send(ctx, userID, chatID, text, keyboard) error` calls `sendMessage`; reminder messages attach an inline keyboard with a single localized "Complete" button (`callback_data = "complete:<task_id>"`). `Invalidate(userID)` drops the cached client (called when the user changes/clears their token). Handle 429 with a bounded retry honoring `retry_after`. MarkdownV2 escaping via a small helper.
3. `notify.Dispatcher`:
   - For each due task (from `store.Tasks.ListDue`), render a message (title, due-local-time, priority icon) using `i18n.T` with the user's locale.
   - If `user.telegram_bot_token != nil AND user.telegram_chat_id != nil` → enqueue telegram send via `telegram.Registry.Send(ctx, userID, chatID, text, completeKeyboard(taskID))` (registry decrypts the token internally).
   - Always `ws.Hub.Publish(userID, "task.due", {task})`.
   - On success set `task.last_notified_at = now` (also if telegram fails but ws has subscribers? keep it simple: set only if at least one channel succeeded; record a `notify.log` audit entry on persistent telegram failures).
4. `scheduler.Run(ctx, interval)`: `time.Ticker` loop calling `dispatcher.Tick(ctx)`. Shutdown on `ctx.Done()`. Even spacing: run the scan in a worker so a slow scan doesn't starve ticks. Each tick also calls `store.Sessions.DeleteExpired(ctx, now)`.
5. Repeat intervals from priority: low=2h, normal=1h, high=20m (single source of truth `models.PriorityReminderInterval`).
6. Startup: ensure a single scheduler goroutine; log scan duration at debug.

**Done when:** unit tests use a fake clock + fake telegram client; assert ordering of notifications, repeat throttling by priority, last_notified_at update, and that a completed recurring task no longer notifies.

---

## Phase 8 — Telegram per-user bot integration (long polling)

**Files:** `internal/telegram/{client.go,types.go,registry.go,poller.go}`, `internal/notify/telegram.go` (dispatcher glue), `internal/api/telegram.go` (config routes).

Each user configures their **own** Telegram bot (created via BotFather) — there is no global bot. The server runs inside a private Tailscale tailnet and is not publicly reachable, so **webhooks are not possible; all updates arrive via long polling** (`getUpdates`). No Telegram library — a hand-rolled Bot API client over `net/http` + `encoding/json`.

**Tasks:**
1. **Client** (`internal/telegram/client.go`): `Client{token, httpClient}` with typed methods `GetMe`, `SendMessage` (chat_id, text, parse_mode, optional inline keyboard), `GetUpdates` (offset, timeout — long poll 50s), `AnswerCallbackQuery`, `EditMessageText`. Bot API errors (`ok:false`) map to a typed error carrying `error_code` + `description`; on 429 honor `parameters.retry_after` with a bounded retry (max 3). `types.go` defines only the fields we use: `Update`, `Message`, `CallbackQuery`, `User`, `Chat`, `InlineKeyboardMarkup/Button`.
2. **Registry** (`internal/telegram/registry.go`): `map[userID]*Client` guarded by `sync.RWMutex`. `GetOrCreate(ctx, userID)` decrypts the stored token via `internal/crypto` and caches; `Invalidate(userID)` drops the entry (called on token change/clear).
3. **Poller** (`internal/telegram/poller.go`): one goroutine per configured user looping `GetUpdates(offset, 50s)`. `Pollers.Start(userID)` / `Stop(userID)` / `StartAll(ctx)` — `StartAll` runs at server startup for every user returned by `store.Users.ListTelegramEnabled`. Token change = `Stop` + `Invalidate` + `Start`. Exponential backoff on repeated errors (1s → 60s cap); a failing poller logs and retries, never crashes the server. All pollers stop on `ctx.Done()`.
4. **Update handling** (in the poller):
   - `message` from a sender not in `telegram_allowed_user_ids` → reply with localized "unauthorized" message, store nothing.
   - `/start` (or any first message) from an allowed sender → store `telegram_chat_id = message.Chat.ID` and `telegram_chat_user_id = sender.ID` via `store.Users.SetTelegramChatID`, reply with localized "linked" confirmation, publish `ws` event `telegram.linked`.
   - Other messages → reply with localized help hint.
   - `callback_query` with data `complete:<task_id>`: verify the sender is allowed and the task belongs to the poller's user (404-style silent notice otherwise); apply §B via the store; `AnswerCallbackQuery` with a localized "completed" toast; `EditMessageText` on the original reminder to append a localized "(completed)" marker and drop the keyboard; publish `ws` event `task.completed`. Already-completed or missing task → `AnswerCallbackQuery` with a localized notice, no state change.
5. **API config flow** (`internal/api/telegram.go`):
   - `POST /api/v1/me/telegram {bot_token, allowed_user_ids}`: validate token via `GetMe`, persist `telegram_bot_token` (encrypted via `internal/crypto`), `telegram_allowed_user_ids`, `telegram_configured_at`; start (or restart) the user's poller. Return `{bot_username, status: "configured"}`. If `telegram_chat_id` is already set (re-configuring), preserve it.
   - `GET /api/v1/me/telegram`: return current config (without echoing the bot token — redacted).
   - `PATCH /api/v1/me/telegram {bot_token?, allowed_user_ids?}`: if `bot_token` changes, re-validate + re-encrypt + restart the poller. If only `allowed_user_ids` changes, just persist.
   - `DELETE /api/v1/me/telegram`: stop the poller, invalidate the cached client, clear all telegram fields.
   - `POST /api/v1/me/telegram/test`: if `telegram_chat_id` is set, send a test message via the registry; return success/failure.
6. **Bot token security**: `telegram_bot_token` is stored **encrypted** at rest using AES-256-GCM (`internal/crypto` package). The key comes from `DODO_ENCRYPTION_KEY` (required env var). Format in DB: `base64(nonce(12) || ciphertext || gcm_tag)` as a single `TEXT` column. Encryption/decryption happens transparently in the store layer — callers never see ciphertext. The `internal/crypto` package exposes `Encrypt(plaintext) (string, error)` and `Decrypt(ciphertext) (string, error)`; a fresh random nonce is generated per encryption. If `DODO_ENCRYPTION_KEY` is missing or invalid at startup, `dodo serve` exits with a clear error. Key rotation is manual: re-encrypt all tokens via a `dodo admin crypto rotate` command (future scope, document in README).

**Done when:** tests run against a stubbed Bot API `httptest` server: user saves bot token → `GetMe` validated → poller receives `/start` from an allowed sender → chat_id stored → a due reminder is sent with the "Complete" button → the callback completes the task, answers the callback, and edits the original message. Rejected when the sender is not in `allowed_user_ids`. Token change restarts the poller with the new token; `DELETE` stops it. `internal/crypto` tests cover round-trip encrypt/decrypt, nonce randomness, tamper detection (GCM auth), and that the stored DB value is not the plaintext.

---

## Phase 9 — `dodo admin` CLI (direct DB)

**Files:** `internal/admin/*.go`, wiring in `cmd/dodo/main.go` (the `admin` subcommand of the `dodo` binary).

The admin CLI operates directly on the DB (no HTTP, no token). It manages **users and api_tokens** (create/list/revoke per user) but does not modify tasks — task data stays reachable only by each user's own credentials (per §0.4 scoping). A token minted by `dodo admin token create --email …` grants access only to that user's tasks/reminders; there is no admin API token that sees all users' data. Telegram bot config is per-user via the API/web UI; it is not managed through the admin CLI (admin can see/clear `telegram_*` columns via `dodo admin user update` if ever needed).

Subcommands (all require `DODO_DATABASE_PATH` to point at the same DB the server uses; stop server or use WAL-safe concurrent access — document that running admin concurrently with the live server is safe under WAL):

```
dodo admin user create      --email --password [--display-name] [--role=user|admin]
dodo admin user list
dodo admin user get         --id | --email
dodo admin user update      --email [--display-name] [--role] [--active]
dodo admin user delete      --email
dodo admin user reset-password --email --password
dodo admin token create     --email --name           (prints full token once)
dodo admin token list       --email
dodo admin token revoke     --id | --prefix
dodo admin migrate          (force-run pending migrations)
dodo admin version
```
Output defaults to JSON; `--pretty` prints a human table. `--yes` skips confirmation prompts for destructive ops.

Empty DB bootstrap: `dodo admin user create --email admin@… --password … --role admin` creates the first admin; if no users exist and `--role admin` is omitted, default the first user to `admin` with a warning.

**Done when:** tests run each command against `:memory:` DB and assert DB state + output JSON.

---

## Phase 10 — `dodo-cli` binary (AI-agent CLI)

**Files:** `internal/cli/*.go`, `internal/clientconfig/*.go`, `cmd/dodo-cli/main.go` (HTTP client; url + token from `~/.config/dodo/config.toml` or flags — no env vars, see §0.6).

The `dodo-cli` binary is a separate program; commands are top-level (not a subcommand). JSON to stdout by default, `--pretty` for humans:
```
dodo-cli init               [--url] [--token] [--config]  (writes ~/.config/dodo/config.toml)
dodo-cli me
dodo-cli tasks list         [--filter=pending|completed|all] [--priority=] [--from=] [--to=] [--view=list|calendar] [--limit=] [--cursor=]
dodo-cli tasks get          <id>
dodo-cli tasks create       --title --due --priority [--desc] [--repeat=freq:interval:byday:byday] [--repeat-end=]
dodo-cli tasks update       <id> [--title] [--due] [--priority] [--desc] [--recalculate]
dodo-cli tasks complete     <id>
dodo-cli tasks snooze       <id> --until
dodo-cli tasks delete      <id>
dodo-cli completions list   [--from=] [--to=]
dodo-cli tokens list
dodo-cli tokens create      --name            (prints full token once)
dodo-cli tokens revoke      <id>
dodo-cli --pretty           (global flag)
dodo-cli --raw              (default; show keys even when null)
```
Dates accept RFC3339 or `YYYY-MM-DDTHH:MM` interpreted in the configured locale timezone, or `now`, `now+30m`, `tomorrow 10:00` (free-form via a small `parseHumanTime` helper backed by stdlib only).

Token and url resolution: `--token`/`--url` flags override `~/.config/dodo/config.toml` (see §0.6). Missing token → exit 5 with a stderr hint about how to create one (`dodo admin token create` or `POST /api/v1/tokens`). The token is sent as `Authorization: Bearer <token>`; server scopes all returned data to that token's owner only (see §0.4).

`dodo-cli init --url <api> --token <token>` writes a minimal `config.toml` for first-time setup.

`--json` is implicit for agents; always include machine-readable exit codes: 0 ok, 1 generic error, 2 usage, 4 not found, 5 auth failure. Errors print to stderr as `{"error":{"code","message"}}` and stdout stays empty on failure so pipelines stay clean.

**Done when:** CLI tests hit an in-process `httptest` server (real handlers) covering list/create/complete/snooze and JSON shape.

---

## Phase 11 — `dodo-tui` binary (Terminal UI)

**Files:** `internal/tui/*.go`, `internal/clientconfig/*.go`, `cmd/dodo-tui/main.go` (bubbletea program; url + token from `~/.config/dodo/config.toml` or flags — no env vars, see §0.6).

The `dodo-tui` binary is a separate program with no subcommands; flags only (`--token`, `--url`, `--help`).

**Tasks:**
1. `internal/tui/client.go`: thin typed wrapper around the `/api/v1/*` endpoints (share request/response types with `internal/cli` via a small `internal/apiclient` package if convenient; otherwise duplicate — keep deps minimal).
2. `internal/tui/model.go`: bubbletea `Model` with views:
   - **List view** (default): pending + future tasks grouped by day (uses user TZ from `/api/v1/me`), columns = priority mark, title, due-local-time. Keys: `↑/↓` move, `Enter` open, `c` complete, `n` new task form, `s` snooze prompt, `d` delete (confirm), `t` toggle completed-only filter, `/` filter, `?` help, `q` quit.
   - **Detail view**: full task info + description; `Esc` back.
   - **New/edit form**: title, due (`parseHumanTime` reused from `internal/cli`), priority (cycle), recurrence fields; `Ctrl+S` save, `Esc` cancel.
   - **Tokens view**: list/create (show full token once in a modal)/revoke.
   - **Account view**: profile fields (display_name, timezone, locale, theme), change-password form, telegram bot config (bot token input, allowed user ids, status showing linked bot username + chat_id).
3. Real-time: open a `/ws` connection (bearer token in the `Authorization` header on the handshake); incoming `task.*` JSON events refresh the current list (debounced 250 ms). Reconnect with backoff on close.
4. Styling: `lipgloss` theme honoring the user's `theme` (system → detect via `os.Getenv("COLORTERM")`/`NO_COLOR`; dark default).
5. Status bar: connection state, current filter, count. Help overlay on `?`.
6. Exit codes mirror `dodo-cli`: 0 ok, 1 error, 2 usage, 5 auth failure. Errors render as a red modal, not stdout JSON (TUI owns the terminal).

**Done when:** `dodo-tui` connects with a token (config file or `--token`), renders the pending list, can create + complete + snooze + delete tasks, switch to the calendar/list filter, view tokens + account; websocket events update the list within ~1s. Tests cover the pure update functions (filtering, grouping, recurrence parse) and a `teatest`-based smoke test for the list → complete flow against an in-process `httptest` server.

---

## Phase 12 — Web frontend (htmx + Alpine + Tailwind)

**Files:** `web/package.json`, `web/tailwind.config.js`, `web/postcss.config.js`, `web/src/*.css`, `web/src/app.js` (websocket client + theme + csrf wiring), vendor htmx + alpine into `web/src/vendor/`, `internal/web/{embed.go,dist/,templates/*.html,handlers.go,assets.go}`, build wiring in `mise.toml`.

**Build:**
- Tailwind CLI compiles `web/src/app.css → internal/web/dist/app.css` with content scanning `internal/web/templates/**/*.html`.
- htmx + alpine minified JS are vendored (downloaded by an `npm` script) into `internal/web/dist/vendor.js`; `web/src/app.js` is copied alongside.
- `internal/web/embed.go`: `//go:embed all:dist` (go:embed cannot reach outside the package directory, hence dist lives under `internal/web/`), plus a generated `internal/web/dist/version.txt` written at build time holding `${git sha}-${build unix}`. `AssetsFS()` returns `(fs.FS, version)`.
- Server serves `/static/{version}/...` (immutable, `Cache-Control: max-age=31536000, immutable`) and templates reference `/static/{version}/app.css` etc.

**Templates** (`html/template` with a shared `layout.html.tmpl` — sets `<meta name="csrf-token">` and `hx-headers` on `<body>` so htmx sends `X-CSRF-Token` on every request, see Phase 4):
- `auth/login.html.tmpl`, `auth/me` (/account), `tasks/index.html.tmpl` (list + calendar toggle), `tasks/_row.html.tmpl`, `tasks/_form.html.tmpl`, `tokens/index.html.tmpl`, `partials/*` for htmx swaps.

**Templates behavior (htmx):**
- List rows mutated via `hx-post`/`hx-patch` returning `_row.html.tmpl` fragments; complete button `hx-post /tasks/{id}/complete` targets the row.
- Browser pages never call `/api/v1/*`. A parallel mount `/ui/*` mirrors the task/token/profile operations and returns HTML fragments (`/ui/tasks`, `/ui/tasks/{id}/complete`, …); `/api/v1/*` stays JSON-only for programmatic clients. Both mounts share handler logic via internal service functions, not via HTTP dispatch.
- Real-time: `app.js` opens `/ws` (session cookie auth) and receives JSON events `{type,payload}` — the hub speaks JSON only, same schema the TUI consumes. Each event type maps to a debounced (250 ms) refresh of the affected container (`htmx.trigger('#task-list', 'refresh')` → `hx-get /ui/tasks?…`). No htmx-ws extension. Reconnect with exponential backoff.
- Alpine micro-state for theme select, mobile nav, optimistic complete animations.

**Pages:**
- `/login`, `/logout`, `/` (tasks list, default pending+future), `/?view=calendar`, `/?filter=completed` (past/completed), `/account` (profile: name, timezone, locale, theme, password change, telegram bot config (token, allowed user ids, bot status, linked chat), tokens list/create/revoke).
- Dark/light/system: Alpine reads `localStorage.theme`, applies `document.documentElement.classList` and `color-scheme`; `<meta name="color-scheme">` set server-side from user profile; "system" listens to `matchMedia('(prefers-color-scheme: dark)')`. Tailwind `darkMode: 'class'`.
- Top bar: dodo logo, view toggle (list/calendar), filter (pending/completed/all), "New task" button, theme select, account link.

**Done when:** a fresh `mise run web:build && dodo serve` renders list + calendar + account pages, completing a task swaps the row without full reload, theme select persists across reloads.

---

## Phase 13 — i18n (en + el)

**Files:** `locales/en.json`, `locales/el.json`, `internal/i18n/i18n.go`, `internal/i18n/embed.go`, usage in templates + telegram + CLI help.

**Design:**
- JSON flat `key.sub.key → string` with `{placeholder}` interpolation; plurality by `key.one`/`key.other` (only if needed; start simple).
- Loaded via `//go:embed locales/*.json`, selected by `user.locale`; fallback chain `user → cookie → Accept-Language → DODO_DEFAULT_LOCALE → en`.
- A `T(key, lang, args...)` function; templates use `{{t "task.due" .Lang}}`; telegram dispatcher uses it; CLI `--pretty` uses it for headings (optional; CLI raw output stays locale-neutral).
- All UI strings, telegram messages, validation error messages, CLI human help text come from these files (no hard-coded Greek/English in Go code).

**Done when:** every user-visible string renders in both `en` and `el`, `go test ./internal/i18n` checks missing-key reporting and interpolation; a test asserts no template references a key absent from both locale files.

---

## Phase 14 — Websocket real-time polish

Already scaffolded in Phase 7; here we wire the browser surface:
- Server-side events: `task.created`, `task.updated`, `task.completed`, `task.deleted`, `task.due`, `telegram.linked`, `tokens.updated`.
- The hub speaks JSON only: `{type,payload}` with ping/pong every 30s; browser (`app.js`) and TUI consume the same event schema.
- Browser: each event triggers a debounced `hx-get` refresh of the affected container (see Phase 12); reconnect with exponential backoff in `app.js`.
- Memory pressure: cap connections per user (default 5), evict oldest; log evictions.

**Done when:** two browser tabs update within ~1s when one completes a task; closing the tab frees the hub slot.

---

## Phase 15 — Dockerfile

**File:** `Dockerfile`.

Multi-stage:
- `FROM node:24-alpine AS web` → `mise run web:build` (or `npm ci && npm run build`).
- `FROM golang:1.26.4-alpine AS go` → `COPY internal/web/dist`, `go build -trimpath -ldflags "-s -w -X main.version=… -X main.commit=…" -o /out/dodo ./cmd/dodo` (only the serve+admin binary; `dodo-cli`/`dodo-tui` are end-user binaries, not shipped in the image). CGO disabled (`CGO_ENABLED=0`) so modernc.org/sqlite pure-Go works without a C toolchain.
- `FROM alpine:3.24` → `ca-certificates`, `tzdata`, `wget` (healthcheck), `mkdir /data`, copy `/out/dodo`, expose `8080/tcp`, `VOLUME ["/data"]`, `ENTRYPOINT ["dodo"]`, `CMD ["serve"]`. Healthcheck `wget -qO- localhost:8080/healthz || exit 1` (route defined in Phase 5).
- Label `org.opencontainers.image.source=https://github.com/mtzanidakis/dodo`.

Verification: `docker build` + `docker run -e DODO_DATABASE_PATH=/data/dodo.sqlite -p 8080:8080 ghcr.io/mtzanidakis/dodo` smoke-tests login page.

---

## Phase 16 — CI / GitHub Actions

**Files:** `.github/workflows/ci.yml`, `.github/workflows/docker.yml`, `.github/workflows/release.yml`.

### `ci.yml` — lint, tests, commit-message check (the gate)

- Triggers: pull_request to `main`, push to `main`.
- Jobs:
  1. **commitlint**: runs on node 24; uses `@commitlint/cli` + `@commitlint/config-conventional` with `.commitlintrc.yml`; checks all commit messages in the PR. Fails on non-conventional messages.
  2. **lint**: mise install; `mise run lint` (golangci-lint on all Go files).
  3. **test**: mise install; `go test -race -covermode=atomic ./...`; uploads coverage artifact. Also runs `go vet ./...` and `gofmt -l` check.
  4. **build**: mise install; `mise run build-all` + `mise run web:build` (verifies all three binaries and the frontend compile); cache `~/go/pkg/mod` and `~/.cache/go-build`.
- **Gating**: `ci.yml` must pass before `docker.yml` and `release.yml` can run. This is enforced via **workflow run gating** — `docker.yml` and `release.yml` use `workflow_run` triggers or `needs`/`jobs.<job>.if` against the `ci` check status. Specifically:
  - `docker.yml` triggers on push to `main` and tags `v*`, but the build/push job has `if: ${{ github.event_name == 'push' || (github.event_name == 'workflow_run' && github.event.workflow_run.conclusion == 'success') }}` or uses `needs: ci-passed` referencing the `ci.yml` workflow via a `workflow_run` event.
  - `release.yml` (triggered only on tags `v*`) similarly waits for `ci.yml` to report success on the same commit before running goreleaser.
  - Alternative simpler approach (preferred): `docker.yml` and `release.yml` both **re-run lint+test inline** as their first job, and the build/release job uses `needs: [lint-and-test]`. This avoids fragile cross-workflow references and works on tags (which don't trigger `workflow_run`). The redundant lint/test run on tags is acceptable overhead.

### `docker.yml` — build & push container image

- Triggers: push to `main`, tags `v*`.
- Guard: first job `lint-and-test` (same as `ci.yml` — mise install, lint, test, build-all) must pass.
- Steps (after guard): checkout, mise install, `go build ./cmd/dodo`, `web:build`, `docker/build-push-action` to `ghcr.io/mtzanidakis/dodo:${tag|sha}` with multi-arch `linux/amd64,linux/arm64` via `--platform` buildx.
- Tag `latest` on `v*`.

### `release.yml` — goreleaser cross-platform distributables

- Triggers: tags `v*`.
- Guard: first job `lint-and-test` (same as above) must pass.
- Steps (after guard): checkout, mise, goreleaser with `.goreleaser.yml`:
  - Build **two** binaries per `(os,arch)`: `dodo-cli` (`./cmd/dodo-cli`) and `dodo-tui` (`./cmd/dodo-tui`), for `linux/darwin/windows` × `amd64/arm64` (skip windows/arm64 if problematic; TUI on windows is best-effort).
  - `dist/` archives + `checksums.txt`, SBOM.
  - Changelog auto from commits since previous tag (conventional commits make this clean).
  - GitHub Release notes.
  - (The `dodo` server binary is NOT shipped here — users run it via the container.)

### Branch protection

- Configure `main` branch protection: require `ci.yml` (lint, test, build) to pass before merge; require conventional commits (commitlint job); require linear history; dismiss stale reviews; no force-push to `main`. Document in `AGENTS.md`.

**Done when:** a PR with a bad commit message or failing lint/test is blocked from merging; tagging `v0.1.0` passes all checks then produces a working GHCR image and a GitHub Release with `dodo-cli` + `dodo-tui` archives + checksums; a manual smoke test on a release artifact shows `dodo-tui --help` and `dodo-cli tasks list --help` working against the GHCR image.

---

## Phase 17 — Tests, polish, docs

**Tasks:**
1. Coverage push: aim ≥ 85% for `internal/store`, `internal/recurrence`, `internal/auth`, `internal/api`, `internal/scheduler`, `internal/notify`.
2. Add `AGENTS.md` documenting: build/test/lint commands (`mise run build|test|lint|web:build`), server env vars (§0.5), client config.toml format (§0.6), conventional commits convention (§0.7), how to run the server + admin bootstrap, how to set up `dodo-cli`/`dodo-tui` (write config.toml with `dodo-cli init`), how to add a migration, branch protection rules (Phase 16).
3. `README.md`: features screen (mini), quickstart (`docker run …`, `dodo admin user create`, `dodo-cli init --url … --token …`, `dodo-cli tasks list`, `dodo-tui`), server env var table link to §0.5, client config link to §0.6, three-binary overview table, license MIT. Include a backup section: the `/data` volume is the only state; online backup via `sqlite3 dodo.sqlite ".backup backup.sqlite"` or a Litestream pointer.
4. License header check (golangci license linter disabled; instead add `// SPDX-License-Identifier: MIT` to top of each `.go` file? — keep simple: rely on repo LICENSE; do not add headers unless desired). Decision: do not add file headers.
5. Final lint + `go vet` + `gofmt -l` clean; `mise run lint` passes in CI-equivalent mode.

---

## §C — acceptance checklist (the whole project)

- [ ] `dodo serve` boots, applies migrations, serves web UI + `/api/v1/*` + `/ui/*` + `/ws` + `/healthz`, and starts one Telegram long-poller per configured user.
- [ ] First admin bootstrap via `dodo admin user create --role admin`.
- [ ] User can log in, create a daily recurring task, see it in list and calendar, mark complete, see next occurrence appear and history under completions.
- [ ] Notifications fire on time and repeat at the priority's interval until completed (verified with fake clock tests).
- [ ] Each user configures their own Telegram bot token in the profile → long-poller starts → allowed user sends `/start` to the bot → chat_id stored → reminders arrive via the user's own bot with a "Complete" button that marks the task completed straight from Telegram.
- [ ] `dodo-cli tasks list` (with `--token <user_token>`) outputs that user's tasks only (never other users'), exit 0.
- [ ] `dodo-tui` shows pending tasks and lets the user complete them over the API.
- [ ] Dark/light/system theme switches and persists (web + TUI); `el` + `en` UI fully translated.
- [ ] `docker build` produces a working alpine image (contains the `dodo` binary with `serve` + `admin` only); tag `v0.1.0` triggers `ci.yml` → `docker.yml` + `release.yml`; `ci.yml` gates both (lint+test+build must pass); release artifacts include `dodo-cli` + `dodo-tui` for the supported platforms.

---

## Implementation order summary
Phases 1 → 17. Phases 6 (recurrence) and 5 (API) can be developed in parallel once 3 is done; Phase 11 (TUI) depends on 4 + 5; Phase 12 (web frontend) depends on 4 + 5; Phase 13 (i18n) wires into 12 + 7 + 8; Phase 14 (ws polish) depends on 12 + 7. Keep tests passing after each phase (`go test ./...`).