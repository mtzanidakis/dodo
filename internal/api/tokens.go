package api

import (
	"errors"
	"net/http"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/models"
)

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	tokens, err := s.store.Tokens.List(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]tokenDTO, 0, len(tokens))
	for _, t := range tokens {
		if t.Revoked() {
			continue
		}
		items = append(items, toTokenDTO(t, ""))
	}
	writeJSON(w, http.StatusOK, listEnvelope[tokenDTO]{Items: items})
}

type createTokenRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	var req createTokenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Name == "" {
		writeError(w, errors.Join(models.ErrValidation, errors.New("name required")))
		return
	}
	gen, err := auth.GenerateAPIToken()
	if err != nil {
		writeError(w, err)
		return
	}
	t, err := s.store.Tokens.Create(r.Context(), u.ID, req.Name, gen.Prefix, gen.Hash)
	if err != nil {
		writeError(w, err)
		return
	}
	s.audit(r, "token.create", "api_token", t.ID, map[string]any{"name": req.Name})
	s.hub.Publish(u.ID, "tokens.updated", nil)
	writeJSON(w, http.StatusCreated, toTokenDTO(t, gen.Full))
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	if err := s.store.Tokens.Revoke(r.Context(), u.ID, id); err != nil {
		writeError(w, err)
		return
	}
	s.audit(r, "token.revoke", "api_token", id, nil)
	s.hub.Publish(u.ID, "tokens.updated", nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
