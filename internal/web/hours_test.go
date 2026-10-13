package web

import (
	"testing"

	"tock/internal/apptest"
)

func TestDayWorkAndStreak(t *testing.T) {
	f := newFixture(t)
	var d dayJSON
	f.get("/api/day", &d)
	if d.Work != nil {
		t.Fatalf("work %+v without [hours]", d.Work)
	}
	f.e.WriteConfig(t, "[hours]\nstart = \"09:00\"\nend = \"17:00\"\n")
	f.e.WriteRules(t, apptest.Rules+"\n[[goal]]\ncategory = \"Code\"\nat_least = \"1h\"\n")
	d = dayJSON{}
	f.get("/api/day", &d)
	if d.Work == nil || !d.Work.Off || d.Work.OutsideMS != d.TrackedMS {
		t.Fatalf("work %+v, want a day off (Saturday)", d.Work)
	}
	// A day off is skipped, so the met goal does not start a streak.
	if len(d.Goals) != 1 || d.Goals[0].State != "met" || d.Goals[0].Streak != 0 {
		t.Fatalf("goals %+v", d.Goals)
	}
}
