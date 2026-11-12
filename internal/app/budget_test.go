package app

import (
	"strings"
	"testing"

	"tock/internal/apptest"
)

func TestBudgets(t *testing.T) {
	e := apptest.New(t, strings.Replace(apptest.Rules, "name = \"tock\"\n", "name = \"tock\"\nbudget = \"2h\"\n", 1))
	e.Seed(t, apptest.DaySpans()...)
	e.Seed(t, apptest.FocusHistory(3)...)
	a := open(t, e)
	rows, err := a.Budgets()
	if err != nil {
		t.Fatal(err)
	}
	// Today's 1h40 on tock and an hour on each of the 3 days before.
	if len(rows) != 1 || rows[0].Project != "tock" || rows[0].SpentMS != 280*60_000 || !rows[0].Over() || rows[0].PerDayMS != 280*60_000/7 {
		t.Fatalf("rows = %+v", rows)
	}

	if err := a.EditProject(ProjectSpec{Name: "tock", Budget: ptr(int64(10 * 3600_000)), BudgetSince: ptr(dayKey(e.Clock))}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	rows, err = a.Budgets()
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].BudgetMS != 10*3600_000 || rows[0].SpentMS != 100*60_000 || rows[0].Over() || rows[0].RunsOutMS == 0 {
		t.Fatalf("since today: %+v", rows)
	}
	if err := a.EditProject(ProjectSpec{Name: "tock", Budget: ptr(int64(-1)), BudgetSince: ptr("")}); err != nil {
		t.Fatal(err)
	}
	a.Reload()
	if rows, _ := a.Budgets(); rows != nil {
		t.Fatalf("dropped: %+v", rows)
	}
}

func ptr[T any](v T) *T { return &v }

func dayKey(t interface{ Format(string) string }) string { return t.Format("2006-01-02") }
