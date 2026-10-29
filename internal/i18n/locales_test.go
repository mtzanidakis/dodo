package i18n

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mtzanidakis/dodo/internal/models"
)

var placeholder = regexp.MustCompile(`\{[0-9]+\}`)

func placeholders(s string) []string {
	p := placeholder.FindAllString(s, -1)
	slices.Sort(p)
	return p
}

// Compares the raw locale maps: T and Bundle.Get fall back to English, so a
// missing translation would never show up through them.
func TestLocalesMatchEnglish(t *testing.T) {
	loadAll()
	en := loaded["en"]
	if len(en) == 0 {
		t.Fatal("en locale not loaded")
	}
	for lang, m := range loaded {
		for k, v := range en {
			got, ok := m[k]
			if !ok {
				t.Errorf("%s: missing %q", lang, k)
				continue
			}
			if strings.TrimSpace(got) == "" {
				t.Errorf("%s: empty %q", lang, k)
			}
			if !slices.Equal(placeholders(got), placeholders(v)) {
				t.Errorf("%s: %q placeholders %v, en has %v", lang, k, placeholders(got), placeholders(v))
			}
		}
		for k := range m {
			if _, ok := en[k]; !ok {
				t.Errorf("%s: %q is not in en", lang, k)
			}
		}
	}
}

func TestLocaleFilesMatchSupportedLocales(t *testing.T) {
	loadAll()
	for _, l := range models.Locales {
		if _, ok := loaded[string(l)]; !ok {
			t.Errorf("supported locale %q has no locales/%s.json", l, l)
		}
	}
	for lang := range loaded {
		if !models.Locale(lang).Valid() {
			t.Errorf("locales/%s.json is not in models.Locales", lang)
		}
	}
}
