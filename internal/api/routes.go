package api

import (
	"net/http"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/web"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mw := s.authMW

	assetsFS, assetVersion := web.AssetsFS()
	webHandler := web.NewHandler(web.Deps{
		Store:    s.store,
		AuthMW:   mw,
		Hub:      s.hub,
		AssetsFS: assetsFS,
		Version:  assetVersion,
	})
	webHandler.Mount(mux)
	apiChain := func(h http.HandlerFunc) http.Handler {
		return mw.AuthBearer(mw.AuthSession(mw.CSRF(mw.RequireUser(h))))
	}
	publicAPI := func(h http.HandlerFunc) http.Handler {
		return mw.AuthBearer(mw.AuthSession(mw.CSRF(h)))
	}

	mux.HandleFunc("GET /healthz", s.handleHealthz)

	mux.Handle("POST /api/v1/auth/login", publicAPI(s.handleLogin))
	mux.Handle("POST /api/v1/auth/logout", apiChain(s.handleLogout))
	mux.Handle("GET /api/v1/auth/me", apiChain(s.handleMe))

	mux.Handle("GET /api/v1/me", apiChain(s.handleGetProfile))
	mux.Handle("PATCH /api/v1/me", apiChain(s.handleUpdateProfile))
	mux.Handle("POST /api/v1/me/password", apiChain(s.handleChangePassword))

	mux.Handle("GET /api/v1/me/telegram", apiChain(s.handleGetTelegram))
	mux.Handle("POST /api/v1/me/telegram", apiChain(s.handleSetTelegram))
	mux.Handle("PATCH /api/v1/me/telegram", apiChain(s.handleUpdateTelegram))
	mux.Handle("DELETE /api/v1/me/telegram", apiChain(s.handleDeleteTelegram))
	mux.Handle("POST /api/v1/me/telegram/test", apiChain(s.handleTestTelegram))

	mux.Handle("GET /api/v1/tokens", apiChain(s.handleListTokens))
	mux.Handle("POST /api/v1/tokens", apiChain(s.handleCreateToken))
	mux.Handle("DELETE /api/v1/tokens/{id}", apiChain(s.handleRevokeToken))

	mux.Handle("GET /api/v1/tasks", apiChain(s.handleListTasks))
	mux.Handle("POST /api/v1/tasks", apiChain(s.handleCreateTask))
	mux.Handle("GET /api/v1/tasks/{id}", apiChain(s.handleGetTask))
	mux.Handle("PATCH /api/v1/tasks/{id}", apiChain(s.handleUpdateTask))
	mux.Handle("POST /api/v1/tasks/{id}/complete", apiChain(s.handleCompleteTask))
	mux.Handle("POST /api/v1/tasks/{id}/snooze", apiChain(s.handleSnoozeTask))
	mux.Handle("DELETE /api/v1/tasks/{id}", apiChain(s.handleDeleteTask))

	mux.Handle("GET /api/v1/completions", apiChain(s.handleListCompletions))

	mux.Handle("GET /ws", publicAPI(s.handleWS))
}

var _ auth.Middleware
