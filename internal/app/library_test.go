package app

import (
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
