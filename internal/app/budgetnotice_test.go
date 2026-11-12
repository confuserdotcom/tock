package app

import (
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/focus"
)

func budgetNotes(t *testing.T, a *App) []focus.Event {
	t.Helper()
	notes, _, err := a.GuardTick(false)
	if err != nil {
		t.Fatal(err)
	}
	var out []focus.Event
	for _, n := range notes {
		if n.Topic == focus.TopicBudget {
			out = append(out, n)
		}
	}
	return out
}

func TestBudgetNotices(t *testing.T) {
	e := apptest.New(t, strings.Replace(apptest.Rules, "name = \"tock\"\n", "name = \"tock\"\nbudget = \"2h\"\n", 1))
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	// Today's 1h40 on tock is 83% of 2h: one notice.
	ns := budgetNotes(t, a)
	if len(ns) != 1 || ns[0].Title != "tock: 83% of its 2h budget" || !strings.Contains(ns[0].Body, "1h40 spent") {
		t.Fatalf("notices = %+v", ns)
	}
	if n := ns[0]; n.Project != "tock" || n.BudgetMS != 2*3600_000 || n.SpentMS != 100*60_000 {
		t.Fatalf("notice numbers = %+v", n)
	}
	// The notch moment reads them back from the queued notice.
	if err := a.Enqueue(ns[0]); err != nil {
		t.Fatal(err)
	}
	ids, err := a.TakeNotices()
	if err != nil || len(ids) == 0 {
		t.Fatalf("pending = %v, %v", ids, err)
	}
	if _, d, err := a.Notice(ids[len(ids)-1].ID); err != nil || d.Project != "tock" || d.BudgetMS != 2*3600_000 || d.SpentMS != 100*60_000 {
		t.Fatalf("queued data = %+v, %v", d, err)
	}
	// Not again: the next tick, an hour later, or from another process.
	if ns := budgetNotes(t, a); len(ns) != 0 {
		t.Fatalf("again within the hour: %+v", ns)
	}
	e.Clock = e.Clock.Add(2 * time.Hour)
	if ns := budgetNotes(t, a); len(ns) != 0 {
		t.Fatalf("again later: %+v", ns)
	}
	if ns := budgetNotes(t, open(t, e)); len(ns) != 0 {
		t.Fatalf("again from another process: %+v", ns)
	}
	// Past the budget: one more.
	if err := a.EditProject(ProjectSpec{Name: "tock", Budget: ptr(int64(90 * 60_000))}); err != nil {
		t.Fatal(err)
	}
	e.Clock = e.Clock.Add(2 * time.Hour)
	ns = budgetNotes(t, a)
	if len(ns) != 1 || ns[0].Title != "tock is over its 1h30 budget" || !strings.Contains(ns[0].Body, "over by 10m") {
		t.Fatalf("over = %+v", ns)
	}
}
