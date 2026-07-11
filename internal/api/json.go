package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/mtzanidakis/dodo/internal/models"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	code, status := mapError(err)
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: err.Error()}})
}

func mapError(err error) (string, int) {
	switch {
	case errors.Is(err, models.ErrNotFound):
		return "not_found", http.StatusNotFound
	case errors.Is(err, models.ErrUnauthorized):
		return "unauthorized", http.StatusUnauthorized
	case errors.Is(err, models.ErrConflict):
		return "conflict", http.StatusConflict
	case errors.Is(err, models.ErrValidation):
		return "validation", http.StatusBadRequest
	default:
		return "internal", http.StatusInternalServerError
	}
}

// maxJSONBody caps request bodies so a client can't exhaust memory with an
// arbitrarily large payload.
const maxJSONBody = 1 << 20 // 1 MiB

func decodeJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return errors.Join(models.ErrValidation, err)
	}
	return nil
}
