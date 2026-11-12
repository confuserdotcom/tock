package report

import (
	"reflect"
	"testing"
)

func TestBurns(t *testing.T) {
	vs, ls := build(t,
		sv{id: 1, start: "08:00", end: "10:00", app: "Code", proj: "tock", cat: "Code"},
		sv{id: 2, start: "10:00", end: "10:30", app: "idle", kind: "idle"},
		sv{id: 3, start: "10:30", end: "11:30", app: "Code", proj: "tock", cat: "Code"},
		sv{id: 4, start: "11:30", end: "12:00", app: "Code", proj: "ATLAS", cat: "Code"},
	)
	now := hm(t, "12:00")
	day := int64(24 * 3600_000)
	got := Burns([]Budget{
		{Project: "tock", BudgetMS: 10 * 3600_000, SinceMS: hm(t, "09:00")},
		{Project: "ATLAS", BudgetMS: 15 * 60_000},
		{Project: "none", BudgetMS: 3600_000},
	}, vs, ls, now)
	want := []BurnRow{
		// 09:00-10:00 + 10:30-11:30 = 2h of 10h; the week's pace is 3h/7 per
		// day, so the 8h left last 18.67 days.
		{Budget: Budget{Project: "tock", BudgetMS: 10 * 3600_000, SinceMS: hm(t, "09:00")}, SpentMS: 2 * 3600_000,
			PerDayMS: 3 * 3600_000 / 7},
		{Budget: Budget{Project: "ATLAS", BudgetMS: 15 * 60_000}, SpentMS: 30 * 60_000, PerDayMS: 30 * 60_000 / 7},
		{Budget: Budget{Project: "none", BudgetMS: 3600_000}},
	}
	if d := got[0].RunsOutMS - (now + 56*day/3); d < -60_000 || d > 60_000 {
		t.Fatalf("runs out %d ms off 18.67 days", d)
	}
	got[0].RunsOutMS = 0
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	if got[0].Pct() != 20 || got[1].Pct() != 200 || !got[1].Over() || got[0].Over() {
		t.Fatalf("pct %d %d", got[0].Pct(), got[1].Pct())
	}
}
