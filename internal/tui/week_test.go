package tui

import (
	"strings"
	"testing"

	"tock/internal/apptest"
)

func TestWeekTab(t *testing.T) {
	m, e := seeded(t)
	e.Seed(t, apptest.Span{ID: 9, Start: "09:00", End: "11:00", Bundle: "term", App: "WezTerm", Title: "tock │ y", Day: -5})
	m = press(t, m, "2")
	mustContain(t, m, "Week of Mon 5 Oct  tracked 4h54 · avg score 75 ▇····▆ · deep 4h14",
		"Mon  5   2h00   75   2h00  ███", "Tue  6     0s    —      —", "Sat 10   2h54   74   2h14  ███",
		"Sun 11      —    —      —",
		"Code             4h14", "Projects         tock 3h40",
		"Apps             WezTerm 4h14 · chatgpt.com 12m · youtube.com 12m · Mail 10m · labs.x.tv 6m")
	if l := line(t, m, "Mon  5"); !strings.HasPrefix(l, "▶") {
		t.Fatalf("cursor starts on Monday: %q", l)
	}
	m = press(t, m, "enter")
	mustContain(t, m, "Mon 5 Oct  score", " tracked 2h00")
	m = press(t, m, "2", "j", "j", "j", "j", "j", "enter")
	mustContain(t, m, "Today  score 74", " tracked 2h54")
	m = press(t, m, "2", "j", "enter") // Sunday is in the future
	mustContain(t, m, "Week of Mon 5 Oct")
}

func TestWeekNavigation(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "2", "l")
	mustContain(t, m, "Week of Mon 5 Oct") // no future weeks
	m = press(t, m, "h")
	mustContain(t, m, "Week of Mon 28 Sep  tracked 0s · avg score — ·······")
	m = press(t, m, "l")
	mustContain(t, m, "Week of Mon 5 Oct")
	m = press(t, m, "h", "h", ".")
	mustContain(t, m, "Week of Mon 5 Oct")
}

func TestSparkCells(t *testing.T) {
	for _, tt := range []struct {
		score int
		want  rune
	}{{0, '▁'}, {12, '▁'}, {13, '▂'}, {50, '▅'}, {87, '▇'}, {88, '█'}, {100, '█'}} {
		if got := spark(tt.score); got != tt.want {
			t.Errorf("score %d: %c, want %c", tt.score, got, tt.want)
		}
	}
}
