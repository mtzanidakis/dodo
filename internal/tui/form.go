package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// textField is a minimal single-line text input. bubbles/textinput is not a
// dependency of this module, so the TUI ships its own tiny widget instead.
type textField struct {
	value []rune
	pos   int // cursor index into value
}

func (t *textField) String() string { return string(t.value) }

func (t *textField) setValue(s string) {
	t.value = []rune(s)
	t.pos = len(t.value)
}

func (t *textField) insert(r rune) {
	t.value = append(t.value, 0)
	copy(t.value[t.pos+1:], t.value[t.pos:])
	t.value[t.pos] = r
	t.pos++
}

func (t *textField) backspace() {
	if t.pos == 0 {
		return
	}
	t.value = append(t.value[:t.pos-1], t.value[t.pos:]...)
	t.pos--
}

func (t *textField) left() {
	if t.pos > 0 {
		t.pos--
	}
}

func (t *textField) right() {
	if t.pos < len(t.value) {
		t.pos++
	}
}

// render returns the field content, drawing a cursor block when focused.
func (t *textField) render(focused bool) string {
	if !focused {
		if len(t.value) == 0 {
			return ""
		}
		return string(t.value)
	}
	// Insert a visible cursor at pos.
	var b strings.Builder
	for i, r := range t.value {
		if i == t.pos {
			b.WriteString(cursorStyle.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	if t.pos >= len(t.value) {
		b.WriteString(cursorStyle.Render(" "))
	}
	return b.String()
}

// form field indices.
const (
	fieldTitle = iota
	fieldDue
	fieldPriority
	fieldDesc
	fieldCount
)

type taskForm struct {
	title    textField
	due      textField
	desc     textField
	priority string
	focus    int
}

func newTaskForm() taskForm {
	return taskForm{priority: "normal", focus: fieldTitle}
}

func (f *taskForm) next() { f.focus = (f.focus + 1) % fieldCount }
func (f *taskForm) prev() { f.focus = (f.focus + fieldCount - 1) % fieldCount }

// activeInput returns the text field for the currently focused row, or nil when
// the focus is on the (non-text) priority selector.
func (f *taskForm) activeInput() *textField {
	switch f.focus {
	case fieldTitle:
		return &f.title
	case fieldDue:
		return &f.due
	case fieldDesc:
		return &f.desc
	default:
		return nil
	}
}

// validate builds the API create arguments from the form, resolving the due
// time via parseHumanTime.
func (f *taskForm) validate() (title, due, priority, desc string, err error) {
	title = strings.TrimSpace(f.title.String())
	if title == "" {
		return "", "", "", "", errors.New("title is required")
	}
	dueRaw := strings.TrimSpace(f.due.String())
	if dueRaw == "" {
		return "", "", "", "", errors.New("due date is required")
	}
	t, err := parseHumanTime(dueRaw, time.Local)
	if err != nil {
		return "", "", "", "", err
	}
	return title, t.UTC().Format(time.RFC3339), f.priority, strings.TrimSpace(f.desc.String()), nil
}

func nextPriority(p string) string {
	switch p {
	case "low":
		return "normal"
	case "normal":
		return "high"
	default:
		return "low"
	}
}

// statusCycle and periodCycle are the two independent list-filter axes,
// advanced by the `t` and `p` keys respectively.
var (
	statusCycle = []string{"pending", "completed", "all"}
	periodCycle = []string{"all", "today", "week", "month"}
)

func advanceCycle(cycle []string, cur string) string {
	for i, v := range cycle {
		if v == cur {
			return cycle[(i+1)%len(cycle)]
		}
	}
	return cycle[0]
}

func nextStatus(s string) string { return advanceCycle(statusCycle, s) }
func nextPeriod(p string) string { return advanceCycle(periodCycle, p) }

// parseHumanTime is adapted from internal/cli; it accepts a handful of human
// friendly forms ("now", "now+2h", "tomorrow 10:00") plus common layouts.
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
		if d, err := time.ParseDuration(strings.TrimPrefix(s, "now")); err == nil {
			return now.Add(d), nil
		}
	case strings.HasPrefix(s, "now-"):
		if d, err := time.ParseDuration(strings.TrimPrefix(s, "now")); err == nil {
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
