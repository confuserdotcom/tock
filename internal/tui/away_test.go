package tui

import (
	"strings"
	"testing"

	"tock/internal/apptest"
)

func TestAwayPromptAndAnswer(t *testing.T) {
	m, _ := seeded(t)
	mustContain(t, m, "⚑ Away 12:25–14:50 (2h25) — what was it?   a label   A leave idle   +1 more", "a label time away")
	m = press(t, m, "a")
	mustContain(t, m, "─ Category ─", "away 12:25–14:50 (2h25)")
	m = typeText(t, m, "comm")
	m = press(t, m, "enter", "enter")
	mustContain(t, m, "labelled away 12:25–14:50 (2h25) → Communication")
	mustContain(t, m, "⚑ Away 11:41–12:22 (41m) — what was it?")
	mustNotContain(t, m, "+1 more")
	m = press(t, m, "A")
	mustContain(t, m, "left 11:41–12:22 (41m) idle")
	mustNotContain(t, m, "a what was it?")
	if l := line(t, m, "12:25–14:50"); !strings.Contains(l, "Communication") || !strings.Contains(l, "Away") {
		t.Fatalf("away row: %q", l)
	}
	m = press(t, m, "a")
	mustContain(t, m, "no time away to label")
}

func TestAwayFromIdleRowAndRelabel(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "k", "k", "t") // the idle row 11:41–12:22
	mustContain(t, m, "─ Category ─", "away 11:41–12:22 (41m)")
	m = typeText(t, m, "stu")
	m = press(t, m, "enter")
	m = typeText(t, m, "atl")
	m = press(t, m, "enter")
	mustContain(t, m, "labelled away 11:41–12:22 (41m) → Study / ATLAS")
	l := line(t, m, "11:41–12:22")
	if !strings.Contains(l, "Study / ATLAS") {
		t.Fatalf("row: %q", l)
	}
	// The away row: no rules, but t relabels it.
	for i := 0; i < 5 && !strings.Contains(line(t, m, "11:41–12:22"), "▸"); i++ {
		m = press(t, m, "k")
	}
	m = press(t, m, "T")
	mustContain(t, m, "away time has no rules: t to relabel it")
	m = press(t, m, "t")
	m = typeText(t, m, "comm")
	m = press(t, m, "enter", "enter")
	if l := line(t, m, "11:41–12:22"); !strings.Contains(l, "Communication") {
		t.Fatalf("relabelled row: %q", l)
	}
}

func TestAwayNoneOnQuietDay(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "x"})
	m := newModel(t, e)
	mustNotContain(t, m, "a what was it?", "a away")
}

func TestAwayPickerOffersRecentLabels(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "a") // 12:25–14:50
	m = typeText(t, m, "comm")
	m = press(t, m, "enter", "enter")
	m = press(t, m, "a") // 11:41–12:22: the last answer comes first
	mustContain(t, m, "Communication", "↺ recent")
	m = press(t, m, "enter")
	mustContain(t, m, "labelled away 11:41–12:22 (41m) → Communication")
}

func TestAwayStillOnIt(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ x"},
		apptest.Span{ID: 2, Start: "10:00", End: "10:30", Bundle: "tock.idle", App: "Idle"},
		apptest.Span{ID: 3, Start: "10:30", End: "11:00", Bundle: "term", App: "WezTerm", Title: "tock │ y"})
	m := newModel(t, e)
	mustContain(t, m, "⚑ Away 10:00–10:30 (30m) after Code / tock — still on it?   y yes   a something else   A leave idle")
	m = press(t, m, "a")
	mustContain(t, m, "Code / tock              ◷ before")
	m = press(t, m, "esc", "y")
	mustContain(t, m, "labelled away 10:00–10:30 (30m) → Code / tock")
	mustNotContain(t, m, "⚑ Away")
	if l := line(t, m, "10:00–10:30"); !strings.Contains(l, "Code / tock") {
		t.Fatalf("away row: %q", l)
	}
}
