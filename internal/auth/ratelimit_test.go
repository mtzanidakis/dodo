package auth

import (
	"net/http"
	"testing"
)

func req(remoteAddr string) *http.Request {
	return &http.Request{RemoteAddr: remoteAddr}
}

// A new TCP connection gets a fresh ephemeral port; the limiter must still
// count those attempts against the same client, otherwise the lockout is
// trivially bypassable by reconnecting per attempt.
func TestRateLimiterKeysOnHostNotPort(t *testing.T) {
	l := NewLoginRateLimiter()
	const email = "user@example.com"

	for i := 0; i < l.limit; i++ {
		port := 40000 + i
		r := req("203.0.113.5:" + itoa(port))
		if !l.Allow(r, email) {
			t.Fatalf("attempt %d should be allowed", i)
		}
		l.RecordFailure(r, email)
	}

	// A brand-new connection (different port) must now be blocked.
	if l.Allow(req("203.0.113.5:59999"), email) {
		t.Fatal("client should be locked out regardless of source port")
	}
	// A different host is unaffected.
	if !l.Allow(req("198.51.100.9:40000"), email) {
		t.Fatal("a different host must not be locked out")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
