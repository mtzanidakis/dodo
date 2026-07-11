package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mtzanidakis/dodo/internal/clientconfig"
)

type taskItem struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Priority    string  `json:"priority"`
	DueAt       string  `json:"due_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
	Kind        string  `json:"kind"`
}

type listResp struct {
	Items []taskItem `json:"items"`
}

type Client struct {
	cfg  clientconfig.ClientConfig
	http *http.Client
}

func NewClient(cfg clientconfig.ClientConfig) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) request(method, path string, body any) (int, []byte, error) {
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, strings.TrimRight(c.cfg.URL, "/")+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}

func (c *Client) ListTasks() ([]taskItem, error) {
	_, b, err := c.request("GET", "/api/v1/tasks?filter=pending&limit=200", nil)
	if err != nil {
		return nil, err
	}
	var r listResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("decode tasks: %w", err)
	}
	return r.Items, nil
}

func (c *Client) Complete(id string) error {
	_, _, err := c.request("POST", "/api/v1/tasks/"+id+"/complete", map[string]any{})
	return err
}

func (c *Client) Create(title, due, priority string) error {
	_, _, err := c.request("POST", "/api/v1/tasks", map[string]any{"title": title, "due_at": due, "priority": priority})
	return err
}

func (c *Client) Me() (string, error) {
	_, b, err := c.request("GET", "/api/v1/me", nil)
	if err != nil {
		return "", err
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	email, _ := m["email"].(string)
	return email, nil
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	rowStyle   = lipgloss.NewStyle().PaddingLeft(2)
	selStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	hintStyle  = lipgloss.NewStyle().Faint(true)
)

type model struct {
	client *Client
	items  []taskItem
	cursor int
	err    string
	width  int
	height int
	user   string
}

func initialModel(c *Client) model {
	m := model{client: c}
	if u, err := c.Me(); err == nil {
		m.user = u
	}
	_ = m.reload()
	return m
}

func (m model) Init() tea.Cmd { return nil }

func (m *model) reload() error {
	items, err := m.client.ListTasks()
	if err != nil {
		m.err = err.Error()
		return err
	}
	m.items = items
	if m.cursor >= len(m.items) {
		m.cursor = 0
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
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
			_ = m.reload()
		case "enter", "c":
			if m.cursor < len(m.items) {
				id := m.items[m.cursor].ID
				if err := m.client.Complete(id); err != nil {
					m.err = err.Error()
				} else {
					_ = m.reload()
				}
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("dodo"))
	if m.user != "" {
		b.WriteString(hintStyle.Render("  " + m.user))
	}
	b.WriteString("\n\n")
	if len(m.items) == 0 {
		b.WriteString(hintStyle.Render("No pending tasks. Press r to refresh, q to quit.\n"))
	} else {
		for i, it := range m.items {
			marker := "  "
			line := fmt.Sprintf("%s %-7s %s", marker, prioTag(it.Priority), it.Title)
			if i == m.cursor {
				line = selStyle.Render("▸ " + fmt.Sprintf("%-7s %s", prioTag(it.Priority), it.Title))
			} else {
				line = rowStyle.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if m.err != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("error: "+m.err) + "\n")
	}
	b.WriteString("\n" + hintStyle.Render("↑/↓ move  c/enter complete  r refresh  q quit"))
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

func Run(cfg clientconfig.ClientConfig) error {
	p := tea.NewProgram(initialModel(NewClient(cfg)), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
