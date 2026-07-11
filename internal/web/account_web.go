package web

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/i18n"
	"github.com/mtzanidakis/dodo/internal/models"
)

// handleSetLocale switches the UI language from the topbar selector and returns
// to the page the request came from.
func (h *Handler) handleSetLocale(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	if loc := r.FormValue("locale"); loc == "en" || loc == "el" {
		u.Locale = models.Locale(loc)
		_ = h.deps.Store.Users.Update(r.Context(), u)
	}
	http.Redirect(w, r, sameOriginRef(r), http.StatusSeeOther)
}

// sameOriginRef returns the path+query of the request's Referer (dropping the
// host to avoid open redirects); falls back to "/".
func sameOriginRef(r *http.Request) string {
	ref := r.Referer()
	if ref == "" {
		return "/"
	}
	parsed, err := url.Parse(ref)
	if err != nil || parsed.Path == "" {
		return "/"
	}
	target := parsed.Path
	if parsed.RawQuery != "" {
		target += "?" + parsed.RawQuery
	}
	return target
}

type tokenView struct {
	ID         string
	Name       string
	Prefix     string
	LastUsedAt string
	CreatedAt  string
}

type telegramView struct {
	Configured   bool
	AllowedIDs   string
	ChatID       string
	Linked       bool
	ConfiguredAt string
}

func (h *Handler) handleAccount(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	pd := h.base(w, r, u, i18n.T("nav.account", string(u.Locale)), "account")
	pd.Telegram = telegramViewFor(u)
	pd.Flash = r.URL.Query().Get("ok")
	pd.Error = r.URL.Query().Get("err")
	h.render(w, "account/index.html", pd)
}

func telegramViewFor(u *models.User) *telegramView {
	tv := &telegramView{
		AllowedIDs: u.TelegramAllowedIDs,
		ChatID:     u.TelegramChatID,
		Linked:     u.TelegramChatID != "",
	}
	if u.TelegramConfiguredAt != nil {
		tv.Configured = true
		tv.ConfiguredAt = u.TelegramConfiguredAt.Format("2006-01-02 15:04")
	}
	return tv
}

func (h *Handler) handleAccountPost(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	if dn := strings.TrimSpace(r.FormValue("display_name")); dn != "" {
		u.DisplayName = dn
	}
	if tz := strings.TrimSpace(r.FormValue("timezone")); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			u.Timezone = tz
		}
	}
	if loc := r.FormValue("locale"); loc == "en" || loc == "el" {
		u.Locale = models.Locale(loc)
	}
	switch r.FormValue("theme") {
	case "light":
		u.Theme = models.ThemeLight
	case "dark":
		u.Theme = models.ThemeDark
	case "system":
		u.Theme = models.ThemeSystem
	}
	_ = h.deps.Store.Users.Update(r.Context(), u)
	http.Redirect(w, r, "/account?ok="+i18n.T("account.saved", string(u.Locale)), http.StatusSeeOther)
}

func (h *Handler) handleAccountPassword(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	current := r.FormValue("current_password")
	newpw := r.FormValue("new_password")
	if len(newpw) < 8 || !auth.VerifyPassword(current, u.PasswordHash) {
		http.Redirect(w, r, "/account?err="+i18n.T("password.invalid", string(u.Locale)), http.StatusSeeOther)
		return
	}
	hash, err := auth.HashPassword(newpw)
	if err == nil {
		err = h.deps.Store.Users.UpdatePassword(r.Context(), u.ID, hash)
	}
	if err != nil {
		http.Redirect(w, r, "/account?err="+i18n.T("password.invalid", string(u.Locale)), http.StatusSeeOther)
		return
	}
	// Invalidate every outstanding session (including any stolen cookie), then
	// issue a fresh one so this browser stays signed in.
	_ = h.deps.Store.Sessions.DeleteByUser(r.Context(), u.ID)
	if gen, gErr := auth.GenerateSession(); gErr == nil {
		if _, cErr := h.deps.Store.Sessions.Create(r.Context(), u.ID, gen.Hash, r.UserAgent(), 30*24*time.Hour); cErr == nil {
			auth.SetSessionCookie(w, auth.SessionCookieOptions{Value: gen.Full, Secure: auth.IsSecure(r), Duration: 30 * 24 * time.Hour})
		}
	}
	http.Redirect(w, r, "/account?ok="+i18n.T("password.changed", string(u.Locale)), http.StatusSeeOther)
}

func (h *Handler) handleAccountTelegram(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	lang := string(u.Locale)
	token := strings.TrimSpace(r.FormValue("bot_token"))
	allowed := strings.TrimSpace(r.FormValue("allowed_user_ids"))
	if h.deps.Telegram == nil || token == "" {
		http.Redirect(w, r, "/account?err="+i18n.T("telegram.token_required", lang), http.StatusSeeOther)
		return
	}
	if _, err := h.deps.Telegram.ConfigureTelegram(r.Context(), u.ID, token, allowed); err != nil {
		http.Redirect(w, r, "/account?err="+i18n.T("telegram.invalid_token", lang), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/account?ok="+i18n.T("telegram.status_configured", lang), http.StatusSeeOther)
}

func (h *Handler) handleAccountTelegramClear(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	if h.deps.Telegram != nil {
		_ = h.deps.Telegram.ClearTelegram(r.Context(), u.ID)
	}
	http.Redirect(w, r, "/account?ok="+i18n.T("telegram.status_not_configured", string(u.Locale)), http.StatusSeeOther)
}

func (h *Handler) handleAccountTelegramTest(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	lang := string(u.Locale)
	if h.deps.Telegram == nil || u.TelegramChatID == "" {
		http.Redirect(w, r, "/account?err="+i18n.T("telegram.not_linked", lang), http.StatusSeeOther)
		return
	}
	if err := h.deps.Telegram.TestTelegram(r.Context(), u.ID, u.TelegramChatID); err != nil {
		http.Redirect(w, r, "/account?err="+i18n.T("telegram.test_failed", lang), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/account?ok="+i18n.T("telegram.test_sent", lang), http.StatusSeeOther)
}

// ---- tokens ---------------------------------------------------------------

func (h *Handler) handleTokens(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	pd := h.base(w, r, u, i18n.T("nav.tokens", string(u.Locale)), "tokens")
	pd.Tokens = h.listTokenViews(r, u.ID)
	pd.NewToken = takeNewToken(w, r)
	h.render(w, "tokens/index.html", pd)
}

func (h *Handler) listTokenViews(r *http.Request, userID string) []tokenView {
	tokens, _ := h.deps.Store.Tokens.List(r.Context(), userID)
	views := make([]tokenView, 0, len(tokens))
	for _, t := range tokens {
		if t.Revoked() {
			continue
		}
		tv := tokenView{ID: t.ID, Name: t.Name, Prefix: t.TokenPrefix, CreatedAt: t.CreatedAt.Format("2006-01-02")}
		if t.LastUsedAt != nil {
			tv.LastUsedAt = t.LastUsedAt.Format("2006-01-02 15:04")
		}
		views = append(views, tv)
	}
	return views
}

func (h *Handler) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/tokens", http.StatusSeeOther)
		return
	}
	gen, err := auth.GenerateAPIToken()
	if err != nil {
		http.Redirect(w, r, "/tokens", http.StatusSeeOther)
		return
	}
	if _, err := h.deps.Store.Tokens.Create(r.Context(), u.ID, name, gen.Prefix, gen.Hash); err != nil {
		http.Redirect(w, r, "/tokens", http.StatusSeeOther)
		return
	}
	h.deps.Hub.Publish(u.ID, "tokens.updated", nil)
	// Hand the plaintext token to the next page via a short-lived, HttpOnly,
	// one-time cookie instead of the URL, so the secret never lands in browser
	// history, access logs, or Referer headers.
	http.SetCookie(w, &http.Cookie{
		Name:     newTokenCookie,
		Value:    gen.Full,
		Path:     "/tokens",
		MaxAge:   120,
		HttpOnly: true,
		Secure:   auth.IsSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

const newTokenCookie = "dodo_new_token"

// takeNewToken reads and immediately clears the one-time new-token cookie so
// the plaintext value is shown exactly once.
func takeNewToken(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(newTokenCookie)
	if err != nil || c.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     newTokenCookie,
		Value:    "",
		Path:     "/tokens",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   auth.IsSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
	return c.Value
}

func (h *Handler) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	_ = h.deps.Store.Tokens.Revoke(r.Context(), u.ID, r.PathValue("id"))
	h.deps.Hub.Publish(u.ID, "tokens.updated", nil)
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}
