package api

import (
	"net/http"
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
	completions, err := s.store.Completions.List(r.Context(), u.ID, fromPtr, toPtr)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]completionDTO, 0, len(completions))
	for _, c := range completions {
		items = append(items, toCompletionDTO(c))
	}
	writeJSON(w, http.StatusOK, listEnvelope[completionDTO]{Items: items})
}
