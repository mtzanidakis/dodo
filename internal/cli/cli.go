package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/clientconfig"
)

const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitNotFound = 4
	ExitAuth     = 5
)

type App struct {
	cfg    clientconfig.ClientConfig
	pretty bool
	Out    io.Writer
	Err    io.Writer
	out    io.Writer
	err    io.Writer
	client *http.Client
}

func New(cfg clientconfig.ClientConfig, pretty bool) *App {
	a := &App{
		cfg:    cfg,
		pretty: pretty,
		Out:    os.Stdout,
		Err:    os.Stderr,
		client: &http.Client{Timeout: 30 * time.Second},
	}
	a.out = a.Out
	a.err = a.Err
	return a
}

func (a *App) refreshWriters() {
	if a.Out != nil {
		a.out = a.Out
	}
	if a.Err != nil {
		a.err = a.Err
	}
}

func (a *App) Run(args []string) int {
	a.refreshWriters()
	if len(args) == 0 {
		a.usage()
		return ExitUsage
	}
	switch args[0] {
	case "init":
		return a.cmdInit(args[1:])
	case "me":
		return a.cmdMe()
	case "tasks":
		return a.cmdTasks(args[1:])
	case "completions":
		return a.cmdCompletions(args[1:])
	case "tokens":
		return a.cmdTokens(args[1:])
	case "-h", "--help", "help":
		a.usage()
		return ExitOK
	default:
		a.eprintf("unknown command %q\n", args[0])
		a.usage()
		return ExitUsage
	}
}

func (a *App) usage() {
	_, _ = io.WriteString(a.err, `dodo-cli - todo client for AI agents

Commands:
  init [--url URL --token TOKEN]
  me
  tasks list [--filter=pending|completed|all] [--priority] [--from] [--to] [--view=list|calendar] [--limit] [--cursor]
  tasks get <id>
  tasks create --title --due [--priority] [--desc] [--repeat=freq:interval:byday] [--repeat-end]
  tasks update <id> [--title] [--due] [--priority] [--desc] [--recalculate]
  tasks complete <id>
  tasks snooze <id> --until
  tasks delete <id>
  completions list [--from] [--to]
  tokens list | tokens create --name | tokens revoke <id>

Global flags: --pretty, --url, --token, --config
Exit codes: 0 ok, 1 error, 2 usage, 4 not found, 5 auth
`)
}

func (a *App) ensureCreds() bool {
	if a.cfg.URL == "" || strings.TrimSpace(a.cfg.Token) == "" {
		a.eprintln(`{"error":{"code":"auth","message":"missing url or token"}}`)
		a.eprintln("hint: run `dodo-cli init --url <api> --token <token>` or `dodo admin token create --email <you>`")
		return false
	}
	return true
}

func (a *App) request(method, path string, body any) (int, []byte, error) {
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	url := strings.TrimRight(a.cfg.URL, "/") + path
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func (a *App) emitJSON(v any) {
	enc := json.NewEncoder(a.out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (a *App) emitRaw(b []byte) {
	if len(bytes.TrimSpace(b)) > 0 {
		var v any
		if err := json.Unmarshal(b, &v); err == nil {
			a.emitJSON(v)
			return
		}
	}
	_, _ = io.WriteString(a.out, strings.TrimSpace(string(b))+"\n")
}

func (a *App) eprintln(args ...any) {
	_, _ = fmt.Fprintln(a.err, args...)
}
func (a *App) eprintf(format string, args ...any) {
	_, _ = fmt.Fprintf(a.err, format, args...)
}

func (a *App) handleResponse(status int, body []byte, notFoundCode int) ([]byte, bool) {
	switch {
	case status >= 200 && status < 300:
		return body, true
	case status == http.StatusNotFound && notFoundCode != 0:
		return body, false
	case status == http.StatusUnauthorized:
		a.eprintln(strings.TrimSpace(string(body)))
		return nil, false
	default:
		a.eprintln(strings.TrimSpace(string(body)))
		return nil, false
	}
}

var _ = errors.New

func parseHumanTime(s string, loc *time.Location) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty time")
	}
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	switch {
	case s == "now":
		return now, nil
	case strings.HasPrefix(s, "now+"):
		d, err := time.ParseDuration(strings.TrimPrefix(s, "now"))
		if err == nil {
			return now.Add(d), nil
		}
	case strings.HasPrefix(s, "now-"):
		d, err := time.ParseDuration(strings.TrimPrefix(s, "now"))
		if err == nil {
			return now.Add(-d), nil
		}
	case s == "tomorrow":
		return now.Add(24 * time.Hour), nil
	}
	for _, layout := range []string{
		time.RFC3339Nano, time.RFC3339,
		"2006-01-02T15:04", "2006-01-02 15:04",
		"2006-01-02 15:04:05", "2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	if strings.HasPrefix(s, "tomorrow ") {
		rest := strings.TrimPrefix(s, "tomorrow ")
		if t, err := time.ParseInLocation("15:04", rest, loc); err == nil {
			return time.Date(now.Year(), now.Month(), now.Day()+1, t.Hour(), t.Minute(), 0, 0, loc), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time %q", s)
}
