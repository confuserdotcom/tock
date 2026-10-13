package tui

import (
	"testing"

	"tock/internal/apptest"
)

func TestTodayWorkHours(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.WriteConfig(t, "[hours]\ndays = [\"sat\"]\nstart = \"09:00\"\nend = \"10:00\"\n")
	m := newModel(t, e)
	mustContain(t, m, "good day  · hours 09:00–10:00 +1h54 outside")

	e2 := apptest.New(t, apptest.Rules)
	e2.Seed(t, apptest.DaySpans()...)
	e2.WriteConfig(t, "[hours]\nstart = \"09:00\"\nend = \"17:00\"\n")
	mustContain(t, newModel(t, e2), "–  score  day off")
}

func TestGoalChipStreak(t *testing.T) {
	e := apptest.New(t, apptest.Rules+"\n[[goal]]\ncategory = \"Code\"\nat_least = \"1h\"\n")
	e.Seed(t, apptest.DaySpans()...)
	e.Seed(t, apptest.Span{ID: 101, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ a", Day: -1})
	mustContain(t, newModel(t, e), "✓ Code 2h14/1h00 · 2d")
}
