package auth

import (
	"net/http"
	"sync"
	"time"
)

type LoginRateLimiter struct {
	mu      sync.Mutex
	fails   map[string]int
	firstAt map[string]time.Time
	limit   int
	window  time.Duration
}

func NewLoginRateLimiter() *LoginRateLimiter {
	return &LoginRateLimiter{
		fails:   make(map[string]int),
		firstAt: make(map[string]time.Time),
		limit:   10,
		window:  15 * time.Minute,
	}
}

func (l *LoginRateLimiter) key(r *http.Request, email string) string {
	return r.RemoteAddr + "|" + email
}

func (l *LoginRateLimiter) Allow(r *http.Request, email string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := l.key(r, email)
	if t, ok := l.firstAt[k]; ok && time.Since(t) > l.window {
		delete(l.fails, k)
		delete(l.firstAt, k)
	}
	return l.fails[k] < l.limit
}

func (l *LoginRateLimiter) RecordFailure(r *http.Request, email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := l.key(r, email)
	if _, ok := l.firstAt[k]; !ok {
		l.firstAt[k] = time.Now()
	}
	l.fails[k]++
}

func (l *LoginRateLimiter) Reset(r *http.Request, email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := l.key(r, email)
	delete(l.fails, k)
	delete(l.firstAt, k)
}
