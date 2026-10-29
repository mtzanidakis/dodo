package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

type client struct {
	userID       string
	conn         *websocket.Conn
	ch           chan Event
	done         chan struct{}
	pingInterval time.Duration
	timeout      time.Duration
}

type Hub struct {
	mu           sync.RWMutex
	clients      map[string]map[*client]struct{}
	maxPerUser   int
	pingInterval time.Duration
	timeout      time.Duration
	logger       *slog.Logger
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients:      make(map[string]map[*client]struct{}),
		maxPerUser:   5,
		pingInterval: 30 * time.Second,
		timeout:      10 * time.Second,
		logger:       logger,
	}
}

func (h *Hub) Subscribe(userID string, conn *websocket.Conn) *client {
	c := &client{userID: userID, conn: conn, ch: make(chan Event, 16), done: make(chan struct{}), pingInterval: h.pingInterval, timeout: h.timeout}
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.clients[userID]
	if set == nil {
		set = make(map[*client]struct{})
		h.clients[userID] = set
	}
	if len(set) >= h.maxPerUser {
		var oldest *client
		for k := range set {
			oldest = k
			break
		}
		if oldest != nil {
			delete(set, oldest)
			close(oldest.done)
			h.logger.Info("ws hub evicting oldest client", "user_id", userID)
		}
	}
	set[c] = struct{}{}
	return c
}

func (h *Hub) Unsubscribe(c *client) {
	if c == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.clients[c.userID]
	if set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, c.userID)
		}
	}
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

func (h *Hub) Publish(userID, eventType string, payload any) {
	evt := Event{Type: eventType, Payload: payload}
	h.mu.RLock()
	set := h.clients[userID]
	clients := make([]*client, 0, len(set))
	for c := range set {
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	for _, c := range clients {
		select {
		case c.ch <- evt:
		case <-c.done:
		default:
		}
	}
}

// heartbeat is sent alongside each protocol ping. Browsers answer pings
// without exposing them to JavaScript, so the web client watches for this
// message to notice a connection that died without closing.
var heartbeat = []byte(`{"type":"ping"}`)

func (c *client) SendLoop() {
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()
	for {
		select {
		case evt := <-c.ch:
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			if err := c.write(data); err != nil {
				return
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
			err := c.conn.Ping(ctx)
			cancel()
			if err != nil {
				_ = c.conn.CloseNow()
				return
			}
			if err := c.write(heartbeat); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

// write closes the connection on failure so the handler's read loop returns
// and the client is unsubscribed instead of lingering half-dead.
func (c *client) write(data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	err := c.conn.Write(ctx, websocket.MessageText, data)
	if err != nil {
		_ = c.conn.CloseNow()
	}
	return err
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.clients {
		for c := range set {
			select {
			case <-c.done:
			default:
				close(c.done)
			}
		}
	}
	h.clients = make(map[string]map[*client]struct{})
}
