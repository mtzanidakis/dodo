package i18n_test

import (
	"strings"
	"testing"

	"github.com/mtzanidakis/dodo/internal/i18n"
)

func TestTInterpolation(t *testing.T) {
	got := i18n.T("reminder.due_label", "en", 1, 2)
	if got != "due" {
		t.Fatalf("plain key: got %q", got)
	}
}

func TestTGreek(t *testing.T) {
	got := i18n.T("nav.tasks", "el")
	if got != "Εργασίες" {
		t.Fatalf("el nav.tasks: got %q", got)
	}
}

func TestTFallbackToEN(t *testing.T) {
	got := i18n.T("nav.tasks", "xx")
	if got != "Tasks" {
		t.Fatalf("fallback should be en, got %q", got)
	}
}

func TestTMissingKeyReturnsKey(t *testing.T) {
	got := i18n.T("does.not.exist", "en")
	if got != "does.not.exist" {
		t.Fatalf("missing key should return itself, got %q", got)
	}
}

func TestBundleGet(t *testing.T) {
	b := i18n.BundleFor("en")
	if b.Get("action.complete") != "Complete" {
		t.Fatalf("bundle get mismatch: %q", b.Get("action.complete"))
	}
}

func TestAllKeysExistInBothLocales(t *testing.T) {
	en := i18n.BundleFor("en")
	el := i18n.BundleFor("el")
	for _, k := range i18n.Keys() {
		if strings.TrimSpace(en.Get(k)) == "" {
			t.Fatalf("en missing value for %q", k)
		}
		if strings.TrimSpace(el.Get(k)) == "" {
			t.Fatalf("el missing value for %q", k)
		}
	}
}

func TestArgsPlaceholder(t *testing.T) {
	got := i18n.T("reminder.summary", "en", "Buy milk", "10:00", "high")
	if got != "Buy milk (due 10:00, priority high)" {
		t.Fatalf("interpolation mismatch: %q", got)
	}
}
