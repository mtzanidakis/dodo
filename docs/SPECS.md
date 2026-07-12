# dodo: a simple todo app

[github](https://github.com/mtzanidakis/dodo)

App for managing todo tasks and reminders. Should support one-off tasks at specific date/time and repeated tasks.
Tasks should be something like "Go to the supermarket today at 10:00", "Pay electric bill next Thursday at 17:00" etc.

Tasks should have low/normal/high priorities, defaulting to normal priority. Notifications are delivered via Telegram and as desktop browser notifications while the web app is open. The user should mark a task as completed; if not it should send new notifications every 2 hours (low priority), 1 hour (normal), 20 minutes (high). Tasks can be snoozed until a later time to silence reminders.

Single API for serving multiple clients (web/tui/cli).

The service is self-hosted inside a private Tailscale tailnet; it is never publicly accessible.

The project will be MIT-licensed.

## Interfaces

- Web app for viewing and managing tasks in a list view and calendar view, showing pending/future tasks by default with separate option for viewing past/completed tasks.
- TUI app for viewing and managing tasks in terminal.
- CLI command for AI agent usage.

## Features

- Multi-user support with email+password authentication. All users are equal — there are no roles. User management is done with a separate admin cli (direct DB access); every other operation is per-user and scoped to the authenticated user. User profile in the web ui for changing password, timezone and theme. Token auth for tui and cli apps. Creating tokens for each user via the admin cli and via the web interface. Create/revoke api tokens.
- Telegram notifications via per-user bots: each user creates their own bot (via BotFather) and saves its token in their profile. The server receives updates with long polling (no webhooks — the service is not publicly reachable). Reminder messages include a "Complete" button that marks the task completed straight from Telegram.
- Browser notifications: opt-in desktop notifications for due tasks (Web Notifications API), enabled from the web Account page.
- Full tasks management and viewing from AI agents with the cli. Raw json default output for agents, with --pretty for humans.
- Modern, clean web ui: topbar user menu, list view with time-period (today/week/month) and status (pending/completed/all) filters, a month calendar, dark/light/system theme selector, and a language switcher.
- i18n: English and Greek UI (web, TUI, telegram messages). Per-user timezone and locale. All three clients (web, TUI, CLI) present task times in the user's timezone: the web and, now, the TUI and CLI resolve the zone from an optional client-config override, then the user's profile timezone, then the host local zone. The API always serializes UTC; conversion happens at the render edge.
- Server configuration via env vars. The cli and tui clients read `~/.config/dodo/config.json` (overridable with flags), no env vars. The client config has an optional `timezone` (IANA name) that overrides the profile timezone for rendering; `dodo-cli init` accepts `--timezone`.

## Stack

- Mise setup for all dev tooling installation and tasks.
- Go 1.26.5 for both the api and the cli commands. Three binaries: `dodo` (server + admin subcommand, ships in the container), `dodo-cli` (agent CLI), `dodo-tui` (terminal UI).
- Use go stdlib as much as possible, reducing external dependencies. Telegram Bot API via a small hand-rolled client (no external telegram library).
- Unit tests with extended coverage.
- golangci-lint v2.12.2 for all go files.
- Sqlite for database with native go library modernc.org/sqlite.
- Web frontend built with htmx + Alpine.js and a hand-written, self-contained stylesheet (no Tailwind or CSS framework). Node 24 is used only to vendor htmx/alpine and assemble the embedded assets.
- Dockerfile for deployment, with alpine:3.24 based container (node:24-alpine build stage for frontend assets).
- Github actions workflow for building the container and the commands. The admin cli should be inside the container. TUI and agent cli should be built via goreleaser with separate workflow.
- Web interface is real-time via websockets; the server pushes typed JSON events (`{type, payload}`) that drive live list refreshes, toasts and browser notifications.
- Frontend assets should be bundled in the server binary with go:embed.
- Frontend assets should be versioned to avoid browser caching issues.
- Conventional commits, enforced in CI.
