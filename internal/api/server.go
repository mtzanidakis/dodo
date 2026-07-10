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
	return s.recoverMW(s.logMW(mux))
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
