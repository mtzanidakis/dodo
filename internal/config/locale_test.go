package config

import (
	"strings"
	"testing"
)

func TestValidateLocale(t *testing.T) {
	t.Parallel()
	for _, l := range []string{"en", "el", "it"} {
		if err := validateLocale(l); err != nil {
			t.Errorf("%s: %v", l, err)
		}
	}
	err := validateLocale("fr")
	if err == nil || !strings.Contains(err.Error(), "want en|el|it") {
		t.Fatalf("fr: %v", err)
	}
}
