package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/stats"
)

// history seeds days -7..-1 with one deep span each of 60, 90, ... 240
// minutes from 09:00 (ids 101..107; day -4 has 150 minutes), and today's
// usual spans. The median focus is 150 minutes, so the adaptive target is
// 165 minutes.
func history(t *testing.T, e *apptest.Env, days int) {
	t.Helper()
	ends := []string{"10:00", "10:30", "11:00", "11:30", "12:00", "12:30", "13:00"}
	for i := range days {
		e.Seed(t, apptest.Span{ID: int64(101 + i), Start: "09:00", End: ends[i], Bundle: "term", App: "WezTerm",
			Title: fmt.Sprintf("tock │ day %d", i), Day: -7 + i})
	}
	e.Seed(t, apptest.DaySpans()...)
}

func target(t *testing.T, a *App) stats.Target {
	t.Helper()
	d, err := a.Day(a.now())
	if err != nil {
		t.Fatal(err)
	}
	return d.Score.Target
}

func TestTargetDefaultUntilSevenWorkdays(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	history(t, e, 6) // days -7..-2: six workdays; yesterday has nothing
	a := open(t, e)
	if got := target(t, a); got != (stats.Target{MS: stats.DefaultTargetMS, Workdays: 6}) {
		t.Fatalf("6 workdays: %+v", got)
	}
	// A day later, today (2h54 active) is the seventh workday.
	e.Clock = e.Clock.Add(24 * time.Hour)
	got := target(t, a)
	// focus minutes 60, 90, 120, 150, 180, 210 and today's 134: median 134 -> 147.4
	if !got.Adaptive || got.Workdays != 7 || got.MS != 147*60_000 {
		t.Fatalf("7 workdays: %+v (%s)", got, report.Dur(got.MS))
	}
}

func TestTargetFollowsOverridesFromAnotherProcess(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	history(t, e, 7)
	a := open(t, e)
	if got := target(t, a); got.MS != 165*60_000 {
		t.Fatalf("before: %s", report.Dur(got.MS))
	}
	b := open(t, e) // a second session, like tock tag in another terminal
	if err := b.Tag([]int64{104}, classify.Want{Category: "Entertainment"}); err != nil {
		t.Fatal(err)
	}
	// day -4 drops from 150 to 0: median of 0, 60, 90, 120, 180, 210, 240 is 120
	if got := target(t, a); got.MS != 132*60_000 {
		t.Fatalf("after tag: %s", report.Dur(got.MS))
	}
}

func TestTargetFollowsRules(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	history(t, e, 7)
	a := open(t, e)
	if got := target(t, a); got.MS != 165*60_000 {
		t.Fatalf("before: %s", report.Dur(got.MS))
	}
	// Code becomes shallow: no focus anywhere, the target clamps at 2 h.
	rules := strings.Replace(apptest.Rules, "name = \"Code\"\nkind = \"deep\"", "name = \"Code\"\nkind = \"shallow\"", 1)
	if rules == apptest.Rules {
		t.Fatal("Code category not found in apptest.Rules")
	}
	e.WriteRules(t, rules)
	if _, err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := target(t, a); got.MS != stats.MinTargetMS {
		t.Fatalf("after rules: %s", report.Dur(got.MS))
	}
}

func TestWeekTargets(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	history(t, e, 7)
	a := open(t, e)
	w, err := a.Week(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	// Mon 5 Oct has days -7..-6 (2 workdays) before it; Sat 10 Oct has 7.
	if got := w.Days[0].Score.Target; got != (stats.Target{MS: stats.DefaultTargetMS, Workdays: 2}) {
		t.Fatalf("Monday target = %+v", got)
	}
	if got := w.Days[5].Score.Target; got.MS != 165*60_000 || !got.Adaptive {
		t.Fatalf("Saturday target = %+v", got)
	}
	if w.Days[6].Score != (stats.Score{}) {
		t.Fatalf("Sunday is in the future: %+v", w.Days[6].Score)
	}
	d, _ := a.Day(e.Clock)
	if d.Score != w.Days[5].Score {
		t.Fatalf("today's score differs between Day and Week: %+v vs %+v", d.Score, w.Days[5].Score)
	}
}

func TestConfigWeightsAndErrors(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.WriteConfig(t, "[score]\ndepth = 0.4\nblocks = 0.4\ncalm = 0.2\n")
	a := open(t, e)
	want := stats.Weights{Depth: 0.4, Blocks: 0.4, Calm: 0.2}
	if a.Config().Score != want || a.ConfigErr() != nil {
		t.Fatalf("config = %+v err %v", a.Config(), a.ConfigErr())
	}
	e.WriteConfig(t, "[score]\ndepth = 0.9\n")
	if changed, err := a.Reload(); !changed || err != nil {
		t.Fatalf("config edit: changed %v err %v", changed, err)
	}
	if a.ConfigErr() == nil || a.Config().Score != want {
		t.Fatalf("broken config must keep the last good weights: %+v err %v", a.Config(), a.ConfigErr())
	}
	h, _ := a.Health()
	if h.ConfigErr == nil {
		t.Fatal("Health must report the config error")
	}
	d, _ := a.Day(e.Clock)
	if d.Score.Weights != want {
		t.Fatalf("score weights = %+v", d.Score.Weights)
	}
}

func TestBrokenConfigAtOpenUsesDefaults(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.WriteConfig(t, "nonsense = [\n")
	a := open(t, e)
	if a.ConfigErr() == nil || a.Config().Score != stats.DefaultWeights {
		t.Fatalf("config = %+v err %v", a.Config(), a.ConfigErr())
	}
}

func TestDayScore(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	d, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.ScoreLine(d); got != "score 74 · focus 2h14 / 4h00 · 1 block, longest 2h14 · 2 switches" {
		t.Fatalf("score line = %q", got)
	}
}
