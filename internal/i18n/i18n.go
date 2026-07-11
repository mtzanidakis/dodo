package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed locales/en.json locales/el.json
var localeFS embed.FS

type Bundle struct {
	lang string
	strs map[string]string
}

var (
	mu       sync.RWMutex
	loaded   = make(map[string]map[string]string)
	loadOnce sync.Once
)

func loadAll() {
	loadOnce.Do(func() {
		for _, name := range []string{"en", "el"} {
			b, err := localeFS.ReadFile("locales/" + name + ".json")
			if err != nil {
				continue
			}
			m := make(map[string]string)
			if err := json.Unmarshal(b, &m); err != nil {
				continue
			}
			loaded[name] = m
		}
	})
}

func BundleFor(lang string) Bundle {
	loadAll()
	mu.RLock()
	defer mu.RUnlock()
	if b, ok := loaded[lang]; ok {
		return Bundle{lang: lang, strs: b}
	}
	if b, ok := loaded["en"]; ok {
		return Bundle{lang: "en", strs: b}
	}
	return Bundle{lang: lang, strs: map[string]string{}}
}

func T(key, lang string, args ...any) string {
	loadAll()
	mu.RLock()
	m, ok := loaded[lang]
	if !ok {
		m = loaded["en"]
	}
	mu.RUnlock()
	s, ok := m[key]
	if !ok {
		return key
	}
	return interpolate(s, args...)
}

func (b Bundle) Get(key string, args ...any) string {
	s, ok := b.strs[key]
	if !ok {
		if fallback, ok := loaded["en"][key]; ok {
			return interpolate(fallback, args...)
		}
		return key
	}
	return interpolate(s, args...)
}

func Available() []string {
	loadAll()
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(loaded))
	for k := range loaded {
		out = append(out, k)
	}
	return out
}

func Keys() []string {
	loadAll()
	mu.RLock()
	defer mu.RUnlock()
	if m, ok := loaded["en"]; ok {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	return nil
}

func interpolate(s string, args ...any) string {
	if len(args) == 0 {
		return s
	}
	for i, a := range args {
		s = strings.ReplaceAll(s, fmt.Sprintf("{%d}", i), fmt.Sprint(a))
	}
	return s
}
