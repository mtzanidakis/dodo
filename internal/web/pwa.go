package web

import (
	"encoding/json"
	"net/http"
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

// handleManifest serves the web app manifest that makes dodo installable.
// It is public because browsers fetch it without credentials, and it is
// rendered rather than shipped as a static file so the icon URLs carry the
// current asset version.
func (h *Handler) handleManifest(w http.ResponseWriter, _ *http.Request) {
	icon := func(name, sizes, purpose string) manifestIcon {
		return manifestIcon{Src: "/static/" + h.deps.Version + "/" + name, Sizes: sizes, Type: "image/png", Purpose: purpose}
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
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(m)
}

// handleServiceWorker serves sw.js from the site root, since a worker's
// scope is limited to the path it is served from. It is revalidated on every
// load so a new deploy replaces the old worker.
func (h *Handler) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	if h.deps.AssetsFS == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, h.deps.AssetsFS, "sw.js")
}
