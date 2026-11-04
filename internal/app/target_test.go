package app

import (
	"fmt"
	"reflect"
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

// A new process reads closed past days from disk instead of computing them
// again: here the span of day -6 is gone, which no key reads, so only
// the cached day can still give 90 minutes.
func TestPastDaysFromDisk(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	history(t, e, 7)
	if got := target(t, open(t, e)); got.MS != 165*60_000 {
		t.Fatalf("first run: %s", report.Dur(got.MS))
	}
	db := e.RawDB(t)
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM derived`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("no day went to disk: %d %v", n, err)
	}
	if _, err := db.Exec(`DELETE FROM span WHERE id = 102`); err != nil {
		t.Fatal(err)
	}
	if got := target(t, open(t, e)); got.MS != 165*60_000 {
		t.Fatalf("second run must read day -6 from disk: %s", report.Dur(got.MS))
	}
	// An away row on day -6 changes its key: computed again, now without
	// its span.
	p := open(t, e)
	d6 := p.midnight(p.now()).AddDate(0, 0, -6)
	at := d6.Add(20 * time.Hour).UnixMilli()
	if _, err := db.Exec(`INSERT INTO away(start_ms, end_ms, at_ms) VALUES(?, ?, 1)`, at, at+60_000); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Day(p.now()); err != nil {
		t.Fatal(err)
	}
	if got := p.past[dateKey(d6)].Stats.FocusMS; got != 0 {
		t.Fatalf("day -6 kept its cached focus %s after its key changed", report.Dur(got))
	}
	if got := p.past[dateKey(d6.AddDate(0, 0, 1))].Stats.FocusMS; got != 120*60_000 {
		t.Fatalf("day -5 must stay as it was: %s", report.Dur(got))
	}
}

// A day an open span reaches is not closed: it is computed, never stored.
func TestOpenDayNotStored(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "", Bundle: "term", App: "WezTerm", Title: "tock │ x", Day: -1})
	a := open(t, e)
	if _, err := a.Day(a.now()); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.RawDB(t).QueryRow(`SELECT count(*) FROM derived WHERE name = ?`,
		dateKey(a.midnight(a.now()).AddDate(0, 0, -1))).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a day with an open span went to disk: %d %v", n, err)
	}
}

// modelRules add a Study rule for PDFs titled "Physique…": the readings of
// other PDFs no rule names are left to the local model.
const modelRules = apptest.Rules + `
[[rule]]
bundle = ["zathura"]
title = '^Physique'
category = "Study"
`

// The target measures past days with the yardstick the day itself shows:
// the local model's labels included. A model trained again later does not
// put closed days back to work.
func TestPastDaysLabelledLikeToday(t *testing.T) {
	e := apptest.New(t, modelRules)
	for day := -1; day <= 0; day++ {
		id := int64(10 * (day + 2))
		e.Seed(t,
			apptest.Span{ID: id, Start: "09:00", End: "10:00", Bundle: "zathura", App: "Zathura", Title: "Physique thermodynamique exercices", Day: day},
			apptest.Span{ID: id + 1, Start: "10:00", End: "11:00", Bundle: "zathura", App: "Zathura", Title: "thermodynamique exercices corriges", Day: day})
	}
	a := open(t, e)
	if _, err := a.Day(a.now()); err != nil {
		t.Fatal(err)
	}
	y := a.midnight(a.now()).AddDate(0, 0, -1)
	shown := func(a *App) stats.Day {
		t.Helper()
		vs, ls, err := a.labelled(y.UnixMilli(), y.AddDate(0, 0, 1).UnixMilli(), a.nowMS())
		if err != nil {
			t.Fatal(err)
		}
		wFrom, wTo := a.cfg.Hours.Window(y)
		return stats.Compute(stats.Clip(vs, ls, wFrom, wTo))
	}
	want := shown(a)
	if want.FocusMS != 120*60_000 {
		t.Fatalf("the model must fill the second hour: yesterday shows %s of focus", report.Dur(want.FocusMS))
	}
	if got := a.past[dateKey(y)].Stats; !reflect.DeepEqual(got, want) {
		t.Fatalf("yesterday for the target %+v, as shown %+v", got, want)
	}
	// A later process, with a model trained again, reads the closed day
	// from disk: its spans are gone, and only the stored day still has them.
	if _, err := e.RawDB(t).Exec(`DELETE FROM span WHERE id IN (10, 11)`); err != nil {
		t.Fatal(err)
	}
	e.Clock = e.Clock.Add(2 * modelEveryMS * time.Millisecond)
	b := open(t, e)
	if _, err := b.Day(b.now()); err != nil {
		t.Fatal(err)
	}
	if got := b.past[dateKey(y)].Stats; !reflect.DeepEqual(got, want) {
		t.Fatalf("a retrained model must not recompute a closed day: %+v, want %+v", got, want)
	}
}
