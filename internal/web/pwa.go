package web

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/mtzanidakis/dodo/internal/i18n"
	"github.com/mtzanidakis/dodo/internal/models"
)

type webManifest struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	Description     string         `json:"description"`
	StartURL        string         `json:"start_url"`
	Scope           string         `json:"scope"`
	Display         string         `json:"display"`
	BackgroundColor string         `json:"background_color"`
	ThemeColor      string         `json:"theme_color"`
	Icons           []manifestIcon `json:"icons"`
}

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

// appIcons are served at stable /icons/ URLs rather than under the
// versioned /static/ path: Chrome treats a changed icon URL in the manifest
// as an app update, which every deploy would otherwise trigger.
var appIcons = map[string]bool{
	"icon-192.png":          true,
	"icon-512.png":          true,
	"icon-maskable-512.png": true,
	"apple-touch-icon.png":  true,
}

// handleManifest is public because browsers fetch the manifest without
// credentials. ?theme=dark gives dark-mode users a dark splash screen.
func (h *Handler) handleManifest(w http.ResponseWriter, r *http.Request) {
	icon := func(name, sizes, purpose string) manifestIcon {
		return manifestIcon{Src: "/icons/" + name, Sizes: sizes, Type: "image/png", Purpose: purpose}
	}
	m := webManifest{
		ID:              "/",
		Name:            "dodo",
		ShortName:       "dodo",
		Description:     "Self-hosted todos and reminders",
		StartURL:        "/",
		Scope:           "/",
		Display:         "standalone",
		BackgroundColor: "#f4f5f7",
		ThemeColor:      "#ffffff",
		Icons: []manifestIcon{
			icon("icon-192.png", "192x192", "any"),
			icon("icon-512.png", "512x512", "any"),
			icon("icon-maskable-512.png", "512x512", "maskable"),
		},
	}
	if r.URL.Query().Get("theme") == "dark" {
		m.BackgroundColor = "#0d0f12"
		m.ThemeColor = "#16191d"
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(m)
}

func (h *Handler) handleIcon(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if h.deps.AssetsFS == nil || !appIcons[name] {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFileFS(w, r, h.deps.AssetsFS, name)
}

// handleServiceWorker serves sw.js from the root so its scope covers the
// whole app, prefixed with the offline page's strings in ?lang. A locale
// change registers a new URL, which makes the browser update the worker.
func (h *Handler) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	if h.deps.AssetsFS == nil {
		http.NotFound(w, r)
		return
	}
	src, err := fs.ReadFile(h.deps.AssetsFS, "sw.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	lang := r.URL.Query().Get("lang")
	if !models.Locale(lang).Valid() {
		lang = string(models.LocaleEn)
	}
	strs, _ := json.Marshal(map[string]string{
		"lang":  lang,
		"title": i18n.T("offline.title", lang),
		"body":  i18n.T("offline.body", lang),
		"retry": i18n.T("offline.retry", lang),
	})
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = fmt.Fprintf(w, "self.DODO_OFFLINE = %s;\n", strs)
	_, _ = w.Write(src)
}
