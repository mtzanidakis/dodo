package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/config"
	"github.com/mtzanidakis/dodo/internal/crypto"
	"github.com/mtzanidakis/dodo/internal/db"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/telegram"
	"github.com/mtzanidakis/dodo/internal/ws"
)

type Server struct {
	cfg      config.Config
	store    *store.Store
	authMW   *auth.Middleware
	loginRL  *auth.LoginRateLimiter
	crypto   *crypto.Crypto
	hub      *ws.Hub
	telegram TelegramService
	logger   *slog.Logger
	version  string
	now      func() time.Time
}

type TelegramService interface {
	ValidateToken(ctx context.Context, botToken string) (username string, err error)
	SendTest(ctx context.Context, userID, chatID, text string) error
	SendReminder(ctx context.Context, userID, chatID, text, taskID, buttonLabel string) error
	StartForUser(ctx context.Context, userID string) error
	StopForUser(userID string) error
	StartAll(ctx context.Context) error
	StopAll()
}

func NewServer(cfg config.Config, st *store.Store, hub *ws.Hub, telegram TelegramService, logger *slog.Logger, version string) (*Server, error) {
	c, err := crypto.New(cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:      cfg,
		store:    st,
		authMW:   &auth.Middleware{Store: st},
		loginRL:  auth.NewLoginRateLimiter(),
		crypto:   c,
		hub:      hub,
		telegram: telegram,
		logger:   logger,
		version:  version,
		now:      time.Now,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return securityHeadersMW(s.recoverMW(s.logMW(mux)))
}

// contentSecurityPolicy allows 'unsafe-inline'/'unsafe-eval' in script-src
// because the templates rely on inline on* handlers and Alpine expression
// evaluation; tightening it requires moving those into app.js. The policy
// still blocks framing, cross-origin form posts, plugins and off-origin
// resource loads.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"object-src 'none'; " +
	"frame-ancestors 'none'"

func securityHeadersMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if auth.IsSecure(r) {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func Serve(cfg config.Config, version, commit string) error {
	logger := slog.Default()
	d, err := openDB(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()

	st := store.New(d)
	hub := ws.NewHub(logger)
	c, err := crypto.New(cfg.EncryptionKey)
	if err != nil {
		return err
	}
	registry := telegram.NewRegistry(st, c)
	pollers := telegram.NewPollers(registry, st, logger, hub)
	tgService := telegram.NewService(pollers, st, hub, logger)

	srv, err := NewServer(cfg, st, hub, tgService, logger, version)
	if err != nil {
		return err
	}

	sched := newScheduler(cfg.SchedulerInterval, st, hub, tgService, logger)
	ctx := context.Background()
	_ = sched.Start(ctx)
	_ = tgService.StartAll(ctx)

	httpServer := &http.Server{
		Addr:    cfg.Listen,
		Handler: srv.Handler(),
	}
	logger.Info("dodo serve starting", "listen", cfg.Listen, "version", version, "commit", commit)
	return httpServer.ListenAndServe()
}

func openDB(path string) (*sql.DB, error) {
	return db.Open(path)
}

func (s *Server) Now() time.Time { return s.now().UTC() }
