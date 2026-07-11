package web

import (
	"context"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/mtzanidakis/dodo/internal/auth"
	"github.com/mtzanidakis/dodo/internal/models"
	"github.com/mtzanidakis/dodo/internal/recurrence"
	"github.com/mtzanidakis/dodo/internal/store"
	"github.com/mtzanidakis/dodo/internal/ws"
)

type Deps struct {
	Store       *store.Store
	AuthMW      *auth.Middleware
	Hub         *ws.Hub
	AssetsFS    fs.FS
	Version     string
	TemplatesFS embed.FS
}

var baseTemplate = template.Must(template.New("").Funcs(template.FuncMap{
	"priorityIcon": func(p models.Priority) string { return p.Icon() },
	"dueLocal":     dueLocal,
}).ParseFS(templatesFS, "templates/layout.html"))

func pageTemplate(name string) *template.Template {
	clone := template.Must(baseTemplate.Clone())
	return template.Must(clone.ParseFS(templatesFS, "templates/"+name))
}

func dueLocal(iso string, tz string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("Mon 15:04")
}

type pageData struct {
	Title        string
	Lang         string
	CSRF         string
	ColorScheme  string
	AssetVersion string
	User         *models.User
	Tasks        []taskView
	Error        string
}

type taskView struct {
	ID          string
	Title       string
	Priority    models.Priority
	DueAt       string
	CompletedAt *string
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

func (h *Handler) Mount(mux *http.ServeMux) {
	deps := h.deps
	mux.Handle("GET /static/", http.StripPrefix("/static/", h.assetsHandler()))

	mux.HandleFunc("GET /login", h.handleLoginPage)
	mux.HandleFunc("POST /login", h.handleLoginPost)
	mux.HandleFunc("GET /logout", h.handleLogout)

	mux.Handle("GET /", deps.AuthMW.AuthSession(http.HandlerFunc(h.handleHome)))
	mux.Handle("GET /account", deps.AuthMW.AuthSession(deps.AuthMW.RequireUser(http.HandlerFunc(h.handleAccount))))
	mux.Handle("POST /account", deps.AuthMW.AuthSession(deps.AuthMW.CSRF(deps.AuthMW.RequireUser(http.HandlerFunc(h.handleAccountPost)))))
	mux.Handle("POST /account/password", deps.AuthMW.AuthSession(deps.AuthMW.CSRF(deps.AuthMW.RequireUser(http.HandlerFunc(h.handleAccountPassword)))))
	mux.Handle("POST /account/telegram", deps.AuthMW.AuthSession(deps.AuthMW.CSRF(deps.AuthMW.RequireUser(http.HandlerFunc(h.handleAccountTelegram)))))

	mux.Handle("POST /ui/tasks/{id}/complete", deps.AuthMW.AuthSession(deps.AuthMW.CSRF(deps.AuthMW.RequireUser(http.HandlerFunc(h.handleCompleteTask)))))
}

func (h *Handler) assetsHandler() http.Handler {
	if h.deps.AssetsFS == nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(h.deps.AssetsFS))
}

func (h *Handler) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	csrf := auth.IssueCSRF(w)
	h.render(w, "auth/login.html", pageData{Title: "Sign in", Lang: "en", CSRF: csrf, ColorScheme: "light dark"})
}

func (h *Handler) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")
	User, err := h.deps.Store.Users.GetByEmail(r.Context(), email)
	if err != nil || !auth.VerifyPassword(password, safeHash(User)) {
		csrf := auth.IssueCSRF(w)
		h.render(w, "auth/login.html", pageData{Title: "Sign in", Lang: "en", CSRF: csrf, ColorScheme: "light dark", Error: "Invalid email or password"})
		return
	}
	gen, err := auth.GenerateSession()
	if err != nil {
		csrf := auth.IssueCSRF(w)
		h.render(w, "auth/login.html", pageData{Title: "Sign in", CSRF: csrf, Error: err.Error()})
		return
	}
	if _, err := h.deps.Store.Sessions.Create(r.Context(), User.ID, gen.Hash, r.UserAgent(), 30*24*time.Hour); err != nil {
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

func (h *Handler) handleHome(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	if u == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "pending"
	}
	tasks, _, _ := h.deps.Store.Tasks.List(r.Context(), u.ID, models.TaskFilter{Filter: filter, Limit: 100})
	views := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		views = append(views, taskView{ID: t.ID, Title: t.Title, Priority: t.Priority, DueAt: t.DueAt.UTC().Format(time.RFC3339), CompletedAt: completedAtStr(t)})
	}
	csrf := csrfOrNew(w, r)
	h.render(w, "tasks/index.html", pageData{
		Title: "Tasks", Lang: string(u.Locale), CSRF: csrf, ColorScheme: colorScheme(u.Theme),
		User: u, Tasks: views,
	})
}

func completedAtStr(t *models.Task) *string {
	if t.CompletedAt == nil {
		return nil
	}
	s := t.CompletedAt.UTC().Format(time.RFC3339)
	return &s
}

func csrfOrNew(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(auth.CSRFCookie); err == nil && c.Value != "" {
		return c.Value
	}
	return auth.IssueCSRF(w)
}

func (h *Handler) handleCompleteTask(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	loc, _ := time.LoadLocation(u.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().UTC()
	t, _, _, err := h.deps.Store.Tasks.Complete(r.Context(), u.ID, id, now, func(t *models.Task, n time.Time) (*models.TaskCompletion, bool, error) {
		if !t.Recurring() {
			return nil, false, nil
		}
		rule := recurrence.Rule{Freq: *t.RecurrenceFreq, Interval: t.RecurrenceInterval}
		next := recurrence.NextOccurrence(rule, t.DueAt, t.DueAt, loc)
		if next.IsZero() {
			t.CompletedAt = &n
			t.RecurrenceFreq = nil
			t.Kind = models.KindOneoff
			return nil, true, nil
		}
		t.DueAt = next
		t.LastNotifiedAt = nil
		return nil, false, nil
	})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.deps.Hub.Publish(u.ID, "task.completed", map[string]any{"id": id})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	rowTmpl := template.Must(baseTemplate.Clone())
	rowTmpl = template.Must(rowTmpl.Funcs(template.FuncMap{"priorityIcon": func(p models.Priority) string { return p.Icon() }}).ParseFS(templatesFS, "templates/tasks/_completed_row.html"))
	_ = rowTmpl.ExecuteTemplate(w, "", taskView{ID: t.ID, Title: t.Title, Priority: t.Priority, DueAt: t.DueAt.UTC().Format(time.RFC3339)})
}

func ptrString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func (h *Handler) handleAccount(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	csrf := csrfOrNew(w, r)
	h.render(w, "account/index.html", pageData{Title: "Account", Lang: string(u.Locale), CSRF: csrf, ColorScheme: colorScheme(u.Theme), User: u})
}

func (h *Handler) handleAccountPost(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	u.DisplayName = r.FormValue("display_name")
	u.Timezone = r.FormValue("timezone")
	u.Locale = models.Locale(r.FormValue("locale"))
	u.Theme = models.Theme(r.FormValue("theme"))
	_ = h.deps.Store.Users.Update(r.Context(), u)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (h *Handler) handleAccountPassword(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	current := r.FormValue("current_password")
	newpw := r.FormValue("new_password")
	if len(newpw) < 8 || !auth.VerifyPassword(current, u.PasswordHash) {
		http.Error(w, "invalid", http.StatusBadRequest)
		return
	}
	hash, _ := auth.HashPassword(newpw)
	_ = h.deps.Store.Users.UpdatePassword(r.Context(), u.ID, hash)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (h *Handler) handleAccountTelegram(w http.ResponseWriter, r *http.Request) {
	_ = r.FormValue("bot_token")
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

var _ http.Handler = http.NotFoundHandler()
var _ context.Context
