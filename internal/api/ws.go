package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/mtzanidakis/dodo/internal/auth"
)

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	// No OriginPatterns: coder/websocket then enforces same-origin (the Origin
	// header host must match Host), blocking cross-site WebSocket hijacking.
	// Non-browser clients (no Origin header) are still accepted.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = c.Close(websocket.StatusNormalClosure, "") }()

	client := s.hub.Subscribe(u.ID, c)
	defer s.hub.Unsubscribe(client)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go client.SendLoop()
	for {
		_, _, err := c.Read(ctx)
		if err != nil {
			return
		}
	}
}

var _ = strings.TrimSpace
var _ time.Duration
