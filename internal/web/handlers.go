package web

import (
	"context"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/i18n"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/ws"
)

// TelegramConfigurator lets the browser /account page configure Telegram using
// the same validated/encrypted/poller-managed path as the JSON API.
type TelegramConfigurator interface {
	ConfigureTelegram(ctx context.Context, userID, botToken, allowedUserIDs string) (string, error)
	ClearTelegram(ctx context.Context, userID string) error
	TestTelegram(ctx context.Context, userID, chatID string) error
}

type Deps struct {
	Store    *store.Store
	AuthMW   *auth.Middleware
	LoginRL  *auth.LoginRateLimiter
	Hub      *ws.Hub
	Telegram TelegramConfigurator
	AssetsFS fs.FS
	Version  string
}

var funcMap = template.FuncMap{
	"priorityIcon": func(p models.Priority) string { return p.Icon() },
	"t":            func(lang, key string, args ...any) string { return i18n.T(key, lang, args...) },
	"dueLocal":     dueLocal,
	"dict":         dict,
}

func dict(pairs ...any) map[string]any {
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		key, _ := pairs[i].(string)
		m[key] = pairs[i+1]
	}
	return m
}

var baseTemplate = template.Must(template.New("").Funcs(funcMap).ParseFS(templatesFS, "templates/layout.html"))

func pageTemplate(name string) *template.Template {
	clone := template.Must(baseTemplate.Clone())
	clone = template.Must(clone.ParseFS(templatesFS, "templates/tasks/_row.html", "templates/tasks/_list.html"))
	return template.Must(clone.ParseFS(templatesFS, "templates/"+name))
}

// taskListTemplate renders the "tasklist" partial (day-groups + load-more) on
// its own, for htmx load-more responses.
func taskListTemplate() *template.Template {
	return template.Must(template.New("").Funcs(funcMap).ParseFS(templatesFS,
		"templates/tasks/_row.html", "templates/tasks/_list.html"))
}

func fragmentTemplate(name string) *template.Template {
	clone := template.Must(template.New("").Funcs(funcMap).Parse(""))
	return template.Must(clone.ParseFS(templatesFS, "templates/"+name))
}

func dueLocal(iso string, tz string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	return t.In(loadLoc(tz)).Format("Mon 2 Jan 15:04")
}

func loadLoc(tz string) *time.Location {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

type pageData struct {
	Title        string
	Lang         string
	CSRF         string
	ColorScheme  string
	ThemeClass   string
	AssetVersion string
	Nav          string
	User         *models.User
	Error        string
	Flash        string

	// tasks page
	View       string
	Filter     string
	Period     string
	Groups     []dayGroup
	NextCursor string
	Calendar   *calendarView
	Freqs      []freqOption

	// tokens page
	Tokens   []tokenView
	NewToken string

	// account page
	Telegram *telegramView
}

func colorScheme(theme models.Theme) string {
	switch theme {
	case models.ThemeDark:
		return "dark"
	case models.ThemeLight:
		return "light"
	default:
		return "light dark"
	}
}

func themeClass(theme models.Theme) string {
	switch theme {
	case models.ThemeDark:
		return "dark"
	case models.ThemeLight:
		return "light"
	default:
		return ""
	}
}

type Handler struct {
	deps Deps
}

func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

func (h *Handler) render(w http.ResponseWriter, name string, data pageData) {
	data.AssetVersion = h.deps.Version
	tmpl := pageTemplate(name)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.ExecuteTemplate(w, "page", data)
}

// base fills the common per-request fields (auth user, csrf, theme).
func (h *Handler) base(w http.ResponseWriter, r *http.Request, u *models.User, title, nav string) pageData {
	lang := "en"
	scheme := "light dark"
	var cls string
	if u != nil {
		lang = string(u.Locale)
		scheme = colorScheme(u.Theme)
		cls = themeClass(u.Theme)
	}
	return pageData{
		Title:       title,
		Lang:        lang,
		CSRF:        csrfOrNew(w, r),
		ColorScheme: scheme,
		ThemeClass:  cls,
		Nav:         nav,
		User:        u,
	}
}

func (h *Handler) Mount(mux *http.ServeMux) {
	deps := h.deps
	mux.Handle("GET /static/{path...}", http.StripPrefix("/static/"+h.deps.Version+"/", h.assetsHandler()))

	mux.HandleFunc("GET /login", h.handleLoginPage)
	mux.HandleFunc("POST /login", h.handleLoginPost)

	sess := func(fn http.HandlerFunc) http.Handler {
		return deps.AuthMW.AuthSession(deps.AuthMW.RequireUser(http.HandlerFunc(fn)))
	}
	post := func(fn http.HandlerFunc) http.Handler {
		return deps.AuthMW.AuthSession(deps.AuthMW.CSRF(deps.AuthMW.RequireUser(http.HandlerFunc(fn))))
	}

	// Logout is a state change, so it must be a CSRF-protected POST.
	mux.Handle("POST /logout", post(h.handleLogout))

	mux.Handle("GET /{$}", sess(h.handleHome))
	mux.Handle("GET /account", sess(h.handleAccount))
	mux.Handle("POST /account", post(h.handleAccountPost))
	mux.Handle("POST /account/password", post(h.handleAccountPassword))
	mux.Handle("POST /account/telegram", post(h.handleAccountTelegram))
	mux.Handle("POST /account/telegram/clear", post(h.handleAccountTelegramClear))
	mux.Handle("POST /account/telegram/test", post(h.handleAccountTelegramTest))

	mux.Handle("POST /ui/locale", post(h.handleSetLocale))

	mux.Handle("GET /tokens", sess(h.handleTokens))
	mux.Handle("POST /ui/tokens", post(h.handleCreateToken))
	mux.Handle("POST /ui/tokens/{id}/delete", post(h.handleRevokeToken))

	mux.Handle("POST /ui/tasks", post(h.handleCreateTask))
	mux.Handle("GET /ui/tasks/page", sess(h.handleTasksPage))
	mux.Handle("GET /ui/tasks/{id}/edit", sess(h.handleEditTaskPage))
	mux.Handle("POST /ui/tasks/{id}", post(h.handleUpdateTask))
	mux.Handle("POST /ui/tasks/{id}/complete", post(h.handleCompleteTask))
	mux.Handle("POST /ui/tasks/{id}/snooze", post(h.handleSnoozeTask))
	mux.Handle("POST /ui/tasks/{id}/delete", post(h.handleDeleteTask))
}

func (h *Handler) assetsHandler() http.Handler {
	if h.deps.AssetsFS == nil {
		return http.NotFoundHandler()
	}
	fsh := http.FileServer(noDirFS{http.FS(h.deps.AssetsFS)})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fsh.ServeHTTP(w, r)
	})
}

// noDirFS wraps an http.FileSystem so directory requests 404 instead of
// rendering an index listing of asset filenames.
type noDirFS struct{ fs http.FileSystem }

func (n noDirFS) Open(name string) (http.File, error) {
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, fs.ErrNotExist
	}
	return f, nil
}

func (h *Handler) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	csrf := auth.IssueCSRF(w)
	h.render(w, "auth/login.html", pageData{Title: "Sign in", Lang: "en", CSRF: csrf, ColorScheme: "light dark"})
}

func (h *Handler) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")
	loginFailed := func() {
		csrf := auth.IssueCSRF(w)
		h.render(w, "auth/login.html", pageData{Title: "Sign in", Lang: "en", CSRF: csrf, ColorScheme: "light dark", Error: i18n.T("login.failed", "en")})
	}
	if h.deps.LoginRL != nil && !h.deps.LoginRL.Allow(r, email) {
		loginFailed()
		return
	}
	user, err := h.deps.Store.Users.GetByEmail(r.Context(), email)
	if err != nil || !auth.VerifyPassword(password, safeHash(user)) {
		if h.deps.LoginRL != nil {
			h.deps.LoginRL.RecordFailure(r, email)
		}
		loginFailed()
		return
	}
	if h.deps.LoginRL != nil {
		h.deps.LoginRL.Reset(r, email)
	}
	gen, err := auth.GenerateSession()
	if err != nil {
		csrf := auth.IssueCSRF(w)
		h.render(w, "auth/login.html", pageData{Title: "Sign in", CSRF: csrf, Error: err.Error()})
		return
	}
	if _, err := h.deps.Store.Sessions.Create(r.Context(), user.ID, gen.Hash, r.UserAgent(), 30*24*time.Hour); err != nil {
		csrf := auth.IssueCSRF(w)
		h.render(w, "auth/login.html", pageData{Title: "Sign in", CSRF: csrf, Error: err.Error()})
		return
	}
	auth.SetSessionCookie(w, auth.SessionCookieOptions{Value: gen.Full, Secure: auth.IsSecure(r), Duration: 30 * 24 * time.Hour})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func safeHash(u *models.User) string {
	if u == nil {
		return ""
	}
	return u.PasswordHash
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil && c.Value != "" {
		hash := auth.HashToken(c.Value)
		if ses, err := h.deps.Store.Sessions.Lookup(r.Context(), hash); err == nil {
			_ = h.deps.Store.Sessions.Expire(r.Context(), ses.ID)
		}
	}
	auth.ClearSessionCookie(w, auth.IsSecure(r))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func csrfOrNew(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(auth.CSRFCookie); err == nil && c.Value != "" {
		return c.Value
	}
	return auth.IssueCSRF(w)
}

var _ = context.Background
