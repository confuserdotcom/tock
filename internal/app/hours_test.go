package app

import (
	"testing"

	"tock/internal/apptest"
	"tock/internal/report"
)

const streakRules = apptest.Rules + `
[[goal]]
category = "Code"
at_least = "1h"
`

// seedStreak puts Code time on the days before Now (a Saturday): 1h on
// Fri, Thu and Wed, none on Tue, 30m on Mon.
func seedStreak(t *testing.T, e *apptest.Env) {
	e.Seed(t,
		apptest.Span{ID: 101, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ a", Day: -1},
		apptest.Span{ID: 102, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ a", Day: -2},
		apptest.Span{ID: 103, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ a", Day: -3},
		apptest.Span{ID: 105, Start: "09:00", End: "09:30", Bundle: "term", App: "WezTerm", Title: "tock │ a", Day: -5},
	)
}

func TestDayStreaks(t *testing.T) {
	e := apptest.New(t, streakRules)
	e.Seed(t, apptest.DaySpans()...)
	seedStreak(t, e)
	a := open(t, e)
	d, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Goals) != 1 || d.Goals[0].Streak != 4 || d.Goals[0].Best != 4 {
		t.Fatalf("goals %+v, want streak 4 (today met + 3 days)", d.Goals)
	}
	w, err := a.Week(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if w.Goals[0].Streak != 4 {
		t.Errorf("week goal streak %d, want 4", w.Goals[0].Streak)
	}
}

func TestDayWorkHours(t *testing.T) {
	e := apptest.New(t, streakRules)
	e.Seed(t, apptest.DaySpans()...)
	seedStreak(t, e)
	e.WriteConfig(t, "[hours]\nstart = \"09:00\"\nend = \"09:30\"\n")
	a := open(t, e)
	d, err := a.Day(e.Clock) // Saturday: a day off
	if err != nil {
		t.Fatal(err)
	}
	if !d.Off || d.Score.Value != 0 || d.TrackedMS == 0 || report.Recap(d) != "Day off: 2h54 tracked, not scored." {
		t.Fatalf("off %v score %d tracked %d recap %q", d.Off, d.Score.Value, d.TrackedMS, report.Recap(d))
	}
	if d.Goals[0].Streak != 3 {
		t.Errorf("streak %d, want 3: a day off is skipped", d.Goals[0].Streak)
	}
	fri, err := a.Day(e.Clock.AddDate(0, 0, -1))
	if err != nil {
		t.Fatal(err)
	}
	if fri.Off || fri.Stats.ActiveMS != 30*60_000 || fri.OutsideMS != 30*60_000 || !fri.Closed {
		t.Fatalf("friday off %v active %d outside %d closed %v", fri.Off, fri.Stats.ActiveMS, fri.OutsideMS, fri.Closed)
	}
}
