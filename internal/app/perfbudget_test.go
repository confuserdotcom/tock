package app

import (
	"testing"
)

// Performance budgets checked on every CI run. Wall-clock time is too noisy
// on shared runners, so these count allocations, which only move when the
// code does: the views tock draws all day must cost the same after half a
// year of history as after a week.

// allocsDay is the allocations of one a.Day over days of history.
func allocsDay(t *testing.T, days int) float64 {
	a, e := benchOpen(t, days)
	if _, err := a.Day(e.Clock); err != nil { // warm caches, as a running tock has
		t.Fatal(err)
	}
	return testing.AllocsPerRun(5, func() {
		if _, err := a.Day(e.Clock); err != nil {
			t.Fatal(err)
		}
	})
}

func TestBudgetDayFlatOverHistory(t *testing.T) {
	if raceOn {
		t.Skip("allocation budgets mean nothing under the race detector")
	}
	if testing.Short() {
		t.Skip("seeds half a year of spans")
	}
	week, half := allocsDay(t, 7), allocsDay(t, 180)
	t.Logf("Day allocs: 7d %.0f, 180d %.0f", week, half)
	if half > week*1.25+500 {
		t.Fatalf("Day over 180 days allocates %.0f, over 7 days %.0f: today's view reads old history", half, week)
	}
}

// allocsGuardTick is the allocations of one trackd focus tick over days of
// history.
func allocsGuardTick(t *testing.T, days int) float64 {
	a, _ := benchOpen(t, days)
	if _, _, err := a.GuardTick(false); err != nil {
		t.Fatal(err)
	}
	return testing.AllocsPerRun(5, func() {
		if _, _, err := a.GuardTick(false); err != nil {
			t.Fatal(err)
		}
	})
}

// guardTickAllocs is the ceiling for one GuardTick, about twice its count
// in November 2026 (~13k). It runs in trackd every 5 s, so its allocations
// are trackd's idle CPU and heap budget.
const guardTickAllocs = 26_000

func TestBudgetGuardTick(t *testing.T) {
	if raceOn {
		t.Skip("allocation budgets mean nothing under the race detector")
	}
	if testing.Short() {
		t.Skip("seeds half a year of spans")
	}
	week, half := allocsGuardTick(t, 7), allocsGuardTick(t, 180)
	t.Logf("GuardTick allocs: 7d %.0f, 180d %.0f", week, half)
	if half > week*1.25+500 {
		t.Fatalf("GuardTick over 180 days allocates %.0f, over 7 days %.0f: the tick reads old history", half, week)
	}
	if half > guardTickAllocs {
		t.Fatalf("GuardTick allocates %.0f per tick, budget %d", half, guardTickAllocs)
	}
}
