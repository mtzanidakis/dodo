package dateformat_test

import (
	"testing"
	"time"

	"github.com/mtzanidakis/dodo/internal/dateformat"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		pattern string
		ok      bool
	}{
		{"auto", dateformat.Auto, true},
		{"slashes", "DD/MM/YYYY", true},
		{"us", "MM/DD/YYYY", true},
		{"iso", "YYYY-MM-DD", true},
		{"short month", "D MMM YYYY", true},
		{"long month", "D MMMM YYYY", true},
		{"two digit year", "DD.MM.YY", true},
		{"with time", "DD/MM/YYYY HH:mm", true},
		{"blank", "   ", false},
		{"stray letters", "day D of MMM YYYY", false},
		{"no year", "DD/MM", false},
		{"no month", "DD YYYY", false},
		{"no day", "MM/YYYY", false},
		{"duplicate month", "MM/MMM/YYYY", false},
		{"too long", "DD/MM/YYYY DD/MM/YYYY DD/MM/YYYY", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := dateformat.Validate(tc.pattern)
			if tc.ok && err != nil {
				t.Fatalf("Validate(%q) = %v, want nil", tc.pattern, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("Validate(%q) = nil, want error", tc.pattern)
			}
		})
	}
}

func TestRender(t *testing.T) {
	t.Parallel()
	ref := time.Date(2026, 7, 5, 9, 4, 0, 0, time.UTC)
	tests := []struct {
		pattern string
		want    string
	}{
		{"DD/MM/YYYY", "05/07/2026"},
		{"MM/DD/YYYY", "07/05/2026"},
		{"YYYY-MM-DD", "2026-07-05"},
		{"D MMM YYYY", "5 Jul 2026"},
		{"D MMMM YYYY", "5 July 2026"},
		{"D/M/YY", "5/7/26"},
		{"DD/MM/YYYY HH:mm", "05/07/2026 09:04"},
	}
	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			t.Parallel()
			if got := dateformat.Render(ref, tc.pattern); got != tc.want {
				t.Fatalf("Render(%q) = %q, want %q", tc.pattern, got, tc.want)
			}
		})
	}
}

func TestRenderInvalidReturnsEmpty(t *testing.T) {
	t.Parallel()
	ref := time.Date(2026, 7, 5, 9, 4, 0, 0, time.UTC)
	for _, p := range []string{dateformat.Auto, "nonsense", "DD/MM"} {
		if got := dateformat.Render(ref, p); got != "" {
			t.Fatalf("Render(%q) = %q, want empty so callers fall back", p, got)
		}
	}
}

func TestParseRoundTrip(t *testing.T) {
	t.Parallel()
	loc, err := time.LoadLocation("Europe/Athens")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	want := time.Date(2026, 7, 5, 9, 4, 0, 0, loc)
	for _, p := range append([]string{}, dateformat.Presets...) {
		p := dateformat.WithTime(p)
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			got, err := dateformat.Parse(dateformat.Render(want, p), p, loc)
			if err != nil {
				t.Fatalf("Parse(%q): %v", p, err)
			}
			if !got.Equal(want) {
				t.Fatalf("round trip through %q = %v, want %v", p, got, want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		pattern string
		want    time.Time
		wantErr bool
	}{
		{"dmy", "05/07/2026", "DD/MM/YYYY", time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), false},
		{"mdy", "07/05/2026", "MM/DD/YYYY", time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), false},
		{"unpadded", "5/7/2026", "D/M/YYYY", time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), false},
		{"month name", "5 Jul 2026", "D MMM YYYY", time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), false},
		{"lowercase month", "5 jul 2026", "D MMM YYYY", time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), false},
		{"two digit year", "05.07.26", "DD.MM.YY", time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), false},
		{"with time", "05/07/2026 09:04", "DD/MM/YYYY HH:mm", time.Date(2026, 7, 5, 9, 4, 0, 0, time.UTC), false},
		{"surrounding space", "  05/07/2026  ", "DD/MM/YYYY", time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), false},
		{"wrong separator", "05-07-2026", "DD/MM/YYYY", time.Time{}, true},
		{"trailing junk", "05/07/2026x", "DD/MM/YYYY", time.Time{}, true},
		{"impossible day", "31/02/2026", "DD/MM/YYYY", time.Time{}, true},
		{"month 13", "13/13/2026", "DD/MM/YYYY", time.Time{}, true},
		{"empty", "", "DD/MM/YYYY", time.Time{}, true},
		{"auto pattern", "05/07/2026", dateformat.Auto, time.Time{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := dateformat.Parse(tc.in, tc.pattern, time.UTC)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q, %q) = %v, want error", tc.in, tc.pattern, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q, %q): %v", tc.in, tc.pattern, err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("Parse(%q, %q) = %v, want %v", tc.in, tc.pattern, got, tc.want)
			}
		})
	}
}

func TestParseUsesLocation(t *testing.T) {
	t.Parallel()
	loc, err := time.LoadLocation("Europe/Athens")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	got, err := dateformat.Parse("05/07/2026 09:04", "DD/MM/YYYY HH:mm", loc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Athens is UTC+3 in July.
	if want := time.Date(2026, 7, 5, 6, 4, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("Parse in Athens = %v (UTC %v), want UTC %v", got, got.UTC(), want)
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()
	if got := dateformat.Normalize("DD/MM/YYYY"); got != "DD/MM/YYYY" {
		t.Fatalf("Normalize kept = %q", got)
	}
	if got := dateformat.Normalize("garbage"); got != dateformat.Auto {
		t.Fatalf("Normalize(garbage) = %q, want Auto", got)
	}
}

func TestWithTime(t *testing.T) {
	t.Parallel()
	if got := dateformat.WithTime("DD/MM/YYYY"); got != "DD/MM/YYYY HH:mm" {
		t.Fatalf("WithTime = %q", got)
	}
	if got := dateformat.WithTime(dateformat.Auto); got != dateformat.Auto {
		t.Fatalf("WithTime(Auto) = %q, want Auto", got)
	}
}

func TestPlaceholder(t *testing.T) {
	t.Parallel()
	if got := dateformat.Placeholder("DD/MM/YYYY HH:mm"); got != "dd/mm/yyyy hh:mm" {
		t.Fatalf("Placeholder = %q", got)
	}
	if got := dateformat.Placeholder("D MMM YYYY"); got != "d MMM yyyy" {
		t.Fatalf("Placeholder with month name = %q", got)
	}
	if got := dateformat.Placeholder(dateformat.Auto); got != "" {
		t.Fatalf("Placeholder(Auto) = %q, want empty", got)
	}
}

func TestPresetsAreValid(t *testing.T) {
	t.Parallel()
	for _, p := range dateformat.Presets {
		if err := dateformat.Validate(p); err != nil {
			t.Fatalf("preset %q invalid: %v", p, err)
		}
		if err := dateformat.Validate(dateformat.WithTime(p)); err != nil {
			t.Fatalf("preset %q invalid with time: %v", p, err)
		}
	}
}
