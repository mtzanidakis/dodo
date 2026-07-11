package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modeList mode = iota
	modeForm
	modeDetail
	modeConfirmDelete
	modeSnooze
	modeHelp
)

type model struct {
	client *Client
	items  []taskItem
	cursor int
	err    string
	width  int
	height int
	user   string
	filter string

	mode   mode
	form   taskForm
	snooze textField
}

func initialModel(c *Client) model {
	m := model{client: c, filter: "pending"}
	if u, err := c.Me(); err == nil {
		m.user = u
	}
	m.reload()
	return m
}

func (m model) Init() tea.Cmd { return nil }

func (m *model) reload() {
	items, err := m.client.ListTasksFilter(m.filter)
	if err != nil {
		m.err = err.Error()
		return
	}
	m.err = ""
	m.items = items
	if m.cursor >= len(m.items) {
		m.cursor = 0
	}
}

func (m *model) selected() (taskItem, bool) {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor], true
	}
	return taskItem{}, false
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeForm:
			return m.updateForm(msg)
		case modeSnooze:
			return m.updateSnooze(msg)
		case modeConfirmDelete:
			return m.updateConfirmDelete(msg)
		case modeDetail:
			return m.updateDetail(msg)
		case modeHelp:
			m.mode = modeList
			return m, nil
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "r":
		m.reload()
	case "c":
		if it, ok := m.selected(); ok {
			if err := m.client.Complete(it.ID); err != nil {
				m.err = err.Error()
			} else {
				m.reload()
			}
		}
	case "enter":
		if _, ok := m.selected(); ok {
			m.mode = modeDetail
		}
	case "n":
		m.form = newTaskForm()
		m.err = ""
		m.mode = modeForm
	case "d":
		if _, ok := m.selected(); ok {
			m.mode = modeConfirmDelete
		}
	case "s":
		if _, ok := m.selected(); ok {
			m.snooze = textField{}
			m.err = ""
			m.mode = modeSnooze
		}
	case "t":
		m.filter = nextFilter(m.filter)
		m.cursor = 0
		m.reload()
	case "?":
		m.mode = modeHelp
	}
	return m, nil
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.err = ""
		return m, nil
	case "ctrl+s":
		return m.saveForm()
	case "tab", "down":
		m.form.next()
		return m, nil
	case "shift+tab", "up":
		m.form.prev()
		return m, nil
	case "enter":
		// Enter on the last field submits; otherwise advance focus.
		if m.form.focus == fieldDesc {
			return m.saveForm()
		}
		m.form.next()
		return m, nil
	case "left":
		if m.form.focus == fieldPriority {
			// cycle backwards: low<-normal<-high
			m.form.priority = nextPriority(nextPriority(m.form.priority))
			return m, nil
		}
		if in := m.form.activeInput(); in != nil {
			in.left()
		}
		return m, nil
	case "right", " ":
		if m.form.focus == fieldPriority {
			m.form.priority = nextPriority(m.form.priority)
			return m, nil
		}
		if msg.String() == "right" {
			if in := m.form.activeInput(); in != nil {
				in.right()
			}
			return m, nil
		}
		// space in a text field falls through to insertion below
	case "backspace":
		if in := m.form.activeInput(); in != nil {
			in.backspace()
		}
		return m, nil
	}
	// Printable rune insertion for the focused text field.
	if in := m.form.activeInput(); in != nil {
		for _, r := range msg.Runes {
			in.insert(r)
		}
	}
	return m, nil
}

func (m model) saveForm() (tea.Model, tea.Cmd) {
	title, due, priority, desc, err := m.form.validate()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if err := m.client.Create(title, due, priority, desc); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.mode = modeList
	m.err = ""
	m.reload()
	return m, nil
}

func (m model) updateSnooze(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.err = ""
		return m, nil
	case "enter":
		it, ok := m.selected()
		if !ok {
			m.mode = modeList
			return m, nil
		}
		t, err := parseHumanTime(m.snooze.String(), time.Local)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		if err := m.client.Snooze(it.ID, t.UTC().Format(time.RFC3339)); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.mode = modeList
		m.err = ""
		m.reload()
		return m, nil
	case "backspace":
		m.snooze.backspace()
		return m, nil
	case "left":
		m.snooze.left()
		return m, nil
	case "right":
		m.snooze.right()
		return m, nil
	}
	for _, r := range msg.Runes {
		m.snooze.insert(r)
	}
	return m, nil
}

func (m model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		if it, ok := m.selected(); ok {
			if err := m.client.Delete(it.ID); err != nil {
				m.err = err.Error()
			} else {
				m.err = ""
			}
		}
		m.mode = modeList
		m.reload()
	case "n", "esc":
		m.mode = modeList
	}
	return m, nil
}

func (m model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		m.mode = modeList
	}
	return m, nil
}
