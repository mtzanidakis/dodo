package cli

import (
	"flag"
	"strconv"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/clientconfig"
)

// normalizeDue converts a user-supplied due string (RFC3339, "2006-01-02T15:04",
// or free-form like "tomorrow 10:00", "now+30m") into RFC3339 UTC. Anything
// parseHumanTime cannot understand is passed through unchanged so the server can
// report a validation error.
func normalizeDue(s string) string {
	if s == "" {
		return ""
	}
	t, err := parseHumanTime(s, time.Local)
	if err != nil {
		return s
	}
	return t.UTC().Format(time.RFC3339)
}

func (a *App) cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	url := fs.String("url", "", "api base url")
	token := fs.String("token", "", "api token")
	configPath := fs.String("config", "", "config.json path")
	_ = fs.Parse(args)

	cfg := a.cfg
	if *url != "" {
		cfg.URL = *url
	}
	if *token != "" {
		cfg.Token = *token
	}
	path := *configPath
	if err := clientconfig.Write(cfg, path); err != nil {
		a.eprintln(err)
		return ExitError
	}
	a.emitJSON(map[string]any{"ok": true, "url": cfg.URL, "config": path})
	return ExitOK
}

func (a *App) cmdMe() int {
	if !a.ensureCreds() {
		return ExitAuth
	}
	status, body, err := a.request("GET", "/api/v1/me", nil)
	if err != nil {
		a.eprintln(err)
		return ExitError
	}
	b, ok := a.handleResponse(status, body, 0)
	if !ok {
		return failFromStatus(status)
	}
	a.emitRaw(b)
	return ExitOK
}

func failFromStatus(status int) int {
	switch status {
	case 401:
		return ExitAuth
	case 404:
		return ExitNotFound
	default:
		return ExitError
	}
}

func (a *App) cmdTasks(args []string) int {
	if len(args) == 0 {
		a.eprintln("usage: dodo-cli tasks <list|get|create|update|complete|snooze|delete>")
		return ExitUsage
	}
	if !a.ensureCreds() {
		return ExitAuth
	}
	switch args[0] {
	case "list":
		return a.tasksList(args[1:])
	case "get":
		return a.tasksGet(args[1:])
	case "create":
		return a.tasksCreate(args[1:])
	case "update":
		return a.tasksUpdate(args[1:])
	case "complete":
		return a.tasksComplete(args[1:])
	case "snooze":
		return a.tasksSnooze(args[1:])
	case "delete":
		return a.tasksDelete(args[1:])
	default:
		a.eprintf("unknown tasks subcommand %q\n", args[0])
		return ExitUsage
	}
}

func (a *App) tasksList(args []string) int {
	fs := flag.NewFlagSet("tasks list", flag.ContinueOnError)
	filter := fs.String("filter", "", "")
	period := fs.String("period", "", "")
	priority := fs.String("priority", "", "")
	from := fs.String("from", "", "")
	to := fs.String("to", "", "")
	limit := fs.String("limit", "", "")
	cursor := fs.String("cursor", "", "")
	_ = fs.Parse(args)

	q := []string{}
	add := func(k, v string) {
		if v != "" {
			q = append(q, k+"="+v)
		}
	}
	add("filter", *filter)
	add("period", *period)
	add("priority", *priority)
	add("from", *from)
	add("to", *to)
	add("limit", *limit)
	add("cursor", *cursor)
	path := "/api/v1/tasks"
	if len(q) > 0 {
		path += "?" + strings.Join(q, "&")
	}
	status, body, err := a.request("GET", path, nil)
	if err != nil {
		a.eprintln(err)
		return ExitError
	}
	b, ok := a.handleResponse(status, body, 0)
	if !ok {
		return failFromStatus(status)
	}
	a.emitRaw(b)
	return ExitOK
}

func (a *App) tasksGet(args []string) int {
	if len(args) < 1 {
		a.eprintln("usage: tasks get <id>")
		return ExitUsage
	}
	status, body, err := a.request("GET", "/api/v1/tasks/"+args[0], nil)
	if err != nil {
		a.eprintln(err)
		return ExitError
	}
	b, ok := a.handleResponse(status, body, 0)
	if !ok {
		if status == 404 {
			a.eprintln(`{"error":{"code":"not_found"}}`)
			return ExitNotFound
		}
		return failFromStatus(status)
	}
	_ = b
	a.emitRaw(body)
	return ExitOK
}

func (a *App) tasksCreate(args []string) int {
	fs := flag.NewFlagSet("tasks create", flag.ContinueOnError)
	title := fs.String("title", "", "")
	due := fs.String("due", "", "")
	priority := fs.String("priority", "normal", "")
	desc := fs.String("desc", "", "")
	repeat := fs.String("repeat", "", "")
	repeatEnd := fs.String("repeat-end", "", "")
	_ = fs.Parse(args)
	if *title == "" || *due == "" {
		a.eprintln(`{"error":{"code":"validation","message":"title and due required"}}`)
		return ExitUsage
	}
	body := map[string]any{"title": *title, "due_at": normalizeDue(*due), "priority": *priority, "description": *desc}
	if *repeat != "" {
		parts := strings.SplitN(*repeat, ":", 4)
		if len(parts) >= 1 {
			body["recurrence_freq"] = parts[0]
		}
		if len(parts) >= 2 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				body["recurrence_interval"] = n
			}
		}
		if len(parts) >= 3 && parts[2] != "" {
			body["recurrence_by_day"] = parts[2]
		}
		if len(parts) >= 4 && parts[3] != "" {
			if n, err := strconv.Atoi(parts[3]); err == nil {
				body["recurrence_by_month_day"] = n
			}
		}
	}
	if *repeatEnd != "" {
		body["recurrence_end_at"] = *repeatEnd
	}
	status, respBody, err := a.request("POST", "/api/v1/tasks", body)
	if err != nil {
		a.eprintln(err)
		return ExitError
	}
	if _, ok := a.handleResponse(status, respBody, 0); !ok {
		return failFromStatus(status)
	}
	a.emitRaw(respBody)
	return ExitOK
}

func (a *App) tasksUpdate(args []string) int {
	if len(args) < 1 {
		a.eprintln("usage: tasks update <id> [flags]")
		return ExitUsage
	}
	fs := flag.NewFlagSet("tasks update", flag.ContinueOnError)
	title := fs.String("title", "", "")
	due := fs.String("due", "", "")
	priority := fs.String("priority", "", "")
	desc := fs.String("desc", "", "")
	recalc := fs.Bool("recalculate", false, "")
	_ = fs.Parse(args[1:])
	body := map[string]any{}
	if fs.Lookup("title").Value.String() != "" {
		body["title"] = *title
	}
	if *due != "" {
		body["due_at"] = normalizeDue(*due)
	}
	if *priority != "" {
		body["priority"] = *priority
	}
	if fs.Lookup("desc").Value.String() != "" {
		body["description"] = *desc
	}
	path := "/api/v1/tasks/" + args[0]
	if *recalc {
		path += "?recalculate=1"
	}
	status, respBody, err := a.request("PATCH", path, body)
	if err != nil {
		a.eprintln(err)
		return ExitError
	}
	if _, ok := a.handleResponse(status, respBody, 0); !ok {
		if status == 404 {
			return ExitNotFound
		}
		return failFromStatus(status)
	}
	a.emitRaw(respBody)
	return ExitOK
}

func (a *App) tasksComplete(args []string) int {
	if len(args) < 1 {
		a.eprintln("usage: tasks complete <id>")
		return ExitUsage
	}
	status, body, err := a.request("POST", "/api/v1/tasks/"+args[0]+"/complete", map[string]any{})
	if err != nil {
		a.eprintln(err)
		return ExitError
	}
	if _, ok := a.handleResponse(status, body, 0); !ok {
		return failFromStatus(status)
	}
	a.emitRaw(body)
	return ExitOK
}

func (a *App) tasksSnooze(args []string) int {
	if len(args) < 1 {
		a.eprintln("usage: tasks snooze <id> --until <time>")
		return ExitUsage
	}
	fs := flag.NewFlagSet("tasks snooze", flag.ContinueOnError)
	until := fs.String("until", "", "")
	_ = fs.Parse(args[1:])
	if *until == "" {
		a.eprintln(`{"error":{"code":"validation","message":"--until required"}}`)
		return ExitUsage
	}
	status, body, err := a.request("POST", "/api/v1/tasks/"+args[0]+"/snooze", map[string]any{"until": *until})
	if err != nil {
		a.eprintln(err)
		return ExitError
	}
	if _, ok := a.handleResponse(status, body, 0); !ok {
		return failFromStatus(status)
	}
	a.emitRaw(body)
	return ExitOK
}

func (a *App) tasksDelete(args []string) int {
	if len(args) < 1 {
		a.eprintln("usage: tasks delete <id>")
		return ExitUsage
	}
	status, body, err := a.request("DELETE", "/api/v1/tasks/"+args[0], nil)
	if err != nil {
		a.eprintln(err)
		return ExitError
	}
	if _, ok := a.handleResponse(status, body, 0); !ok {
		if status == 404 {
			return ExitNotFound
		}
		return failFromStatus(status)
	}
	a.emitRaw(body)
	return ExitOK
}

func (a *App) cmdCompletions(args []string) int {
	if !a.ensureCreds() {
		return ExitAuth
	}
	fs := flag.NewFlagSet("completions list", flag.ContinueOnError)
	from := fs.String("from", "", "")
	to := fs.String("to", "", "")
	_ = fs.Parse(args)
	path := "/api/v1/completions"
	q := []string{}
	if *from != "" {
		q = append(q, "from="+*from)
	}
	if *to != "" {
		q = append(q, "to="+*to)
	}
	if len(q) > 0 {
		path += "?" + strings.Join(q, "&")
	}
	status, body, err := a.request("GET", path, nil)
	if err != nil {
		a.eprintln(err)
		return ExitError
	}
	if _, ok := a.handleResponse(status, body, 0); !ok {
		return failFromStatus(status)
	}
	a.emitRaw(body)
	return ExitOK
}

func (a *App) cmdTokens(args []string) int {
	if !a.ensureCreds() {
		return ExitAuth
	}
	switch {
	case len(args) == 0:
		a.eprintln("usage: dodo-cli tokens <list|create|revoke>")
		return ExitUsage
	case args[0] == "list":
		status, body, err := a.request("GET", "/api/v1/tokens", nil)
		if err != nil {
			a.eprintln(err)
			return ExitError
		}
		if _, ok := a.handleResponse(status, body, 0); !ok {
			return failFromStatus(status)
		}
		a.emitRaw(body)
		return ExitOK
	case args[0] == "create":
		fs := flag.NewFlagSet("tokens create", flag.ContinueOnError)
		name := fs.String("name", "", "")
		_ = fs.Parse(args[1:])
		if *name == "" {
			a.eprintln(`{"error":{"code":"validation","message":"--name required"}}`)
			return ExitUsage
		}
		status, body, err := a.request("POST", "/api/v1/tokens", map[string]any{"name": *name})
		if err != nil {
			a.eprintln(err)
			return ExitError
		}
		if _, ok := a.handleResponse(status, body, 0); !ok {
			return failFromStatus(status)
		}
		a.emitRaw(body)
		return ExitOK
	case args[0] == "revoke":
		if len(args) < 2 {
			a.eprintln("usage: tokens revoke <id>")
			return ExitUsage
		}
		status, body, err := a.request("DELETE", "/api/v1/tokens/"+args[1], nil)
		if err != nil {
			a.eprintln(err)
			return ExitError
		}
		if _, ok := a.handleResponse(status, body, 0); !ok {
			if status == 404 {
				return ExitNotFound
			}
			return failFromStatus(status)
		}
		a.emitRaw(body)
		return ExitOK
	default:
		return ExitUsage
	}
}
