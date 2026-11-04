package app

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"tock/internal/apptest"
)

// seedDays writes days of busy history before today through one
// transaction: 300 spans a day from 08:00, terminals, browsers and idle.
func seedDays(t *testing.T, e *apptest.Env, days int) {
	t.Helper()
	db := e.RawDB(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	apps := []struct{ bundle, app, title string }{
		{"term", "WezTerm", "tock │ nvim %d"}, {"com.google.Chrome", "Chrome", "page %d - YouTube"},
		{"md.obsidian", "Obsidian", "Thermodynamique %d"}, {"tock.idle", "Idle", ""},
	}
	id := int64(1)
	midnight := time.Date(e.Clock.Year(), e.Clock.Month(), e.Clock.Day(), 0, 0, 0, 0, e.Clock.Location())
	for d := days; d >= 1; d-- {
		at := midnight.AddDate(0, 0, -d).Add(8 * time.Hour).UnixMilli()
		for i := 0; i < 300; i++ {
			a := apps[(i*7+d)%len(apps)]
			var title any
			if a.title != "" {
				title = fmt.Sprintf(a.title, i%40)
			}
			end := at + int64(30_000+(i%13)*20_000)
			if _, err := tx.Exec(`INSERT INTO span(id, start, "end", bundle_id, app, title) VALUES(?,?,?,?,?,?)`,
				id, at, end, a.bundle, a.app, title); err != nil {
				t.Fatal(err)
			}
			id++
			at = end
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// allocDay is the bytes a fresh process allocates to open today.
func allocDay(t *testing.T, e *apptest.Env) uint64 {
	t.Helper()
	a := open(t, e)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if _, err := a.Day(a.now()); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestBudgetGrowsLinearly guards against work that grows with the square
// of history: building every past day (after a rules change) from twice
// the history may cost about twice as much, not four times.
func TestBudgetGrowsLinearly(t *testing.T) {
	if testing.Short() {
		t.Skip("builds 120 days of history")
	}
	cold := func(days int) uint64 {
		e := apptest.New(t, apptest.Rules)
		seedDays(t, e, days)
		return allocDay(t, e)
	}
	small, big := cold(60), cold(120)
	t.Logf("60 days %d MB, 120 days %d MB", small>>20, big>>20)
	if r := float64(big) / float64(small); r > 2.2 {
		t.Fatalf("60 days cost %d MB, 120 days %d MB: x%.1f, want about x2", small>>20, big>>20, r)
	}
}

// TestBudgetWarmRun guards the disk cache: a second process on the same
// data allocates a small part of what the first did.
func TestBudgetWarmRun(t *testing.T) {
	if testing.Short() {
		t.Skip("builds 60 days of history")
	}
	e := apptest.New(t, apptest.Rules)
	seedDays(t, e, 60)
	first := allocDay(t, e)
	second := allocDay(t, e)
	if second*5 > first {
		t.Fatalf("first run %d MB, second %d MB: the second must take less than a fifth", first>>20, second>>20)
	}
}
