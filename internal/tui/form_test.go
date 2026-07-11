package tui

import (
	"testing"
	"time"
)

func TestNextStatus(t *testing.T) {
	cases := map[string]string{
		"pending":   "completed",
		"completed": "all",
		"all":       "pending",
		"":          "pending",
	}
	for in, want := range cases {
		if got := nextStatus(in); got != want {
			t.Errorf("nextStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNextPeriod(t *testing.T) {
	cases := map[string]string{
		"all":   "today",
		"today": "week",
		"week":  "month",
		"month": "all",
		"":      "all",
	}
	for in, want := range cases {
		if got := nextPeriod(in); got != want {
			t.Errorf("nextPeriod(%q) = %q, want %q", in, got, want)
		}
	}
	// Full cycle returns to start.
	p := "all"
	for range periodCycle {
		p = nextPeriod(p)
	}
	if p != "all" {
		t.Errorf("cycle did not return to all, got %q", p)
	}
}

func TestNextPriority(t *testing.T) {
	cases := map[string]string{
		"low":    "normal",
		"normal": "high",
		"high":   "low",
		"":       "low",
	}
	for in, want := range cases {
		if got := nextPriority(in); got != want {
			t.Errorf("nextPriority(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseHumanTime(t *testing.T) {
	loc := time.UTC
	if _, err := parseHumanTime("now", loc); err != nil {
		t.Errorf("now: %v", err)
	}
	if got, err := parseHumanTime("now+2h", loc); err != nil {
		t.Errorf("now+2h: %v", err)
	} else if d := time.Until(got); d < 90*time.Minute || d > 150*time.Minute {
		t.Errorf("now+2h out of range: %v", d)
	}
	got, err := parseHumanTime("2026-07-11 09:00", loc)
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	if got.Hour() != 9 || got.Year() != 2026 || got.Month() != time.July || got.Day() != 11 {
		t.Errorf("unexpected parse: %v", got)
	}
	if _, err := parseHumanTime("not a time", loc); err == nil {
		t.Error("expected error for garbage input")
	}
	if _, err := parseHumanTime("", loc); err == nil {
		t.Error("expected error for empty input")
	}
}

func TestTaskFormValidate(t *testing.T) {
	f := newTaskForm()
	if _, _, _, _, err := f.validate(); err == nil {
		t.Error("expected error with empty title")
	}
	f.title.setValue("Buy milk")
	if _, _, _, _, err := f.validate(); err == nil {
		t.Error("expected error with empty due")
	}
	f.due.setValue("2026-07-11 09:00")
	f.desc.setValue("2%")
	title, due, prio, desc, err := f.validate()
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if title != "Buy milk" || prio != "normal" || desc != "2%" {
		t.Errorf("unexpected fields: %q %q %q", title, prio, desc)
	}
	if _, perr := time.Parse(time.RFC3339, due); perr != nil {
		t.Errorf("due not RFC3339: %q (%v)", due, perr)
	}
}

func TestTextFieldEditing(t *testing.T) {
	var tf textField
	for _, r := range "abc" {
		tf.insert(r)
	}
	if tf.String() != "abc" {
		t.Fatalf("insert: %q", tf.String())
	}
	tf.left()
	tf.insert('X') // ab X c -> abXc
	if tf.String() != "abXc" {
		t.Fatalf("mid-insert: %q", tf.String())
	}
	tf.backspace()
	if tf.String() != "abc" {
		t.Fatalf("backspace: %q", tf.String())
	}
}
