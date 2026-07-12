package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	rowStyle    = lipgloss.NewStyle().PaddingLeft(2)
	selStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	hintStyle   = lipgloss.NewStyle().Faint(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	labelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	focusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	cursorStyle = lipgloss.NewStyle().Reverse(true)
)

func (m model) View() string {
	switch m.mode {
	case modeForm:
		return m.formView()
	case modeSnooze:
		return m.snoozeView()
	case modeConfirmDelete:
		return m.confirmDeleteView()
	case modeDetail:
		return m.detailView()
	case modeHelp:
		return m.helpView()
	default:
		return m.listView()
	}
}

func (m model) header() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("dodo"))
	if m.user != "" {
		b.WriteString(hintStyle.Render("  " + m.user))
	}
	return b.String()
}

func (m model) listView() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	if len(m.items) == 0 {
		b.WriteString(hintStyle.Render("No tasks for this filter. Press n to add, r to refresh, q to quit.\n"))
	} else {
		for i, it := range m.items {
			done := "  "
			if it.CompletedAt != nil {
				done = "✓ "
			}
			label := fmt.Sprintf("%s%-5s %-16s %s", done, prioTag(it.Priority), fmtLocal(it.DueAt), it.Title)
			if i == m.cursor {
				b.WriteString(selStyle.Render("▸ " + label))
			} else {
				b.WriteString(rowStyle.Render(label))
			}
			b.WriteString("\n")
		}
		if m.nextCursor != "" {
			b.WriteString(hintStyle.Render("  ↓ more… (scroll down to load)") + "\n")
		}
	}
	if m.err != "" {
		b.WriteString("\n" + errStyle.Render("error: "+m.err) + "\n")
	}
	b.WriteString("\n" + m.statusBar())
	b.WriteString("\n" + hintStyle.Render("↑/↓ move  c complete  enter detail  n new  d delete  s snooze  t status  p period  r refresh  ? help  q quit"))
	return b.String()
}

func (m model) statusBar() string {
	count := fmt.Sprintf("%d", len(m.items))
	if m.nextCursor != "" {
		count += "+"
	}
	return hintStyle.Render(fmt.Sprintf("status: %s   period: %s   tasks: %s", m.filter, m.period, count))
}

func (m model) formView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("new task"))
	b.WriteString("\n\n")
	b.WriteString(m.formRow(fieldTitle, "Title", m.form.title.render(m.form.focus == fieldTitle)))
	b.WriteString(m.formRow(fieldDue, "Due", m.form.due.render(m.form.focus == fieldDue)+hintStyle.Render("  (e.g. tomorrow 10:00, now+2h, 2026-07-11 09:00)")))
	prio := m.form.priority
	if m.form.focus == fieldPriority {
		prio = focusStyle.Render("‹ " + prio + " ›")
	}
	b.WriteString(m.formRow(fieldPriority, "Priority", prio))
	b.WriteString(m.formRow(fieldDesc, "Description", m.form.desc.render(m.form.focus == fieldDesc)))
	if m.err != "" {
		b.WriteString("\n" + errStyle.Render("error: "+m.err) + "\n")
	}
	b.WriteString("\n" + hintStyle.Render("tab/↑↓ move  ←/→ or space change priority  ctrl+s or enter-on-last save  esc cancel"))
	return b.String()
}

func (m model) formRow(field int, label, value string) string {
	marker := "  "
	var l string
	if m.form.focus == field {
		marker = focusStyle.Render("▸ ")
		l = labelStyle.Render(label)
	} else {
		l = hintStyle.Render(label)
	}
	return fmt.Sprintf("%s%-12s %s\n", marker, l, value)
}

func (m model) snoozeView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("snooze task"))
	b.WriteString("\n\n")
	if it, ok := m.selected(); ok {
		b.WriteString(hintStyle.Render(it.Title) + "\n\n")
	}
	b.WriteString(labelStyle.Render("Until") + " " + m.snooze.render(true) + "\n")
	b.WriteString(hintStyle.Render("e.g. tomorrow 10:00, now+2h, 2026-07-11 09:00") + "\n")
	if m.err != "" {
		b.WriteString("\n" + errStyle.Render("error: "+m.err) + "\n")
	}
	b.WriteString("\n" + hintStyle.Render("enter snooze  esc cancel"))
	return b.String()
}

func (m model) confirmDeleteView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("delete task"))
	b.WriteString("\n\n")
	if it, ok := m.selected(); ok {
		b.WriteString("Delete " + selStyle.Render(it.Title) + "?\n")
	}
	b.WriteString("\n" + hintStyle.Render("y confirm  n/esc cancel"))
	return b.String()
}

func (m model) detailView() string {
	it, ok := m.selected()
	if !ok {
		return "no task selected\n\n" + hintStyle.Render("esc back")
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("task detail"))
	b.WriteString("\n\n")
	b.WriteString(detailRow("Title", it.Title))
	desc := it.Description
	if desc == "" {
		desc = hintStyle.Render("(none)")
	}
	b.WriteString(detailRow("Description", desc))
	b.WriteString(detailRow("Priority", it.Priority))
	b.WriteString(detailRow("Due", fmtLocal(it.DueAt)))
	if it.CompletedAt != nil {
		b.WriteString(detailRow("Completed", fmtLocal(*it.CompletedAt)))
	}
	if it.SnoozedUntil != nil {
		b.WriteString(detailRow("Snoozed until", fmtLocal(*it.SnoozedUntil)))
	}
	b.WriteString(detailRow("Recurrence", recurrenceText(it)))
	b.WriteString("\n" + hintStyle.Render("esc/enter back"))
	return b.String()
}

func detailRow(label, value string) string {
	return fmt.Sprintf("%s %s\n", labelStyle.Render(fmt.Sprintf("%-14s", label)), value)
}

func recurrenceText(it taskItem) string {
	if it.RecurrenceFreq == nil || *it.RecurrenceFreq == "" {
		return hintStyle.Render("one-off")
	}
	interval := it.RecurrenceInterval
	if interval < 1 {
		interval = 1
	}
	s := fmt.Sprintf("every %d %s", interval, *it.RecurrenceFreq)
	if it.RecurrenceByDay != nil && *it.RecurrenceByDay != "" {
		s += " on " + *it.RecurrenceByDay
	}
	return s
}

func (m model) helpView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("dodo - keybindings"))
	b.WriteString("\n\n")
	rows := [][2]string{
		{"↑/↓, k/j", "move selection"},
		{"c", "complete task"},
		{"enter", "open task detail"},
		{"n", "new task"},
		{"d", "delete task (confirm)"},
		{"s", "snooze task"},
		{"t", "cycle status (pending/completed/all)"},
		{"p", "cycle period (all/today/week/month)"},
		{"r", "refresh list"},
		{"?", "toggle this help"},
		{"q, ctrl+c", "quit"},
	}
	for _, r := range rows {
		fmt.Fprintf(&b, "  %s %s\n", labelStyle.Render(fmt.Sprintf("%-12s", r[0])), r[1])
	}
	b.WriteString("\n" + hintStyle.Render("press any key to close"))
	return b.String()
}

func prioTag(p string) string {
	switch p {
	case "high":
		return "HIGH"
	case "low":
		return "LOW"
	default:
		return "norm"
	}
}

// fmtLocal renders an RFC3339 timestamp in the local timezone, or "-" when empty.
func fmtLocal(rfc string) string {
	if rfc == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return rfc
	}
	return t.Local().Format("2006-01-02 15:04")
}
