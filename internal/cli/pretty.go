package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mtzanidakis/dodo/internal/dateformat"
)

// resource names the shape of a response so --pretty knows how to lay it out.
// The JSON path ignores it entirely; agents keep byte-identical output.
type resource string

const (
	resTask        resource = "task"
	resTaskList    resource = "tasks"
	resUser        resource = "user"
	resTokenList   resource = "tokens"
	resToken       resource = "token"
	resCompletions resource = "completions"
	resOK          resource = "ok"
)

// emit writes a successful API response. Without --pretty this is the
// unchanged JSON path; with it, a human-readable rendering of the same data.
func (a *App) emit(res resource, b []byte) {
	if !a.pretty {
		a.emitRaw(b)
		return
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		// Not JSON (or empty): there is nothing to lay out, so pass it through.
		a.emitRaw(b)
		return
	}
	m, _ := v.(map[string]any)
	switch res {
	case resTaskList:
		a.prettyTaskList(m)
	case resTask:
		// complete/snooze answer with {"task": {...}}; unwrap so one renderer
		// covers every task-shaped response.
		if inner, ok := m["task"].(map[string]any); ok {
			a.prettyTask(inner)
			return
		}
		a.prettyTask(m)
	case resUser:
		a.prettyUser(m)
	case resTokenList:
		a.prettyTokens(m)
	case resToken:
		a.prettyNewToken(m)
	case resCompletions:
		a.prettyCompletions(m)
	case resOK:
		a.println("OK")
	default:
		a.emitRaw(b)
	}
}

func (a *App) println(args ...any) { _, _ = fmt.Fprintln(a.out, args...) }

// prettyTime renders an RFC3339 timestamp in the display zone using the user's
// date pattern, falling back to a sortable layout when no pattern is set.
func (a *App) prettyTime(s string) string {
	if s == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	local := t.In(a.displayLoc())
	if out := dateformat.Render(local, dateformat.WithTime(a.displayFormat())); out != "" {
		return out
	}
	return local.Format("2006-01-02 15:04")
}

func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// table writes rows through a tabwriter so columns line up regardless of
// content width.
func (a *App) table(header []string, rows [][]string) {
	if len(rows) == 0 {
		a.println("No results.")
		return
	}
	w := tabwriter.NewWriter(a.out, 0, 0, 2, ' ', 0)
	_, _ = io.WriteString(w, strings.Join(header, "\t")+"\n")
	for _, r := range rows {
		_, _ = io.WriteString(w, strings.Join(r, "\t")+"\n")
	}
	_ = w.Flush()
}

// fields writes an aligned key/value block for a single object.
func (a *App) fields(pairs [][2]string) {
	w := tabwriter.NewWriter(a.out, 0, 0, 2, ' ', 0)
	for _, p := range pairs {
		_, _ = io.WriteString(w, p[0]+"\t"+p[1]+"\n")
	}
	_ = w.Flush()
}

// taskStatus collapses the flags the API returns into one word.
func taskStatus(t map[string]any, now time.Time) string {
	if str(t, "completed_at") != "" {
		return "done"
	}
	if s := str(t, "snoozed_until"); s != "" {
		if until, err := time.Parse(time.RFC3339, s); err == nil && until.After(now) {
			return "snoozed"
		}
	}
	if s := str(t, "due_at"); s != "" {
		if due, err := time.Parse(time.RFC3339, s); err == nil && due.Before(now) {
			return "overdue"
		}
	}
	return "pending"
}

// repeatLabel renders the recurrence as "daily", "weekly x2" or "-".
func repeatLabel(t map[string]any) string {
	freq := str(t, "recurrence_freq")
	if freq == "" {
		return "-"
	}
	if n, ok := t["recurrence_interval"].(float64); ok && n > 1 {
		return fmt.Sprintf("%s x%d", freq, int(n))
	}
	return freq
}

func items(m map[string]any) []map[string]any {
	raw, _ := m["items"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		if obj, ok := it.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func (a *App) prettyTaskList(m map[string]any) {
	now := time.Now()
	rows := make([][]string, 0)
	for _, t := range items(m) {
		rows = append(rows, []string{
			a.prettyTime(str(t, "due_at")),
			taskStatus(t, now),
			dash(str(t, "priority")),
			dash(str(t, "title")),
			repeatLabel(t),
			str(t, "id"),
		})
	}
	a.table([]string{"DUE", "STATUS", "PRIORITY", "TITLE", "REPEAT", "ID"}, rows)
	// The cursor is opaque and only useful pasted back verbatim.
	if c := str(m, "cursor"); c != "" {
		a.println()
		a.println("More results: --cursor " + c)
	}
}

func (a *App) prettyTask(t map[string]any) {
	pairs := [][2]string{
		{"Title", dash(str(t, "title"))},
		{"Due", a.prettyTime(str(t, "due_at"))},
		{"Status", taskStatus(t, time.Now())},
		{"Priority", dash(str(t, "priority"))},
		{"Repeat", repeatLabel(t)},
	}
	if s := str(t, "snoozed_until"); s != "" {
		pairs = append(pairs, [2]string{"Snoozed until", a.prettyTime(s)})
	}
	if s := str(t, "completed_at"); s != "" {
		pairs = append(pairs, [2]string{"Completed", a.prettyTime(s)})
	}
	if s := str(t, "description"); s != "" {
		pairs = append(pairs, [2]string{"Description", s})
	}
	pairs = append(pairs, [2]string{"Id", str(t, "id")})
	a.fields(pairs)
}

func (a *App) prettyUser(u map[string]any) {
	pairs := [][2]string{
		{"Email", dash(str(u, "email"))},
		{"Name", dash(str(u, "display_name"))},
		{"Timezone", dash(str(u, "timezone"))},
		{"Date format", dash(str(u, "date_format"))},
		{"Language", dash(str(u, "locale"))},
		{"Theme", dash(str(u, "theme"))},
	}
	if linked, ok := u["telegram_linked"].(bool); ok && linked {
		pairs = append(pairs, [2]string{"Telegram", "linked"})
	} else if conf, ok := u["telegram_configured"].(bool); ok && conf {
		pairs = append(pairs, [2]string{"Telegram", "configured, not linked"})
	}
	pairs = append(pairs,
		[2]string{"Member since", a.prettyTime(str(u, "created_at"))},
		[2]string{"Id", str(u, "id")},
	)
	a.fields(pairs)
}

func (a *App) prettyTokens(m map[string]any) {
	rows := make([][]string, 0)
	for _, t := range items(m) {
		rows = append(rows, []string{
			dash(str(t, "name")),
			dash(str(t, "prefix")),
			a.prettyTime(str(t, "last_used_at")),
			a.prettyTime(str(t, "created_at")),
			str(t, "id"),
		})
	}
	a.table([]string{"NAME", "PREFIX", "LAST USED", "CREATED", "ID"}, rows)
}

func (a *App) prettyNewToken(t map[string]any) {
	a.fields([][2]string{
		{"Name", dash(str(t, "name"))},
		{"Token", dash(str(t, "token"))},
		{"Id", str(t, "id")},
	})
	if str(t, "token") != "" {
		a.println()
		a.println("Copy it now - the server stores only a hash and will not show it again.")
	}
}

func (a *App) prettyCompletions(m map[string]any) {
	rows := make([][]string, 0)
	for _, c := range items(m) {
		rows = append(rows, []string{
			a.prettyTime(str(c, "completed_at")),
			dash(str(c, "title")),
			a.prettyTime(str(c, "due_at")),
			dash(str(c, "priority")),
			str(c, "task_id"),
		})
	}
	a.table([]string{"COMPLETED", "TITLE", "DUE", "PRIORITY", "TASK ID"}, rows)
}

// prettyError renders an API error envelope as a single human line. Returns
// false when body is not an error envelope, so callers keep their raw output.
func prettyError(body []byte) (string, bool) {
	var env struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Fields  map[string]any `json:"fields"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Error.Code == "" {
		return "", false
	}
	msg := env.Error.Message
	if msg == "" {
		msg = env.Error.Code
	}
	out := "error: " + msg
	if len(env.Error.Fields) > 0 {
		keys := make([]string, 0, len(env.Error.Fields))
		for k := range env.Error.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s: %v", k, env.Error.Fields[k]))
		}
		out += " (" + strings.Join(parts, "; ") + ")"
	}
	return out, true
}
