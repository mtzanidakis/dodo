package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mtzanidakis/dodo/internal/i18n"
	"github.com/mtzanidakis/dodo/internal/models"
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
		if !appIcons[strings.TrimPrefix(ic.Src, "/icons/")] {
			t.Fatalf("icon not at a stable /icons/ path: %q", ic.Src)
		}
		sizes[ic.Sizes] = true
		maskable = maskable || ic.Purpose == "maskable"
	}
	if !sizes["192x192"] || !sizes["512x512"] || !maskable {
		t.Fatalf("icons: %+v", m.Icons)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/manifest.webmanifest?theme=dark", nil))
	var dark webManifest
	if err := json.Unmarshal(rec.Body.Bytes(), &dark); err != nil {
		t.Fatalf("decode dark: %v", err)
	}
	if dark.BackgroundColor == m.BackgroundColor || dark.ThemeColor == m.ThemeColor {
		t.Fatalf("dark manifest colors unchanged: %+v", dark)
	}
}

func TestIconsServedAtStablePaths(t *testing.T) {
	t.Parallel()
	h := NewHandler(Deps{Version: "test", AssetsFS: fstest.MapFS{
		"icon-192.png": {Data: []byte("png")},
		"app.js":       {Data: []byte("js")},
	}})
	mux := http.NewServeMux()
	h.Mount(mux)
	cases := []struct {
		path string
		code int
	}{
		{"/icons/icon-192.png", http.StatusOK},
		{"/icons/app.js", http.StatusNotFound},
		{"/icons/icon-512.png", http.StatusNotFound},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.code {
			t.Fatalf("%s: %d, want %d", tc.path, rec.Code, tc.code)
		}
		if tc.code == http.StatusOK {
			if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age") {
				t.Fatalf("%s cache-control: %q", tc.path, cc)
			}
		}
	}
}

func TestServiceWorkerServedFromRoot(t *testing.T) {
	t.Parallel()
	h := NewHandler(Deps{Version: "test", AssetsFS: fstest.MapFS{"sw.js": {Data: []byte("self.x=1;")}}})
	mux := http.NewServeMux()
	h.Mount(mux)
	cases := []struct {
		url, lang, title string
	}{
		{"/sw.js?lang=el", "el", i18n.T("offline.title", "el")},
		{"/sw.js?lang=en", "en", i18n.T("offline.title", "en")},
		{"/sw.js?lang=xx", "en", i18n.T("offline.title", "en")},
		{"/sw.js", "en", i18n.T("offline.title", "en")},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", tc.url, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
			t.Fatalf("%s content-type: %q", tc.url, ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Fatalf("%s cache-control: %q", tc.url, cc)
		}
		body := rec.Body.String()
		prefix, src, ok := strings.Cut(body, "\n")
		if !ok || src != "self.x=1;" {
			t.Fatalf("%s body: %q", tc.url, body)
		}
		var strs map[string]string
		if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(prefix, "self.DODO_OFFLINE = "), ";")), &strs); err != nil {
			t.Fatalf("%s prefix %q: %v", tc.url, prefix, err)
		}
		if strs["lang"] != tc.lang || strs["title"] != tc.title || strs["body"] == "" || strs["retry"] == "" {
			t.Fatalf("%s strings: %v", tc.url, strs)
		}
	}
	if i18n.T("offline.title", "el") == i18n.T("offline.title", "en") {
		t.Fatalf("offline.title not translated to el")
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
		`<link rel="apple-touch-icon" href="/icons/apple-touch-icon.png">`,
		`<meta name="theme-color"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("login page missing %s", want)
		}
	}
}

func TestThemeColorFollowsSavedTheme(t *testing.T) {
	t.Parallel()
	cases := []struct {
		theme    models.Theme
		metas    []string
		manifest string
	}{
		{models.ThemeSystem, []string{"#ffffff", "#16191d"}, `href="/manifest.webmanifest"`},
		{models.ThemeLight, []string{"#ffffff", "#ffffff"}, `href="/manifest.webmanifest"`},
		{models.ThemeDark, []string{"#16191d", "#16191d"}, `href="/manifest.webmanifest?theme=dark"`},
	}
	for _, tc := range cases {
		mux, st, u, session := newWebEnv(t)
		u.Theme = tc.theme
		if err := st.Users.Update(context.Background(), u); err != nil {
			t.Fatalf("update user: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		withSession(req, session)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		body := rec.Body.String()
		for i, scheme := range []string{"light", "dark"} {
			want := `<meta name="theme-color" content="` + tc.metas[i] + `" media="(prefers-color-scheme: ` + scheme + `)">`
			if !strings.Contains(body, want) {
				t.Fatalf("%s: missing %s", tc.theme, want)
			}
		}
		if !strings.Contains(body, tc.manifest) {
			t.Fatalf("%s: missing manifest %s", tc.theme, tc.manifest)
		}
	}
}

func TestNotificationTextIsLocalized(t *testing.T) {
	t.Parallel()
	mux, st, u, session := newWebEnv(t)
	u.Locale = models.LocaleEl
	if err := st.Users.Update(context.Background(), u); err != nil {
		t.Fatalf("update user: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	withSession(req, session)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	want := `data-notify-due="` + i18n.T("notify.due_now", "el") + `"`
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("missing %s", want)
	}
	if i18n.T("notify.due_now", "el") == i18n.T("notify.due_now", "en") {
		t.Fatalf("notify.due_now not translated to el")
	}
}
