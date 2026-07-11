package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/mtzanidakis/dodo/internal/auth"
)

func (s *Server) handleListCompletions(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	loc := userLoc(u)
	var fromPtr, toPtr *time.Time
	if v := r.URL.Query().Get("from"); v != "" {
		fromPtr = ptrTime(parseDueAt(v, loc))
	}
	if v := r.URL.Query().Get("to"); v != "" {
		toPtr = ptrTime(parseDueAt(v, loc))
	}
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	cursor := r.URL.Query().Get("cursor")
	completions, next, err := s.store.Completions.List(r.Context(), u.ID, fromPtr, toPtr, limit, cursor)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]completionDTO, 0, len(completions))
	for _, c := range completions {
		items = append(items, toCompletionDTO(c))
	}
	resp := listEnvelope[completionDTO]{Items: items}
	if next != "" {
		resp.Cursor = &next
	}
	writeJSON(w, http.StatusOK, resp)
}
