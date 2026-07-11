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

	"github.com/mtzanidakis/dodo/internal/clientconfig"
)

type taskItem struct {
	ID                 string  `json:"id"`
	Title              string  `json:"title"`
	Description        string  `json:"description"`
	Priority           string  `json:"priority"`
	Kind               string  `json:"kind"`
	DueAt              string  `json:"due_at"`
	CompletedAt        *string `json:"completed_at,omitempty"`
	RecurrenceFreq     *string `json:"recurrence_freq,omitempty"`
	RecurrenceInterval int     `json:"recurrence_interval"`
	RecurrenceByDay    *string `json:"recurrence_by_day,omitempty"`
	SnoozedUntil       *string `json:"snoozed_until,omitempty"`
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

// checkStatus turns a non-2xx response into an error carrying the server body.
func checkStatus(status int, body []byte) error {
	if status >= 200 && status < 300 {
		return nil
	}
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Errorf("request failed: status %d", status)
	}
	return fmt.Errorf("status %d: %s", status, msg)
}

// ListTasks returns pending tasks (kept for backward compatibility / tests).
func (c *Client) ListTasks() ([]taskItem, error) {
	return c.ListTasksFilter("pending")
}

// ListTasksFilter fetches tasks for the given filter (pending|completed|all).
func (c *Client) ListTasksFilter(filter string) ([]taskItem, error) {
	if filter == "" {
		filter = "pending"
	}
	status, b, err := c.request("GET", "/api/v1/tasks?filter="+filter+"&limit=200", nil)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(status, b); err != nil {
		return nil, err
	}
	var r listResp
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("decode tasks: %w", err)
	}
	return r.Items, nil
}

func (c *Client) Complete(id string) error {
	status, b, err := c.request("POST", "/api/v1/tasks/"+id+"/complete", map[string]any{})
	if err != nil {
		return err
	}
	return checkStatus(status, b)
}

// Create posts a new task. due must already be an API-acceptable string
// (RFC3339 preferred); description may be empty.
func (c *Client) Create(title, due, priority, description string) error {
	body := map[string]any{"title": title, "due_at": due, "priority": priority}
	if description != "" {
		body["description"] = description
	}
	status, b, err := c.request("POST", "/api/v1/tasks", body)
	if err != nil {
		return err
	}
	return checkStatus(status, b)
}

// Snooze reschedules a task until the given time string.
func (c *Client) Snooze(id, until string) error {
	status, b, err := c.request("POST", "/api/v1/tasks/"+id+"/snooze", map[string]any{"until": until})
	if err != nil {
		return err
	}
	return checkStatus(status, b)
}

// Delete removes a task.
func (c *Client) Delete(id string) error {
	status, b, err := c.request("DELETE", "/api/v1/tasks/"+id, nil)
	if err != nil {
		return err
	}
	return checkStatus(status, b)
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

func Run(cfg clientconfig.ClientConfig) error {
	p := tea.NewProgram(initialModel(NewClient(cfg)), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
