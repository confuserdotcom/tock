package tui

import (
	"strings"
	"testing"
)

func actNames(acts []action) string {
	var b []string
	for _, a := range acts {
		b = append(b, a.key+" "+a.name)
	}
	return strings.Join(b, "|")
}

func TestMatch(t *testing.T) {
	if match("", "x") != 0 || match("zz", "tag") != -1 {
		t.Fatal("empty or missing")
	}
	if !(match("tag", "tag") > match("tag", "tag and learn a rule")) {
		t.Fatal("shorter prefix should win")
	}
	if !(match("week", "week") > match("wk", "week")) || match("wk", "week") < 0 {
		t.Fatal("subsequence")
	}
}

func TestActionsFollowTheRow(t *testing.T) {
	m, _ := seeded(t)
	// The cursor starts on the open Mail row: unlabelled.
	got := actNames(m.available())
	for _, want := range []string{"enter label it", "t tag", "p set project", "T tag and learn a rule", "a label time away"} {
		if !strings.Contains(got, want) {
			t.Errorf("unlabelled row lacks %q: %s", want, got)
		}
	}
	// The idle row 11:41–12:22 is pending time away.
	m = press(t, m, "k", "k")
	got = actNames(m.available())
	if !strings.Contains(got, "t label away") || strings.Contains(got, "p set project") || !strings.Contains(got, "enter label it") {
		t.Errorf("idle row: %s", got)
	}
	// A labelled row: no enter.
	m = press(t, m, "g")
	if got := actNames(m.available()); strings.Contains(got, "enter") || !strings.Contains(got, "T tag and learn") {
		t.Errorf("labelled row: %s", got)
	}
}

func TestFooterFits(t *testing.T) {
	m, _ := seeded(t)
	for _, w := range []int{60, 100, 140} {
		acts := m.footerActions(w)
		h := hintOf(acts)
		if len([]rune(h)) > w || !strings.HasSuffix(h, "? help · : commands") {
			t.Errorf("width %d: %q", w, h)
		}
	}
	if !strings.HasPrefix(hintOf(m.footerActions(140)), "enter label it · a label time away") {
		t.Errorf("order: %q", hintOf(m.footerActions(140)))
	}
}

func TestPaletteRuns(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, ":")
	mustContain(t, m, "─ Commands ─", "label time away")
	m = typeText(t, m, "week")
	m = press(t, m, "enter")
	if m.palette != nil || m.tab != tabWeek {
		t.Fatalf("palette %v tab %d", m.palette, m.tab)
	}
	m = press(t, m, "ctrl+k")
	m = typeText(t, m, "50 min")
	m = press(t, m, "enter")
	mustContain(t, m, "session started: 50 min")
	if m.tab != tabFocus {
		t.Fatalf("tab %d, want focus", m.tab)
	}
	m = press(t, m, ":")
	m = typeText(t, m, "qqqzzz")
	mustContain(t, m, "no command matches")
	m = press(t, m, "esc")
	if m.palette != nil {
		t.Fatal("esc must close")
	}
}

func TestEnterLabelsRow(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "enter") // the unlabelled Mail row
	if m.picker == nil || m.picker.action != pickTag {
		t.Fatalf("enter on an unlabelled row: picker %+v", m.picker)
	}
	m = press(t, m, "esc", "k", "k", "enter") // the idle row in pending time away
	mustContain(t, m, "Category  away 11:41–12:22 (41m)")
}
