package api

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"time"
)

func (s *Server) recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic recovered",
					"error", fmt.Sprint(rec),
					"stack", string(debug.Stack()),
					"path", r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, errorEnvelope{
					Error: apiError{Code: "internal", Message: "internal server error"},
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		s.logger.Debug("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying ResponseWriter, so
// its Hijacker/Flusher work through this wrapper — required for the /ws
// websocket upgrade, which otherwise fails with 501.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
