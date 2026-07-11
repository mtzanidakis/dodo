package telegram

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/mtzanidakis/dodo/internal/crypto"
	"github.com/mtzanidakis/dodo/internal/store"
)

type Registry struct {
	store   *store.Store
	crypto  *crypto.Crypto
	apiBase string
	mu      sync.RWMutex
	clients map[string]*Client
}

func NewRegistry(st *store.Store, c *crypto.Crypto) *Registry {
	return &Registry{store: st, crypto: c, clients: make(map[string]*Client)}
}

func (r *Registry) WithAPIBase(base string) *Registry { r.apiBase = base; return r }

func (r *Registry) GetOrCreate(ctx context.Context, userID string) (*Client, error) {
	r.mu.RLock()
	if c, ok := r.clients[userID]; ok {
		r.mu.RUnlock()
		return c, nil
	}
	r.mu.RUnlock()

	encToken, _, _, _, _, err := r.store.Users.GetTelegramConfig(ctx, userID)
	if err != nil || encToken == "" {
		return nil, errors.New("telegram not configured")
	}
	token, err := r.crypto.Decrypt(encToken)
	if err != nil {
		return nil, err
	}
	c := New(token)
	if r.apiBase != "" {
		c = c.WithAPIBase(r.apiBase)
	}
	r.mu.Lock()
	r.clients[userID] = c
	r.mu.Unlock()
	return c, nil
}

func (r *Registry) Invalidate(userID string) {
	r.mu.Lock()
	delete(r.clients, userID)
	r.mu.Unlock()
}

type Pollers struct {
	registry *Registry
	store    *store.Store
	logger   *slog.Logger
	now      func() time.Time
	hub      pub
	onUpdate func(userID string, u Update)
	mu       sync.Mutex
	stopped  map[string]chan struct{}
	rootCtx  context.Context
	apiBase  string
}

type pub interface {
	Publish(userID, eventType string, payload any)
}

func NewPollers(reg *Registry, st *store.Store, logger *slog.Logger, hub pub) *Pollers {
	return &Pollers{
		registry: reg, store: st, logger: logger, hub: hub,
		now:     time.Now,
		stopped: make(map[string]chan struct{}),
		rootCtx: context.Background(),
	}
}

func (p *Pollers) WithAPIBase(base string) *Pollers { p.apiBase = base; return p }

// StartForUser launches a poller goroutine. The goroutine's lifetime is bound
// to the server-wide root context (not the caller's ctx, which for an API
// request is cancelled as soon as the response is sent); the returned stop
// channel and StopForUser control per-user shutdown.
func (p *Pollers) StartForUser(_ context.Context, userID string) error {
	p.mu.Lock()
	if _, running := p.stopped[userID]; running {
		p.mu.Unlock()
		return nil
	}
	stop := make(chan struct{})
	p.stopped[userID] = stop
	root := p.rootCtx
	p.mu.Unlock()

	go p.runLoop(root, userID, stop)
	return nil
}

func (p *Pollers) StopForUser(userID string) error {
	p.mu.Lock()
	stop, ok := p.stopped[userID]
	if ok {
		delete(p.stopped, userID)
	}
	p.mu.Unlock()
	if ok {
		close(stop)
	}
	p.registry.Invalidate(userID)
	return nil
}

func (p *Pollers) StartAll(ctx context.Context) error {
	p.mu.Lock()
	p.rootCtx = ctx
	p.mu.Unlock()
	users, err := p.store.Users.ListTelegramEnabled(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if err := p.StartForUser(ctx, u.ID); err != nil {
			p.logger.Warn("telegram poller start failed", "user_id", u.ID, "error", err)
		}
	}
	return nil
}

func (p *Pollers) StopAll() {
	p.mu.Lock()
	for id, stop := range p.stopped {
		delete(p.stopped, id)
		close(stop)
	}
	p.mu.Unlock()
}

func (p *Pollers) runLoop(ctx context.Context, userID string, stop chan struct{}) {
	var offset int64
	backoff := time.Second
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		default:
		}
		client, err := p.registry.GetOrCreate(ctx, userID)
		if err != nil {
			p.logger.Warn("telegram poller: no client", "user_id", userID, "error", err)
			sleepCtx(ctx, stop, backoff)
			backoff = min(backoff*2, 60*time.Second)
			continue
		}
		updates, err := client.GetUpdates(ctx, offset, LongPollSeconds)
		if err != nil {
			p.logger.Warn("telegram getUpdates error", "user_id", userID, "error", err)
			sleepCtx(ctx, stop, backoff)
			backoff = min(backoff*2, 60*time.Second)
			continue
		}
		backoff = time.Second
		for _, u := range updates {
			offset = u.UpdateID + 1
			if p.onUpdate != nil {
				p.onUpdate(userID, u)
			}
		}
	}
}

func sleepCtx(ctx context.Context, stop chan struct{}, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-stop:
	case <-ctx.Done():
	case <-timer.C:
	}
}
