package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type LoginRateLimiter struct {
	mu        sync.Mutex
	fails     map[string]int
	firstAt   map[string]time.Time
	lastSweep time.Time
	limit     int
	window    time.Duration
}

func NewLoginRateLimiter() *LoginRateLimiter {
	return &LoginRateLimiter{
		fails:   make(map[string]int),
		firstAt: make(map[string]time.Time),
		limit:   10,
		window:  15 * time.Minute,
	}
}

// sweep drops entries whose window has elapsed. Bounded to run at most once per
// window so a client spraying many distinct emails/IPs can't grow the maps
// without bound. The caller must hold l.mu.
func (l *LoginRateLimiter) sweep(now time.Time) {
	if !l.lastSweep.IsZero() && now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	for k, t := range l.firstAt {
		if now.Sub(t) > l.window {
			delete(l.firstAt, k)
			delete(l.fails, k)
		}
	}
}

func (l *LoginRateLimiter) key(r *http.Request, email string) string {
	// RemoteAddr is "host:port"; the ephemeral port changes on every new
	// connection, so keying on it lets an attacker reset the counter per
	// attempt. Key on the host only.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host + "|" + email
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
	l.sweep(time.Now())
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
