package auth

import (
	"net/http"
	"time"
)

type SessionCookieOptions struct {
	Value    string
	Secure   bool
	Duration time.Duration
}

func SetSessionCookie(w http.ResponseWriter, opts SessionCookieOptions) {
	maxAge := int(opts.Duration.Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    opts.Value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   opts.Secure,
	})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func IsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if xfp := r.Header.Get("X-Forwarded-Proto"); xfp == "https" {
		return true
	}
	return false
}
