package app

import (
	"errors"
	"os"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
)

func projectNamed(t *testing.T, a *App, name string) classify.Project {
	t.Helper()
	for _, p := range a.Rules().Projects {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no project %q in %+v", name, a.Rules().Projects)
	return classify.Project{}
}

func TestProjectRate(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	f := func(v float64) *float64 { return &v }

	if err := a.AddProject(ProjectSpec{Name: "Client", Rate: f(45.5)}); err != nil {
		t.Fatal(err)
	}
	if p := projectNamed(t, a, "Client"); p.Rate == nil || *p.Rate != 45.5 {
		t.Fatalf("added rate %v", p.Rate)
	}
	if err := a.EditProject(ProjectSpec{Name: "client", Rate: f(60)}); err != nil {
		t.Fatal(err)
	}
	if p := projectNamed(t, a, "Client"); p.Rate == nil || *p.Rate != 60 {
		t.Fatalf("edited rate %v", p.Rate)
	}
	if err := a.EditProject(ProjectSpec{Name: "Client", Rate: f(-1)}); err != nil {
		t.Fatal(err)
	}
	if p := projectNamed(t, a, "Client"); p.Rate != nil {
		t.Fatalf("dropped rate is %v", *p.Rate)
	}
	big := f(float64(classify.MaxRate + 1))
	if err := a.AddProject(ProjectSpec{Name: "Big", Rate: big}); err == nil {
		t.Fatal("added a rate over MaxRate")
	}
	if err := a.EditProject(ProjectSpec{Name: "Client", Rate: big}); err == nil {
		t.Fatal("set a rate over MaxRate")
	}
}

func TestProjectBillableAndClient(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	f := func(v float64) *float64 { return &v }
	b := func(v bool) *bool { return &v }
	str := func(v string) *string { return &v }

	if err := a.AddProject(ProjectSpec{Name: "Acme", Rate: f(95.5), Billable: b(false), Client: str(" Acme Corp ")}); err != nil {
		t.Fatal(err)
	}
	if p := projectNamed(t, a, "Acme"); p.Billable == nil || *p.Billable || p.Client != "Acme Corp" || a.Rules().Billable("Acme") {
		t.Fatalf("added %+v", p)
	}
	if err := a.EditProject(ProjectSpec{Name: "acme", Billable: b(true), Client: str("Globex")}); err != nil {
		t.Fatal(err)
	}
	if p := projectNamed(t, a, "Acme"); p.Billable != nil || p.Client != "Globex" || !a.Rules().Billable("Acme") {
		t.Fatalf("edited %+v", p)
	}
	if err := a.EditProject(ProjectSpec{Name: "Acme", Client: str("")}); err != nil {
		t.Fatal(err)
	}
	if p := projectNamed(t, a, "Acme"); p.Client != "" || p.Rate == nil {
		t.Fatalf("cleared client %+v", p)
	}
	long := str(strings.Repeat("c", classify.MaxClient+1))
	if err := a.EditProject(ProjectSpec{Name: "Acme", Client: long}); err == nil {
		t.Fatal("set a client over MaxClient")
	}
}

func TestWithRulesUndo(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	err := a.WithRulesUndo("added category Reading", func() error {
		return a.AddCategory(CategorySpec{Name: "Reading", Kind: classify.KindNeutral})
	})
	if err != nil || !a.Rules().HasCategory("Reading") {
		t.Fatalf("add: %v", err)
	}
	what, err := a.Undo()
	if err != nil || what != "added category Reading" {
		t.Fatalf("undo: %q %v", what, err)
	}
	if b, _ := os.ReadFile(e.Rules); string(b) != apptest.Rules || a.Rules().HasCategory("Reading") {
		t.Fatalf("undo left rules.toml:\n%s", b)
	}

	// A failed change and a no-op journal nothing.
	if err := a.WithRulesUndo("x", func() error { return errors.New("no") }); err == nil {
		t.Fatal("lost fn's error")
	}
	if err := a.WithRulesUndo("y", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Undo(); !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("journaled a failed or empty change: %v", err)
	}
}

func TestLibrary(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, append(apptest.DaySpans(),
		apptest.Span{ID: 50, Start: "09:00", End: "09:30", Bundle: "arc", App: "Arc", Title: "Docs", URL: "example.com/d", Day: -40})...)
	a := open(t, e)
	code := "Code"
	if err := a.AddProject(ProjectSpec{Name: "ana1", Category: &code}); err != nil {
		t.Fatal(err)
	}
	if err := a.Tag([]int64{50}, classify.Want{Category: "Code", Project: "ATLAS"}); err != nil {
		t.Fatal(err)
	}
	lib, err := a.Library()
	if err != nil {
		t.Fatal(err)
	}
	proj := map[string]LibProject{}
	for _, p := range lib.Projects {
		proj[p.Name] = p
	}
	cat := map[string]LibCategory{}
	for _, c := range lib.Categories {
		cat[c.Name] = c
	}
	if p := proj["tock"]; p.Last30MS != 100*60_000 || p.LastSeenMS != apptest.At(t, "10:40", 0) {
		t.Errorf("tock %+v", p.LibraryUse)
	}
	if p := proj["ATLAS"]; p.Last30MS != 0 || p.LastSeenMS != apptest.At(t, "09:30", -40) {
		t.Errorf("ATLAS %+v", p.LibraryUse)
	}
	if p := proj["Old"]; !p.Archived || p.LastSeenMS != 0 {
		t.Errorf("Old %+v", p)
	}
	if c := cat["Entertainment"]; c.Last30MS != 12*60_000 {
		t.Errorf("Entertainment %+v", c.LibraryUse)
	}
	if c := cat["Code"]; c.Projects != 1 {
		t.Errorf("Code projects %d", c.Projects)
	}
	if c := cat["Study"]; c.LastSeenMS != 0 {
		t.Errorf("Study %+v", c.LibraryUse)
	}
	if len(lib.Goals) != len(a.Rules().Goals) || len(lib.Lookalikes) != len(classify.Duplicates(a.Rules())) {
		t.Errorf("goals %d lookalikes %d", len(lib.Goals), len(lib.Lookalikes))
	}
}
