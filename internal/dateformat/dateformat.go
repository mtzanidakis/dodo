// Package dateformat renders and parses dates using a small user-facing token
// pattern (DD/MM/YYYY and friends) instead of a Go layout string.
//
// Go layouts are unusable as a user setting: the reference-time components are
// ordinary text, so a literal in the user's pattern can silently become a
// format verb. The tokens here are matched explicitly and anything that is not
// a token is rejected at validation time rather than reinterpreted.
package dateformat

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Auto is the zero value of a user's date_format: keep the built-in
// locale-ish rendering each surface already used before this setting existed.
const Auto = ""

// TimePattern is appended to a date pattern to make a datetime pattern. The
// setting itself is date-only; times stay 24h everywhere.
const TimePattern = "HH:mm"

// Presets are the patterns offered in the UI, in display order. Auto is
// presented alongside these but is not itself a pattern.
var Presets = []string{
	"DD/MM/YYYY",
	"MM/DD/YYYY",
	"YYYY-MM-DD",
	"D MMM YYYY",
	"MMM D, YYYY",
}

// maxPatternLen bounds what we will store and compile into a regexp.
const maxPatternLen = 32

var (
	ErrEmpty       = errors.New("empty pattern")
	ErrTooLong     = fmt.Errorf("pattern longer than %d characters", maxPatternLen)
	ErrStrayLetter = errors.New("pattern contains letters that are not tokens")
	ErrNoYear      = errors.New("pattern must contain a year token")
	ErrNoMonth     = errors.New("pattern must contain a month token")
	ErrNoDay       = errors.New("pattern must contain a day token")
	ErrDuplicate   = errors.New("pattern repeats a field")
	ErrMismatch    = errors.New("input does not match the date format")
)

// tokenRE is longest-match-first so MMMM beats MMM and YYYY beats YY.
var tokenRE = regexp.MustCompile(`YYYY|YY|MMMM|MMM|MM|M|DD|D|HH|mm`)

type field int

const (
	fieldYear field = iota
	fieldMonth
	fieldDay
	fieldHour
	fieldMinute
)

func fieldOf(token string) field {
	switch token {
	case "YYYY", "YY":
		return fieldYear
	case "MMMM", "MMM", "MM", "M":
		return fieldMonth
	case "DD", "D":
		return fieldDay
	case "HH":
		return fieldHour
	default:
		return fieldMinute
	}
}

// Validate reports whether pattern is usable as a user's date format. Auto is
// valid and means "no pattern".
func Validate(pattern string) error {
	if pattern == Auto {
		return nil
	}
	if strings.TrimSpace(pattern) == "" {
		return ErrEmpty
	}
	if len(pattern) > maxPatternLen {
		return ErrTooLong
	}

	seen := map[field]bool{}
	for _, tok := range tokenRE.FindAllString(pattern, -1) {
		f := fieldOf(tok)
		if seen[f] {
			return ErrDuplicate
		}
		seen[f] = true
	}
	// Everything the tokenizer did not consume must be punctuation or spaces.
	// Otherwise a literal "May" would be read as M + a + y and reassembled into
	// something the user never wrote.
	if hasLetter(tokenRE.ReplaceAllString(pattern, "")) {
		return ErrStrayLetter
	}
	switch {
	case !seen[fieldYear]:
		return ErrNoYear
	case !seen[fieldMonth]:
		return ErrNoMonth
	case !seen[fieldDay]:
		return ErrNoDay
	}
	return nil
}

func hasLetter(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// Normalize returns pattern when it is valid and Auto when it is not, so a bad
// value stored by an older version degrades to the built-in rendering instead
// of breaking every date on the page.
func Normalize(pattern string) string {
	if Validate(pattern) != nil {
		return Auto
	}
	return pattern
}

// WithTime returns the datetime pattern for a date pattern: the user's date
// format followed by a 24h clock. Auto stays Auto.
func WithTime(pattern string) string {
	if pattern == Auto {
		return Auto
	}
	return pattern + " " + TimePattern
}

// goLayout maps a token to the equivalent Go reference-time component.
func goLayout(token string) string {
	switch token {
	case "YYYY":
		return "2006"
	case "YY":
		return "06"
	case "MMMM":
		return "January"
	case "MMM":
		return "Jan"
	case "MM":
		return "01"
	case "M":
		return "1"
	case "DD":
		return "02"
	case "D":
		return "2"
	case "HH":
		return "15"
	default:
		return "04"
	}
}

// Render formats t with pattern. An empty or invalid pattern returns "" so
// callers can fall back to their own layout.
func Render(t time.Time, pattern string) string {
	if Validate(pattern) != nil {
		return ""
	}
	return tokenRE.ReplaceAllStringFunc(pattern, func(tok string) string {
		return t.Format(goLayout(tok))
	})
}

// Parse reads s as a date written in pattern, in loc. Leading and trailing
// space is ignored; the rest must match exactly.
func Parse(s, pattern string, loc *time.Location) (time.Time, error) {
	if err := Validate(pattern); err != nil {
		return time.Time{}, err
	}
	if pattern == Auto {
		return time.Time{}, ErrEmpty
	}
	if loc == nil {
		loc = time.UTC
	}

	re, tokens, err := compile(pattern)
	if err != nil {
		return time.Time{}, err
	}
	m := re.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, ErrMismatch
	}

	year, month, day, hour, minute := 0, 1, 1, 0, 0
	haveYear := false
	for i, tok := range tokens {
		v := m[i+1]
		switch tok {
		case "YYYY":
			year, _ = strconv.Atoi(v)
			haveYear = true
		case "YY":
			n, _ := strconv.Atoi(v)
			year = 2000 + n
			haveYear = true
		case "MMMM", "MMM":
			month = monthByName(v)
			if month == 0 {
				return time.Time{}, ErrMismatch
			}
		case "MM", "M":
			month, _ = strconv.Atoi(v)
		case "DD", "D":
			day, _ = strconv.Atoi(v)
		case "HH":
			hour, _ = strconv.Atoi(v)
		case "mm":
			minute, _ = strconv.Atoi(v)
		}
	}
	if !haveYear {
		return time.Time{}, ErrNoYear
	}

	out := time.Date(year, time.Month(month), day, hour, minute, 0, 0, loc)
	// time.Date normalises out-of-range values (31 February becomes 3 March).
	// The user typed a date that does not exist, so reject it rather than
	// silently storing a different day.
	if out.Year() != year || int(out.Month()) != month || out.Day() != day {
		return time.Time{}, ErrMismatch
	}
	return out, nil
}

// compile turns a pattern into an anchored regexp plus the tokens matching its
// capture groups, in order.
func compile(pattern string) (*regexp.Regexp, []string, error) {
	var (
		b       strings.Builder
		tokens  []string
		lastIdx int
	)
	// Case-insensitive so a user typing "12 jan 2026" is not rejected over the
	// capital letter; every other group is digits, which are unaffected.
	b.WriteString(`(?i)^`)
	for _, m := range tokenRE.FindAllStringIndex(pattern, -1) {
		if m[0] > lastIdx {
			b.WriteString(regexp.QuoteMeta(pattern[lastIdx:m[0]]))
		}
		tok := pattern[m[0]:m[1]]
		tokens = append(tokens, tok)
		b.WriteString(group(tok))
		lastIdx = m[1]
	}
	if lastIdx < len(pattern) {
		b.WriteString(regexp.QuoteMeta(pattern[lastIdx:]))
	}
	b.WriteString(`$`)

	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, nil, err
	}
	return re, tokens, nil
}

func group(token string) string {
	switch token {
	case "YYYY":
		return `(\d{4})`
	case "YY", "MM", "DD", "HH", "mm":
		return `(\d{2})`
	case "M", "D":
		return `(\d{1,2})`
	case "MMMM":
		return `(` + strings.Join(monthNames(false), "|") + `)`
	default:
		return `(` + strings.Join(monthNames(true), "|") + `)`
	}
}

func monthNames(short bool) []string {
	layout := "January"
	if short {
		layout = "Jan"
	}
	out := make([]string, 0, 12)
	for m := time.January; m <= time.December; m++ {
		out = append(out, time.Date(2000, m, 1, 0, 0, 0, 0, time.UTC).Format(layout))
	}
	return out
}

func monthByName(name string) int {
	for m := time.January; m <= time.December; m++ {
		ref := time.Date(2000, m, 1, 0, 0, 0, 0, time.UTC)
		if strings.EqualFold(name, ref.Format("January")) || strings.EqualFold(name, ref.Format("Jan")) {
			return int(m)
		}
	}
	return 0
}

// Placeholder renders a human-readable hint for an input expecting pattern,
// e.g. "dd/mm/yyyy hh:mm". Auto returns "".
func Placeholder(pattern string) string {
	if Validate(pattern) != nil || pattern == Auto {
		return ""
	}
	return tokenRE.ReplaceAllStringFunc(pattern, func(tok string) string {
		switch tok {
		case "MMMM", "MMM":
			return tok
		default:
			return strings.ToLower(tok)
		}
	})
}
