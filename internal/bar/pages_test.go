package bar

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/stats"
)

func pagesDay() (report.Day, report.Week, app.Usual) {
	day := time.Date(2026, 11, 5, 0, 0, 0, 0, islandLoc) // a Thursday
	h := func(hh int) int64 { return day.Add(time.Duration(hh) * time.Hour).UnixMilli() }
	var hours [24]int64
	hours[9], hours[10] = 50*60_000, 40*60_000
	d := report.Day{Date: day, TrackedMS: 5 * 3600_000,
		Score:    stats.Score{Value: 81},
		Stats:    stats.Day{FocusMS: 192 * 60_000},
		Hours:    report.Heatmap{First: 9, Last: 11, MS: map[string][24]int64{"deep": hours}},
		Projects: []report.Share{{Name: "tock", MS: 3 * 3600_000}, {Name: "", MS: 3600_000}, {Name: "Licence", MS: 1800_000}, {Name: "x", MS: 1}},
		Timeline: []report.Row{{Start: h(9), End: h(12), Label: classify.Label{Kind: "deep"}}},
		Goals: []report.GoalRow{
			{Goal: classify.Goal{Category: "Study", MS: 2 * 3600_000}, DoneMS: 3600_000, State: "short", Streak: 3},
			{Goal: classify.Goal{Category: "Fun", AtMost: true, MS: 45 * 60_000}, DoneMS: 50 * 60_000, State: "over"},
		}}
	w := report.Week{Monday: day.AddDate(0, 0, -3), TrackedMS: 20 * 3600_000,
		ByKind: map[string]int64{"deep": 12 * 3600_000, "distract": 3600_000, "": 7 * 3600_000}}
	for i := range w.Days {
		w.Days[i].Date = w.Monday.AddDate(0, 0, i)
		w.Days[i].Future = i > 3
		if i <= 3 {
			w.Days[i].TrackedMS = 5 * 3600_000
			w.Days[i].ByKind = map[string]int64{"deep": 4 * 3600_000, "distract": 3600_000}
			w.Days[i].Score = stats.Score{Value: 70 + i}
		}
	}
	return d, w, app.Usual{Days: 3, Score: 75, FocusMS: 168 * 60_000}
}

func pageIDs(ps []Page) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func TestTodayPagesNormalDay(t *testing.T) {
	d, w, u := pagesDay()
	s := Snapshot{OK: true, Loc: islandLoc}
	ps := TodayPages(s, d, w, u, d.Date.Add(15*time.Hour).UnixMilli())
	if got := pageIDs(ps); !reflect.DeepEqual(got, []string{"today", "week", "goals"}) {
		t.Fatalf("pages = %v", got)
	}
	today := ps[0]
	if today.Cells[0] != (Cell{"Score", "81", "+6", "good"}) || today.Cells[1] != (Cell{"Deep focus", "3h12", "+24m", "good"}) {
		t.Fatalf("today cells = %+v", today.Cells)
	}
	if len(today.Strip) != stripCells || today.Note != "" {
		t.Fatalf("today strip %d note %q", len(today.Strip), today.Note)
	}
	if today.Mid == nil || len(today.Mid.Bars) != 3 || today.Mid.Bars[0].Parts[0] != (Part{"deep", 50 * 60_000}) || today.Mid.Bars[0].Label != "09" {
		t.Fatalf("today mid = %+v", today.Mid)
	}
	if today.Big == nil || len(today.Big.Rows) != 3 || today.Big.Rows[1] != [2]string{"No project", "1h00"} {
		t.Fatalf("today big = %+v", today.Big)
	}
	week := ps[1]
	if len(week.Bars) != 7 || !week.Bars[3].Hi || week.Bars[3].Label != "Thu" || len(week.Bars[5].Parts) != 0 {
		t.Fatalf("week bars = %+v", week.Bars)
	}
	if week.Bars[0].Parts[0] != (Part{"deep", 4 * 3600_000}) || week.Bars[0].Sub != "70" {
		t.Fatalf("monday = %+v", week.Bars[0])
	}
	if week.Mid == nil || week.Mid.Cells[0] != (Cell{K: "Tracked", V: "20h00"}) || week.Mid.Cells[2] != (Cell{K: "Avg score", V: "72"}) {
		t.Fatalf("week mid = %+v", week.Mid)
	}
	if week.Big == nil || week.Big.Rows[0] != [2]string{"Deep", "12h00"} || week.Big.Rows[len(week.Big.Rows)-1] != [2]string{"No label", "7h00"} {
		t.Fatalf("week big = %+v", week.Big)
	}
	goals := ps[2]
	want := []Ring{
		{Label: "Study", Value: "1h00 / 2h00", Sub: "3 day streak", Frac: 0.5, Kind: "shallow"},
		{Label: "Fun", Value: "50m / 45m", Frac: 1, Kind: "distract"},
	}
	if !reflect.DeepEqual(goals.Rings, want) {
		t.Fatalf("rings = %+v", goals.Rings)
	}
}

func TestTodayPagesEmptyDayAndNoGoals(t *testing.T) {
	day := time.Date(2026, 11, 5, 0, 0, 0, 0, islandLoc)
	d := report.Day{Date: day}
	w := report.Week{Monday: day.AddDate(0, 0, -3)}
	ps := TodayPages(Snapshot{OK: true, Loc: islandLoc}, d, w, app.Usual{}, day.Add(8*time.Hour).UnixMilli())
	if ps[0].Note != "Nothing tracked yet today." || ps[0].Mid != nil {
		t.Fatalf("today = %+v", ps[0])
	}
	if g := ps[2]; g.Note != "No goals yet · tock goal add" || len(g.Rings) != 0 {
		t.Fatalf("goals = %+v", g)
	}
}

func TestTodayPagesSessionFirst(t *testing.T) {
	d, w, u := pagesDay()
	start := d.Date.Add(14 * time.Hour).UnixMilli()
	s := Snapshot{OK: true, Loc: islandLoc,
		Session: &focus.Session{Goal: "Write the notch pages", StartMS: start, PlannedMS: start + 50*60_000},
		Live:    focus.Live{Level: 82}}
	ps := TodayPages(s, d, w, u, start+20*60_000)
	if got := pageIDs(ps); !reflect.DeepEqual(got, []string{"session", "today", "week", "goals"}) {
		t.Fatalf("pages = %v", got)
	}
	ss := ps[0]
	if ss.Lead != "Write the notch pages" || len(ss.Rings) != 1 || ss.Rings[0].Value != "30m left" || ss.Rings[0].Frac != 0.4 || ss.Rings[0].Kind != "deep" {
		t.Fatalf("session = %+v", ss)
	}
	if ss.Cells[0] != (Cell{K: "Focus", V: "82%"}) || ss.Cells[1] != (Cell{K: "Started", V: "14:00"}) || ss.Cells[2] != (Cell{K: "Ends", V: "14:50"}) {
		t.Fatalf("session cells = %+v", ss.Cells)
	}
}

func TestPagesJSON(t *testing.T) {
	d, w, u := pagesDay()
	i := Snapshot{OK: true, Loc: islandLoc}.IslandBase(d.Date.Add(15 * time.Hour).UnixMilli())
	i.Hover.Pages = TodayPages(Snapshot{OK: true, Loc: islandLoc}, d, w, u, d.Date.Add(15*time.Hour).UnixMilli())
	var back Island
	if err := json.Unmarshal([]byte(i.JSON()), &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Hover.Pages, i.Hover.Pages) {
		t.Fatalf("pages did not round-trip:\n%+v\n%+v", back.Hover.Pages, i.Hover.Pages)
	}
}
