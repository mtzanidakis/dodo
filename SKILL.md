---
name: dodo
description: >-
  Manage todo tasks and reminders through the dodo CLI (`dodo-cli`). Use this
  skill whenever the user wants to add a todo or reminder, asks what is due
  (today / this week / this month), wants to list, complete, snooze, update or
  delete tasks, or mentions "dodo". On first use it installs the `dodo-cli`
  binary from the latest GitHub release into `~/.local/bin`, then drives it
  (JSON output, machine-readable exit codes).
---

# dodo CLI skill

`dodo` is a self-hosted todo & reminders service. `dodo-cli` is its
agent-facing client: it prints raw JSON to stdout by default (add `--pretty`
for humans), reports errors as JSON on stderr, and uses machine-readable exit
codes. Read this file top to bottom on first use, then jump to **Usage**.

## 1. Install (first use only)

Skip if `dodo-cli` is already on `PATH` (`command -v dodo-cli`).

Download the binary for the host's OS/arch from the latest release and drop it
in `~/.local/bin` (the default install dir; change if the user prefers another
dir already on `PATH`).

```bash
set -euo pipefail
REPO=mtzanidakis/dodo
BINDIR="${DODO_BINDIR:-$HOME/.local/bin}"
mkdir -p "$BINDIR"

# host os/arch as goreleaser names them
os=$(uname -s | tr '[:upper:]' '[:lower:]')          # linux | darwin
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac

# resolve the latest release tag (e.g. v1.2.3) and the version without the v
tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
      | grep -m1 '"tag_name"' | cut -d'"' -f4)
ver=${tag#v}

# archive name comes from .goreleaser.yml:
#   dodo-dodo-cli_<version>_<os>_<arch>.tar.gz  (extracts to `dodo-cli`)
asset="dodo-dodo-cli_${ver}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$tag/$asset"

tmp=$(mktemp -d)
curl -fsSL "$url" -o "$tmp/cli.tar.gz"
tar -xzf "$tmp/cli.tar.gz" -C "$tmp"
install -m 0755 "$tmp/dodo-cli" "$BINDIR/dodo-cli"
rm -rf "$tmp"
"$BINDIR/dodo-cli" --help >/dev/null && echo "installed dodo-cli to $BINDIR"
```

Ensure `~/.local/bin` is on `PATH`. If `command -v dodo-cli` fails afterward,
add it for the current shell and tell the user to persist it:

```bash
export PATH="$HOME/.local/bin:$PATH"   # add to ~/.bashrc / ~/.zshrc to persist
```

(If `gh` is available, `gh release download --repo mtzanidakis/dodo --pattern 'dodo-dodo-cli_*_'"${os}_${arch}"'.tar.gz'` is an alternative to the curl step.)

Once installed, `dodo-cli` can update itself: `dodo-cli upgrade` checks the
latest release and, if it is newer than the installed build, downloads the
matching archive, verifies its checksum, and replaces the binary in place.
`dodo-cli version` prints the installed version.

## 2. Configure

`dodo-cli` needs the API base URL and a bearer token. It reads
`~/.config/dodo/config.json`; flags `--url` / `--token` / `--config` override
it. It never reads environment variables.

```json
{
  "url": "http://localhost:8080",
  "token": "dodo_xxxxxxxxxxxx",
  "log_level": "info",
  "timezone": "Europe/Athens"
}
```

`timezone` is optional (an IANA name, or `UTC`). It overrides the display zone
for timestamps; when omitted, `dodo-cli` uses the account's profile timezone
from the server. Times in the JSON output are rendered in that zone as RFC3339
with a local offset (e.g. `2026-07-11T20:00:00+03:00`), not UTC `Z`.

Write it once with:

```bash
dodo-cli init --url http://<host>:<port> --token dodo_xxxxxxxxxxxx [--timezone Europe/Athens]
```

Getting a token (ask the user which applies):
- Web UI: **Account → API tokens → Create token** (copy the token, shown once).
- Server host: `dodo admin token create --email <user> --name agent` (prints the token once).

A missing url/token exits **5**; the error hints how to create one. If the user
hasn't provided a URL/token yet, ask for them before making API calls.

## 3. Usage

Default output is JSON on stdout; add `--pretty` for a human table. Exit codes:
`0` ok, `1` error, `2` usage, `4` not found, `5` auth. On failure stdout stays
empty and the error goes to stderr as `{"error":{"code","message"}}`.

```
dodo-cli me
dodo-cli tasks list [--filter=pending|completed|all] [--period=all|today|week|month]
                    [--priority=low|normal|high] [--from=] [--to=] [--limit=] [--cursor=]
dodo-cli tasks get <id>
dodo-cli tasks create --title <t> --due <when> [--priority=low|normal|high] [--desc=<d>]
                      [--repeat=<freq>:<interval>[:<byday>]] [--repeat-end=<when>]
dodo-cli tasks update <id> [--title] [--due] [--priority] [--desc] [--recalculate]
dodo-cli tasks complete <id>
dodo-cli tasks snooze <id> --until <when>
dodo-cli tasks delete <id>
dodo-cli completions list [--from=] [--to=]
dodo-cli tokens list | tokens create --name <n> | tokens revoke <id>
dodo-cli version           # print the installed version
dodo-cli upgrade           # self-update to the latest release
```

Time values (`--due`, `--until`, `--from`, `--to`) accept RFC3339
(`2026-07-11T17:00:00Z`), `YYYY-MM-DDTHH:MM` in the user's timezone, or the
shorthands `now`, `now+30m`, `now-1h`, `tomorrow`, `tomorrow 10:00`.

`--repeat` is `freq:interval[:byday]`, freq ∈ daily|weekly|monthly|yearly,
interval ≥ 1, byday a comma list of `MO,TU,WE,TH,FR,SA,SU` (weekly). Examples:
`--repeat=daily:1`, `--repeat=weekly:1:MO,WE,FR`, `--repeat=monthly:1`.

### Common intents → commands

- **Add a task**: `dodo-cli tasks create --title "Pay electric bill" --due "tomorrow 17:00" --priority high`
- **What's due today**: `dodo-cli tasks list --filter=pending --period=today`
- **This week's tasks**: `dodo-cli tasks list --filter=pending --period=week`
- **All open tasks**: `dodo-cli tasks list --filter=pending`
- **Complete a task**: look up its `id` (from a list), then `dodo-cli tasks complete <id>`
- **Snooze**: `dodo-cli tasks snooze <id> --until "tomorrow 09:00"`
- **Recurring rent**: `dodo-cli tasks create --title "Pay rent" --due "2026-08-01T21:00" --priority high --repeat=monthly:1`

When the user asks about tasks, prefer `tasks list` with the narrowest
`--filter`/`--period` that fits, parse the JSON `items[]`, and summarize
(title, due time, priority). To act on a specific task you need its `id` — get
it from a `tasks list` first.
