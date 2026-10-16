package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func rightClickOn(t *testing.T, m Model, s string) Model {
	t.Helper()
	for y, l := range strings.Split(screen(m), "\n") {
		if i := strings.Index(l, s); i >= 0 {
			return step(t, m, tea.MouseClickMsg{X: ansi.StringWidth(l[:i]), Y: y, Button: tea.MouseRight})
		}
	}
	t.Fatalf("no %q on screen:\n%s", s, screen(m))
	return m
}

func hoverOn(t *testing.T, m Model, s string) Model {
	t.Helper()
	for y, l := range strings.Split(screen(m), "\n") {
		if i := strings.Index(l, s); i >= 0 {
			return step(t, m, tea.MouseMotionMsg{X: ansi.StringWidth(l[:i]), Y: y})
		}
	}
	t.Fatalf("no %q on screen:\n%s", s, screen(m))
	return m
}

func TestClickKeycapAnywhere(t *testing.T) {
	m, _ := seeded(t)
	// The attention line's "A leave idle" is a button.
	m = clickOn(t, m, "A leave idle")
	mustNotContain(t, m, "⚑ Away 12:25–14:50")
}

func TestClickScoreOpensBreakdown(t *testing.T) {
	m, _ := seeded(t)
	m = clickOn(t, m, "score  good day")
	if !m.score {
		t.Fatal("score breakdown not open")
	}
}

func TestClickCategoryFilters(t *testing.T) {
	m, _ := seeded(t)
	m = step(t, m, windowSize(150, 36))
	m = clickOn(t, m, "● Entertainment")
	if m.today.filter != "Entertainment" {
		t.Fatalf("filter = %q", m.today.filter)
	}
	mustNotContain(t, m, "ATLAS │ y")
	m = clickOn(t, m, "▸ Entertainment")
	if m.today.filter != "" {
		t.Fatalf("second click keeps filter %q", m.today.filter)
	}
}

func TestClickWorkBlockJumps(t *testing.T) {
	m, _ := seeded(t)
	m = step(t, m, windowSize(150, 36))
	m = clickOn(t, m, "09:00 tock")
	if r := m.today.selected(); r == nil || !strings.Contains(r.Text, "tock") {
		t.Fatalf("selected %+v", r)
	}
}

func TestHoverShowsTooltip(t *testing.T) {
	m, _ := seeded(t)
	m = step(t, m, windowSize(150, 36))
	m = hoverOn(t, m, "● Code ")
	mustContain(t, m, "◆ show only Code rows")
	m = step(t, m, tea.MouseMotionMsg{X: 0, Y: 34})
	mustNotContain(t, m, "◆ show only")
}

func TestRightClickMenu(t *testing.T) {
	m, _ := seeded(t)
	m = rightClickOn(t, m, "XOR Equivalence Check")
	if m.menu == nil {
		t.Fatal("no menu")
	}
	if r := m.today.selected(); r == nil || r.Text != "XOR Equivalence Check" {
		t.Fatalf("right click did not select the row: %+v", r)
	}
	mustContain(t, m, "tag and learn a rule")
	m = clickOn(t, m, "set project")
	if m.menu != nil || m.picker == nil {
		t.Fatalf("menu %v picker %v", m.menu, m.picker)
	}
	m = press(t, m, "esc")
	m = rightClickOn(t, m, "Low Level Labs")
	m = press(t, m, "esc")
	if m.menu != nil {
		t.Fatal("esc keeps the menu")
	}
}

func TestClickRowTwiceIsEnter(t *testing.T) {
	m, _ := seeded(t)
	m = clickOn(t, m, "XOR Equivalence Check")
	if m.picker != nil {
		t.Fatal("first click opened the picker")
	}
	m = clickOn(t, m, "XOR Equivalence Check")
	if m.picker == nil && m.learn == nil {
		t.Fatal("second click did nothing")
	}
}
