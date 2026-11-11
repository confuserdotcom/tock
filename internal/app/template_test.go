package app

import (
	"crypto/sha256"
	"os"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/config"
	"tock/internal/rulesfile"
)

// zedDays seeds an hour of Zed (a template "Code" editor, unlabelled by
// apptest.Rules) on each of the days days before today.
func zedDays(t *testing.T, e *apptest.Env, days int) {
	var sp []apptest.Span
	for i := 1; i <= days; i++ {
		sp = append(sp, apptest.Span{ID: int64(2000 + i), Start: "09:00", End: "10:00", Bundle: "dev.zed.Zed", App: "Zed", Title: "main.go", Day: -i})
	}
	e.Seed(t, sp...)
}

func codeMS(t *testing.T, a *App, from, to int64) int64 {
	t.Helper()
	vs, ls, err := a.labelled(from, to, a.nowMS())
	if err != nil {
		t.Fatal(err)
	}
	var ms int64
	for i := range vs {
		if ls[i].Category == "Code" {
			ms += vs[i].DurMS()
		}
	}
	return ms
}

func fileSum(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

// AC3, AC4, AC5: yours kept on top, a second apply changes nothing, and
// one tock undo puts the file back byte for byte.
func TestApplyTemplateKeepsUndoesAndIsIdempotent(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	before, _ := os.ReadFile(e.Rules)
	p, err := a.ApplyTemplates([]string{"client"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Empty() {
		t.Fatal("nothing applied")
	}
	after, _ := os.ReadFile(e.Rules)
	if !strings.HasPrefix(string(after), string(before)) {
		t.Fatal("your rules.toml is not kept at the top")
	}
	if !a.Rules().HasCategory("Meeting") {
		t.Fatal("not reloaded")
	}
	h := fileSum(t, e.Rules)
	st, _ := os.Stat(e.Rules)
	p2, err := a.ApplyTemplates([]string{"client"}, false)
	if err != nil {
		t.Fatal(err)
	}
	st2, _ := os.Stat(e.Rules)
	if !p2.Empty() || fileSum(t, e.Rules) != h || !st2.ModTime().Equal(st.ModTime()) {
		t.Fatal("second apply wrote rules.toml")
	}
	what, err := a.Undo()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(what, "template client") {
		t.Fatalf("undo said %q", what)
	}
	if got, _ := os.ReadFile(e.Rules); string(got) != string(before) {
		t.Fatal("undo did not restore rules.toml")
	}
}

// AC9: applied the default way, no label before the apply time changes;
// time after it gets the template's labels.
func TestApplyTemplateLeavesHistory(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	zedDays(t, e, 30)
	a := open(t, e)
	from, now := a.nowMS()-31*libDayMS, a.nowMS()
	if got := codeMS(t, a, from, now); got != 0 {
		t.Fatalf("Zed is Code before the template: %d", got)
	}
	if _, err := a.ApplyTemplates([]string{"own"}, false); err != nil {
		t.Fatal(err)
	}
	if got := codeMS(t, a, from, now); got != 0 {
		t.Fatalf("template relabelled %d ms of history", got)
	}
	e.Seed(t, apptest.Span{ID: 3001, Start: "09:00", End: "10:00", Bundle: "dev.zed.Zed", App: "Zed", Title: "main.go", Day: 1})
	e.Clock = e.Clock.Add(48 * time.Hour)
	if got := codeMS(t, a, now, a.nowMS()); got != 3_600_000 {
		t.Fatalf("time after the apply: %d ms Code, want 1h", got)
	}
}

// AC9 --past: the plan's newly labelled time is what the apply changes.
func TestApplyTemplatePastMatchesPlan(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	zedDays(t, e, 10)
	a := open(t, e)
	p, err := a.PlanTemplates([]string{"own"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.PastNewMS != 10*3_600_000 {
		t.Fatalf("plan says %d ms newly labelled, want 10h", p.PastNewMS)
	}
	if b, _ := os.ReadFile(e.Rules); strings.Contains(string(b), "dev.zed.Zed") {
		t.Fatal("plan wrote rules.toml")
	}
	from, now := a.nowMS()-int64(StatsDays)*libDayMS, a.nowMS()
	if _, err := a.ApplyTemplates([]string{"own"}, true); err != nil {
		t.Fatal(err)
	}
	if got := codeMS(t, a, from, now); got != p.PastNewMS {
		t.Fatalf("apply labelled %d ms, plan said %d", got, p.PastNewMS)
	}
	for _, r := range a.Rules().Rules {
		if r.SinceMS != 0 {
			t.Fatalf("past apply wrote since on %s", r.Describe())
		}
	}
}

func TestPlanTemplateGroups(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.WriteConfig(t, "[billing]\nenabled = false\n")
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	p, err := a.PlanTemplates([]string{"client", "own"}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"billing": false, "focus": true}
	for _, g := range p.Groups {
		if g.On != want[g.Group] {
			t.Errorf("%s on = %v", g.Group, g.On)
		}
	}
	if _, err := a.PlanTemplates([]string{"student"}, false); err == nil {
		t.Fatal("unknown template planned")
	}
}

func TestApplyTemplatesNoDB(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	f := rulesfile.Files{Rules: e.Rules, Learned: e.Learned}
	p, err := ApplyTemplatesNoDB(f, config.Default(), []string{"own"}, e.Clock.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(e.Rules)
	if p.Empty() || !strings.Contains(string(b), "dev.zed.Zed") || !strings.Contains(string(b), "since = ") {
		t.Fatalf("not applied:\n%s", b)
	}
}
