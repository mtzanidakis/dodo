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
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
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
