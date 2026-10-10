package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func clickAt(t *testing.T, m Model, x, y int) Model {
	t.Helper()
	return step(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// clickOn clicks the first cell of the first screen occurrence of s, so a
// test only passes when the click lands on what the screen shows there.
func clickOn(t *testing.T, m Model, s string) Model {
	t.Helper()
	for y, l := range strings.Split(screen(m), "\n") {
		if i := strings.Index(l, s); i >= 0 {
			return clickAt(t, m, ansi.StringWidth(l[:i]), y)
		}
	}
	t.Fatalf("no %q on screen:\n%s", s, screen(m))
	return m
}

func wheel(t *testing.T, m Model, b tea.MouseButton, n int) Model {
	t.Helper()
	for range n {
		m = step(t, m, tea.MouseWheelMsg{Button: b})
	}
	return m
}

func TestMouseModeOn(t *testing.T) {
	m, _ := seeded(t)
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("MouseMode = %v", got)
	}
}

func TestMouseHeaderSwitchesTabs(t *testing.T) {
	m, _ := seeded(t)
	m = clickOn(t, m, "inbox ³")
	if m.tab != tabInbox {
		t.Fatalf("tab = %d", m.tab)
	}
	m = clickOn(t, m, "rules")
	mustContain(t, m, "rules.toml · 3 rules")
	m = clickOn(t, m, "tock") // the name, not a tab
	if m.tab != tabRules {
		t.Fatalf("click on the name switched to %d", m.tab)
	}
	m = press(t, m, "?")
	m = clickOn(t, m, "today ")
	if m.tab != tabRules {
		t.Fatal("header clicks must wait until the overlay closes")
	}
}

func TestMouseFooterButtons(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "4")
	m = clickOn(t, m, "s start a session")
	mustContain(t, m, "Start a focus session")
	m = clickOn(t, m, "esc cancel")
	mustNotContain(t, m, "Start a focus session")

	m = press(t, m, "1")
	m = clickOn(t, m, "t tag")
	if m.picker == nil {
		t.Fatal("t tag must open the picker")
	}
	m = clickOn(t, m, "esc cancel")
	m = clickOn(t, m, "? help")
	if !m.help {
		t.Fatal("? help must open the help")
	}
}

func TestFooterKey(t *testing.T) {
	hint := "j/k move · enter open · ? help"
	for _, c := range []struct {
		x    int
		want string
	}{{0, ""}, {7, ""}, {8, ""}, {11, "enter"}, {20, "enter"}, {21, ""}, {23, ""}, {24, "?"}, {29, "?"}, {30, ""}} {
		got, ok := footerKey(hint, c.x)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("x=%d: %q %v, want %q", c.x, got, ok, c.want)
		}
	}
}

func TestButtonAt(t *testing.T) {
	l := "│ [y] accept   [s] more specific   [n] cancel    │"
	for _, c := range []struct {
		x    int
		want string
	}{{0, ""}, {2, "y"}, {11, "y"}, {12, ""}, {15, "s"}, {31, "s"}, {35, "n"}, {44, "n"}, {45, ""}, {48, ""}} {
		got, ok := buttonAt(l, c.x)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("x=%d: %q %v, want %q", c.x, got, ok, c.want)
		}
	}
}

func TestMouseWheelMovesCursor(t *testing.T) {
	m, _ := seeded(t)
	last := m.today.cursor
	m = wheel(t, m, tea.MouseWheelUp, 2)
	if m.today.cursor != last-2 {
		t.Fatalf("cursor = %d, want %d", m.today.cursor, last-2)
	}
	m = wheel(t, m, tea.MouseWheelDown, 1)
	if m.today.cursor != last-1 {
		t.Fatalf("cursor = %d, want %d", m.today.cursor, last-1)
	}
	m = press(t, m, "/")
	m = wheel(t, m, tea.MouseWheelUp, 1)
	if m.today.filter != "" || !m.today.filtering {
		t.Fatal("wheel must not touch the filter")
	}
}

func TestMouseTodaySelectsRow(t *testing.T) {
	m, _ := seeded(t)
	m = clickOn(t, m, "11:26–11:35")
	if l := line(t, m, "11:26–11:35"); !strings.Contains(l, "▸") {
		t.Fatalf("row not selected: %q", l)
	}
	if m.picker != nil {
		t.Fatal("a click on today only selects")
	}
	before := m.today.cursor
	m = clickOn(t, m, "Today") // the title is not a row
	if m.today.cursor != before {
		t.Fatal("title click moved the cursor")
	}
}

func TestMouseInboxSelectThenLearn(t *testing.T) {
	m, e := seeded(t)
	m = press(t, m, "3")
	m = clickOn(t, m, "Mail · inbox")
	if l := line(t, m, "Mail · inbox"); !strings.HasPrefix(l, "▸") {
		t.Fatalf("not selected: %q", l)
	}
	m = clickOn(t, m, "Mail · inbox")
	mustContain(t, m, "Category")
	m = clickOn(t, m, "Communication")
	if !strings.Contains(line(t, m, "Communication"), "▸") {
		t.Fatal("option not selected")
	}
	m = clickOn(t, m, "Communication")
	mustContain(t, m, "Project for Communication")
	m = clickOn(t, m, noProject) // already selected: picks it
	mustContain(t, m, "Learn a rule")
	m = clickOn(t, m, "[n] cancel")
	mustContain(t, m, "cancelled")
	if _, err := os.Stat(e.Learned); !os.IsNotExist(err) {
		t.Fatal("cancel must not write learned.toml")
	}
}

func TestMouseWeekOpensDay(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "2")
	m = clickOn(t, m, "Sat 10   2h54")
	if l := line(t, m, "Sat 10   2h54"); !strings.HasPrefix(l, "▸") {
		t.Fatalf("not selected: %q", l)
	}
	m = clickOn(t, m, "Sat 10   2h54")
	mustContain(t, m, "Today  ‹ h  l ›")
}

func TestMouseRulesSelects(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "5")
	m = clickOn(t, m, "Entertainment")
	if l := line(t, m, "Entertainment"); !strings.Contains(l, "▸") {
		t.Fatalf("not selected: %q", l)
	}
	m = clickOn(t, m, "rules.toml · 3 rules")
	if l := line(t, m, "Entertainment"); !strings.Contains(l, "▸") {
		t.Fatal("section title click moved the cursor")
	}
}

func TestMouseStartDialog(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "4", "s")
	m = clickOn(t, m, "50 min")
	if m.start == nil || m.start.cursor != 1 {
		t.Fatal("50 min not selected")
	}
	m = clickOn(t, m, "50 min")
	mustContain(t, m, "session started: 50 min")
}

func TestMouseIgnoredWhenTooSmall(t *testing.T) {
	m, _ := seeded(t)
	m = step(t, m, windowSize(40, 10))
	m = clickAt(t, m, 12, 0)
	m = wheel(t, m, tea.MouseWheelUp, 1)
	if m.tab != tabToday {
		t.Fatal("click while too small")
	}
}

func TestMouseScrolledTimeline(t *testing.T) {
	m, _ := seeded(t)
	m = step(t, m, windowSize(MinWidth, MinHeight))
	if m.today.top == 0 {
		t.Fatal("the timeline must be scrolled for this test")
	}
	want := m.today.top // the first visible row
	body := m.body(m.bodyH())
	for line := range body {
		if i, ok := m.today.rowAt(line, m.width, m.bodyH()); ok {
			if i != want {
				t.Fatalf("first mapped row = %d, want %d", i, want)
			}
			m = clickAt(t, m, 5, line+m.bodyTop())
			if m.today.cursor != want {
				t.Fatalf("cursor = %d, want %d", m.today.cursor, want)
			}
			if !strings.Contains(ansi.Strip(m.body(m.bodyH())[line]), "▸") {
				t.Fatalf("clicked line not selected: %q", ansi.Strip(m.body(m.bodyH())[line]))
			}
			return
		}
	}
	t.Fatal("no timeline row mapped")
}
