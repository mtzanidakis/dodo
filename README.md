# dodo

A self-hosted todo & reminders service. MIT licensed.

Tasks support one-off reminders at a specific date/time and recurring
tasks (daily/weekly/monthly/yearly). Notifications repeat at the
priority's interval until completed (low=2h, normal=1h, high=20m) and
are delivered through **each user's own Telegram bot**, with a
"Complete" button straight from the chat. The service is designed to
run inside a private Tailscale tailnet and uses long polling (no
webhooks).

| Binary     | Purpose                                                            |
|------------|-------------------------------------------------------------------|
| `dodo`     | HTTP API + web UI + scheduler + telegram pollers; admin CLI. Ships in the container. |
| `dodo-cli` | AI-agent CLI client (JSON stdout, `--pretty` for humans).          |
| `dodo-tui` | Terminal UI client (API + token auth).                            |

## Quickstart (docker)

```
docker run -d --name dodo \
  -p 8080:8080 \
  -v dodo-data:/data \
  -e DODO_ENCRYPTION_KEY=$(openssl rand -base64 32) \
  ghcr.io/mtzanidakis/dodo

# bootstrap the first admin (inside the container)
docker exec -it dodo dodo admin user create --email admin@example.com --password supersecret --role admin
TOK=$(docker exec -it dodo dodo admin token create --email admin@example.com --name agent | jq -r .token)

dodo-cli init --url http://localhost:8080 --token "$TOK"
dodo-cli tasks create --title "Pay electric bill" --due 2026-07-11T17:00:00Z --priority high
dodo-cli tasks list
```

## Quickstart (local, from source)

```
mise install
mise run build-all
export DODO_ENCRYPTION_KEY=$(openssl rand -base64 32)
export DODO_DATABASE_PATH=/tmp/dodo.sqlite
./dodo admin user create --email admin@example.com --password supersecret --role admin
./dodo serve &
TOK=$(./dodo admin token create --email admin@example.com --name agent | jq -r .token)
./dodo-cli --url http://localhost:8080 --token "$TOK" tasks create --title "Pay bill" --due 2026-07-11T17:00:00Z --priority high
./dodo-cli --url http://localhost:8080 --token "$TOK" tasks list
```

Open `http://localhost:8080/login` in a browser and sign in with the
admin credentials to use the web UI.

## Server env vars

See `AGENTS.md` for the full table. Highlights:

| Env | Default | Notes |
|-----|---------|-------|
| `DODO_DATABASE_PATH` | `/data/dodo.sqlite` | SQLite path (serve + admin). |
| `DODO_LISTEN` | `:8080` | HTTP bind. |
| `DODO_ENCRYPTION_KEY` | (required) | 32-byte base64; AES-256-GCM for telegram bot tokens. |
| `DODO_SCHEDULER_INTERVAL` | `1m` | Reminder scan cadence (min 1m). |

The CLI/TUI clients ignore env vars and read
`~/.config/dodo/config.json` (overridable with `--url`/`--token`/`--config`).

## Client config (`dodo-cli` / `dodo-tui`)

```json
{
  "url": "http://localhost:8080",
  "token": "dodo_xxxxxxxxxxxx",
  "log_level": "info"
}
```

`dodo-cli init --url <api> --token <token>` writes the minimal config for
first-time setup.

## Telegram setup

1. Create a bot with [@BotFather](https://t.me/BotFather) and copy its
   token.
2. In the web UI Account page (or `POST /api/v1/me/telegram`), save the
   bot token and your Telegram user id (comma-separated list of allowed
   ids).
3. Send `/start` to your bot from Telegram; the chat gets linked.
4. Reminders now arrive via your own bot with a Complete button.

Bot tokens are encrypted at rest with AES-256-GCM
(`DODO_ENCRYPTION_KEY`). The server receives updates with long polling
(no webhooks needed).

## Backup

The `/data` volume is the only state. Online backup:

```
sqlite3 /data/dodo.sqlite ".backup /data/backup.sqlite"
```

## Development

```
mise run lint          # golangci-lint
mise run test         # go test -race -cover ./...
mise run build-all    # all three binaries
mise run web:build    # frontend assets -> internal/web/dist
```

Commits follow [Conventional Commits](https://www.conventionalcommits.org/),
enforced by CI (`feat fix refactor test chore docs ci build perf style`).

## License

MIT - see [LICENSE](./LICENSE).