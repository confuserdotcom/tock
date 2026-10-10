package tui

import (
	"testing"

	"tock/internal/apptest"
)

const goalRules = apptest.Rules + `
[[goal]]
category = "code"
at_least = "2h"

[[goal]]
project = "tock"
at_least = "10h"
per = "week"

[[goal]]
category = "entertainment"
at_most = "10m"
`

func TestTodayGoalsLine(t *testing.T) {
	e := apptest.New(t, goalRules)
	e.Seed(t, apptest.DaySpans()...)
	m := newModel(t, e)
	mustContain(t, m, " goals  Code 2h14/2h00 · project tock 1h40/10h00 wk · Entertainment 12m/10m over")
	// The timeline still maps clicks to rows below the extra line.
	m = clickOn(t, m, "11:26–11:35")
	if l := line(t, m, "11:26–11:35"); l[0:3] != "▶" {
		t.Fatalf("row not selected: %q", l)
	}
}

func TestWeekGoalsBlock(t *testing.T) {
	e := apptest.New(t, goalRules)
	e.Seed(t, apptest.DaySpans()...)
	m := newModel(t, e)
	m = press(t, m, "2")
	mustContain(t, m, " Code ≥2h/day                  met 1 of 1 day",
		" project tock ≥10h/week        1h40/10h00 short 8h20",
		" Entertainment ≤10m/day        over 1 of 1 day")
}
