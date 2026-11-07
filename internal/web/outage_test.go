package web

import (
	"testing"

	"tock/internal/apptest"
	"tock/internal/stats"
)

// /api/day and /api/week carry the outages with why, apart from the
// timeline; the recorded totals do not change.
func TestDayWeekOutages(t *testing.T) {
	f := newFixture(t)
	var before dayJSON
	f.get("/api/day", &before)
	f.e.Runs(t,
		apptest.Run{Start: "08:00", Last: "12:30", Boot: 1},
		apptest.Run{Start: "13:10", Last: "14:59", Boot: 1},
	)
	var d dayJSON
	f.get("/api/day", &d)
	want := outageJSON{From: apptest.At(t, "12:30", 0), To: apptest.At(t, "13:10", 0), Kind: stats.GapCrash,
		Why: "trackd stopped unexpectedly"}
	if len(d.Outages) != 1 || d.Outages[0] != want {
		t.Fatalf("day outages = %+v, want [%+v]", d.Outages, want)
	}
	if d.TrackedMS != before.TrackedMS || d.IdleMS != before.IdleMS || len(d.Timeline) != len(before.Timeline) {
		t.Fatal("outages changed the recorded day")
	}
	var w weekJSON
	f.get("/api/week", &w)
	if len(w.Outages) != 1 || w.Outages[0] != want {
		t.Fatalf("week outages = %+v", w.Outages)
	}
	// none: an empty list, not null
	f2 := newFixture(t)
	var d2 dayJSON
	f2.get("/api/day", &d2)
	if d2.Outages == nil || len(d2.Outages) != 0 {
		t.Fatalf("no outages = %#v, want []", d2.Outages)
	}
}
