package report

import (
	"testing"
)

func TestBurn(t *testing.T) {
	now := int64(1_000_000_000_000)
	day := int64(24 * 3600_000)
	// 2h of 10h spent, 3h this week: 8h left at 3h/7 a day is 18.67 days.
	r := Burn(Budget{Project: "tock", BudgetMS: 10 * 3600_000}, 2*3600_000, 3*3600_000, now)
	if r.SpentMS != 2*3600_000 || r.PerDayMS != 3*3600_000/7 || r.Pct() != 20 || r.Over() {
		t.Fatalf("burn = %+v", r)
	}
	if d := r.RunsOutMS - (now + 56*day/3); d < -60_000 || d > 60_000 {
		t.Fatalf("runs out %d ms off", d)
	}
	if r := Burn(Budget{BudgetMS: 15 * 60_000}, 30*60_000, 30*60_000, now); !r.Over() || r.Pct() != 200 || r.RunsOutMS != 0 {
		t.Fatalf("over = %+v", r)
	}
	// A few seconds a week is no pace: no run-out day.
	if r := Burn(Budget{BudgetMS: 3600_000}, 0, 60_000, now); r.RunsOutMS != 0 || r.PerDayMS != 60_000/7 {
		t.Fatalf("slow = %+v", r)
	}
}

func TestProjectDayMS(t *testing.T) {
	vs, ls := build(t,
		sv{id: 1, start: "08:00", end: "10:00", app: "Code", proj: "tock", cat: "Code"},
		sv{id: 2, start: "10:00", end: "10:30", app: "idle", kind: "idle"},
		sv{id: 3, start: "10:30", end: "11:00", app: "Code", proj: "ATLAS", cat: "Code"},
		sv{id: 4, start: "11:00", end: "11:10", app: "Code", cat: "Code"},
	)
	got := ProjectDayMS(vs, ls)
	if len(got) != 2 || got["tock"] != 2*3600_000 || got["ATLAS"] != 30*60_000 {
		t.Fatalf("got %v", got)
	}
	if ProjectDayMS(nil, nil) != nil {
		t.Fatal("empty day must be nil")
	}
}
