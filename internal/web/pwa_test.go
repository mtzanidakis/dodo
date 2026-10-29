package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestManifestIsPublicAndInstallable(t *testing.T) {
	t.Parallel()
	mux, _, _, _ := newWebEnv(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("manifest: %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/manifest+json" {
		t.Fatalf("content-type: %q", ct)
	}
	var m webManifest
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Name == "" || m.StartURL != "/" || m.Scope != "/" || m.Display != "standalone" {
		t.Fatalf("manifest fields: %+v", m)
	}
	sizes := map[string]bool{}
	maskable := false
	for _, ic := range m.Icons {
		if !strings.HasPrefix(ic.Src, "/static/test/") {
			t.Fatalf("icon not versioned: %q", ic.Src)
		}
		sizes[ic.Sizes] = true
		maskable = maskable || ic.Purpose == "maskable"
	}
	if !sizes["192x192"] || !sizes["512x512"] || !maskable {
		t.Fatalf("icons: %+v", m.Icons)
	}
}

func TestServiceWorkerServedFromRoot(t *testing.T) {
	t.Parallel()
	h := NewHandler(Deps{Version: "test", AssetsFS: fstest.MapFS{"sw.js": {Data: []byte("self.x=1;")}}})
	mux := http.NewServeMux()
	h.Mount(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sw.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("sw.js: %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("content-type: %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("cache-control: %q", cc)
	}
	if rec.Body.String() != "self.x=1;" {
		t.Fatalf("body: %q", rec.Body.String())
	}
}

func TestServiceWorkerMissingAssets(t *testing.T) {
	t.Parallel()
	mux, _, _, _ := newWebEnv(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sw.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("sw.js without assets: %d", rec.Code)
	}
}

func TestLayoutLinksManifest(t *testing.T) {
	t.Parallel()
	mux, _, _, _ := newWebEnv(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`<link rel="manifest" href="/manifest.webmanifest">`,
		`<link rel="apple-touch-icon" href="/static/test/apple-touch-icon.png">`,
		`<meta name="theme-color"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("login page missing %s", want)
		}
	}
}
