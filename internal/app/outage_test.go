package app

import (
	"reflect"
	"testing"

	"tock/internal/apptest"
	"tock/internal/stats"
	"tock/internal/store"
)

// The day lists the outages, apart from the recorded time: a crash, a
// config pause; sleep and a quiet run (no window open) are not outages.
func TestDayOutages(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Runs(t,
		apptest.Run{Start: "07:00", Stop: "08:00", Reason: store.RunAsleep, Boot: 1},
		apptest.Run{Start: "08:30", Last: "09:40", Boot: 1}, // left open: crashed
		apptest.Run{Start: "10:05", Stop: "11:00", Reason: store.RunState, Boot: 1},
		apptest.Run{Start: "11:00", Stop: "11:30", Reason: store.RunState, State: store.RunPaused, Boot: 1},
		apptest.Run{Start: "11:30", Last: "14:59", Boot: 1}, // live
	)
	a := open(t, e)
	d, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	want := []stats.Gap{
		{From: apptest.At(t, "09:40", 0), To: apptest.At(t, "10:05", 0), Kind: stats.GapCrash},
		{From: apptest.At(t, "11:00", 0), To: apptest.At(t, "11:30", 0), Kind: stats.GapConfig},
	}
	if !reflect.DeepEqual(d.Outages, want) {
		t.Fatalf("outages = %+v, want %+v", d.Outages, want)
	}
	// the recorded totals are what they were without the run rows
	e2 := apptest.New(t, apptest.Rules)
	e2.Seed(t, apptest.DaySpans()...)
	d2, err := open(t, e2).Day(e2.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if d.TrackedMS != d2.TrackedMS || d.IdleMS != d2.IdleMS || len(d.Timeline) != len(d2.Timeline) {
		t.Fatalf("outages changed the recorded day: tracked %d/%d idle %d/%d rows %d/%d",
			d.TrackedMS, d2.TrackedMS, d.IdleMS, d2.IdleMS, len(d.Timeline), len(d2.Timeline))
	}
	w, err := a.Week(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w.Outages, want) {
		t.Fatalf("week outages = %+v", w.Outages)
	}
}

// Before trackd ever ran there is nothing to explain.
func TestDayNoRunsNoOutages(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	d, err := open(t, e).Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Outages) != 0 {
		t.Fatalf("outages = %+v", d.Outages)
	}
}
