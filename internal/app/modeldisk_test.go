package app

import (
	"reflect"
	"testing"
	"time"

	"tock/internal/apptest"
)

// A new process uses the model another trained while it is fresh and its
// labels are the same; otherwise it trains its own.
func TestModelFromDisk(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	day := func(a *App) {
		t.Helper()
		if _, err := a.Day(a.now()); err != nil {
			t.Fatal(err)
		}
	}
	a := open(t, e)
	day(a)
	if a.model == nil {
		t.Fatal("no model trained")
	}
	// Spans are not in the key: with them gone only the stored model can
	// still be a's.
	if _, err := e.RawDB(t).Exec(`DELETE FROM span`); err != nil {
		t.Fatal(err)
	}
	b := open(t, e)
	day(b)
	if !reflect.DeepEqual(b.model, a.model) {
		t.Fatal("a second process trained again instead of using the stored model")
	}
	// Too old: trained again, on nothing now.
	e.Clock = e.Clock.Add(modelEveryMS * time.Millisecond)
	c := open(t, e)
	day(c)
	if reflect.DeepEqual(c.model, a.model) {
		t.Fatal("a model older than modelEveryMS must not be used")
	}
}
