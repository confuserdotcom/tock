package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCard(t *testing.T) {
	got := card("Day", []string{"a", "a much longer line than fits"}, 16)
	want := []string{
		"╭─ Day ────────╮",
		"│ a            │",
		"│ a much long… │",
		"╰──────────────╯",
	}
	for i := range want {
		if s := ansi.Strip(got[i]); s != want[i] {
			t.Errorf("line %d = %q, want %q", i, s, want[i])
		}
	}
	if s := ansi.Strip(card("", nil, 6)[0]); s != "╭────╮" {
		t.Errorf("untitled top = %q", s)
	}
}

func TestSideBySideAndMeter(t *testing.T) {
	got := sideBySide([]string{"ab"}, []string{"x", "y"}, 4, 2)
	if len(got) != 2 || got[0] != "ab    x" || got[1] != "      y" {
		t.Fatalf("sideBySide = %q", got)
	}
	for _, c := range []struct {
		v, total int64
		want     string
	}{{0, 10, "─────"}, {10, 10, "━━━━━"}, {1, 100, "━────"}, {5, 10, "━━━──"}, {20, 10, "━━━━━"}, {3, 0, "━────"}} {
		if s := ansi.Strip(meter(c.v, c.total, 5, sBrand)); s != c.want {
			t.Errorf("meter(%d/%d) = %q, want %q", c.v, c.total, s, c.want)
		}
	}
}

func TestScoreBand(t *testing.T) {
	for _, c := range []struct {
		score  int
		active bool
		want   string
	}{{90, false, "not started"}, {80, true, "great day"}, {79, true, "good day"}, {60, true, "good day"},
		{59, true, "steady"}, {40, true, "steady"}, {39, true, "scattered"}, {0, true, "scattered"}} {
		if _, w := scoreBand(c.score, c.active); w != c.want {
			t.Errorf("scoreBand(%d, %v) = %q, want %q", c.score, c.active, w, c.want)
		}
	}
}

func TestBigDigits(t *testing.T) {
	rows := bigDigits("12:05")
	for i, r := range rows {
		if w := ansi.StringWidth(r); w != 4*3+1+4 {
			t.Errorf("row %d width %d: %q", i, w, r)
		}
	}
	if !strings.HasPrefix(rows[0], "▀█ ") {
		t.Errorf("row 0 = %q", rows[0])
	}
}

func TestThemeSwitch(t *testing.T) {
	defer setTheme(true)
	dark := sDeep.GetForeground()
	setTheme(false)
	if sDeep.GetForeground() == dark || themeDark {
		t.Fatal("light theme kept the dark colours")
	}
}
