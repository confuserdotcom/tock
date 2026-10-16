package classify

import (
	"strings"
	"testing"
)

const moduleRules = `
[[category]]
name = "Study"
kind = "deep"

[[category]]
name = "Fun"
kind = "distract"

[[project]]
name = "Algèbre 1"
category = "Study"
aliases = ["alg1"]

[[project]]
name = "Analyse 1"
category = "Study"
aliases = ["ana1", "Nombres réels"]

[[project]]
name = "Méthodologie"
category = "Study"
match = false

[[project]]
name = "tock"

[[rule]]
url = '^youtube\.com'
category = "Fun"
`

func TestFoldWords(t *testing.T) {
	for in, want := range map[string]string{
		"Algèbre 1 – TD": " algebre 1 td ", "ALGEBRE 1": " algebre 1 ", "": " ", "  Œuvre!! ": " oeuvre ",
		"Alge\u0300bre 1": " algebre 1 ",
	} {
		if got := foldWords(in); got != want {
			t.Errorf("foldWords(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNFC(t *testing.T) {
	if got := NFC("Alge\u0300bre 1 · Me\u0301canique · c\u0327a"); got != "Algèbre 1 · Mécanique · ça" {
		t.Fatalf("NFC = %q", got)
	}
	if got := NFC("\u0301x"); got != "\u0301x" {
		t.Fatalf("lone mark: %q", got)
	}
}

func TestModules(t *testing.T) {
	rs, err := Parse([]byte(moduleRules), []byte("[[rule]]\nurl = '^youtube'\ncategory = 'Fun'\n"))
	if err != nil {
		t.Fatal(err)
	}
	str := func(s string) *string { return &s }
	cases := []struct {
		v              SpanView
		cat, proj      string
		catSrc, prjSrc Source
	}{
		{SpanView{BundleID: "pdf", Title: str("Cours Complet ALGEBRE 1 Par Pr. Taous")}, "Study", "Algèbre 1", SourceModule, SourceModule},
		{SpanView{BundleID: "obs", Title: str("Cours 1 - Nombres reels - Licence - Obsidian")}, "Study", "Analyse 1", SourceModule, SourceModule},
		{SpanView{BundleID: "obs", Title: str("Analyse 12 notes")}, "", "", SourceNone, SourceNone},
		{SpanView{BundleID: "b", Title: str("Analyse 1 cours"), URL: str("youtube.com/watch")}, "Study", "Analyse 1", SourceModule, SourceModule},
		{SpanView{BundleID: "term", Title: str("zsh"), Dir: str("~/Licence/S1/Algèbre 1")}, "Study", "Algèbre 1", SourceModule, SourceModule},
		{SpanView{BundleID: "obs", Title: str("Méthodologie notes")}, "", "", SourceNone, SourceNone},
	}
	for _, c := range cases {
		l := rs.Label(c.v, nil)
		if l.Category != c.cat || l.Project != c.proj || l.CategorySource != c.catSrc || l.ProjectSource != c.prjSrc {
			t.Errorf("%q: got %+v", *c.v.Title, l)
		}
	}
	// A project-only manual tag takes the module's category.
	l := rs.Label(SpanView{BundleID: "x", Title: str("q")}, &Override{Project: "Analyse 1"})
	if l.Category != "Study" || l.Kind != KindDeep || l.CategorySource != SourceProject {
		t.Fatalf("inherit: %+v", l)
	}
	if rs.ModuleOf("ANA1") != "Analyse 1" || rs.ModuleOf("Méthodologie") != "Méthodologie" || rs.ModuleOf("tock") != "" || rs.ModuleOf("") != "" {
		t.Fatal("ModuleOf")
	}
}

func TestPlannedModule(t *testing.T) {
	rs, err := Parse([]byte(moduleRules+"[[rule]]\napp = 'Obsidian'\ncategory = 'Study'\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	str := func(s string) *string { return &s }
	v := SpanView{BundleID: "obs", App: "Obsidian", Title: str("TD 1 - Licence - Obsidian"), Planned: "Analyse 1"}
	if l := rs.Label(v, nil); l.Project != "Analyse 1" || l.ProjectSource != SourcePlan || l.Category != "Study" {
		t.Fatalf("planned: %+v", l)
	}
	// Not Study time: the plan does not apply.
	v = SpanView{BundleID: "b", App: "Arc", Title: str("x"), URL: str("youtube.com/x"), Planned: "Analyse 1"}
	if l := rs.Label(v, nil); l.Project != "" {
		t.Fatalf("planned fun: %+v", l)
	}
}

func TestModuleErrors(t *testing.T) {
	for _, c := range []struct{ rules, want string }{
		{moduleRules + "[[project]]\nname = 'X'\ncategory = 'Nope'\n", `project "X": unknown category "Nope"`},
		{moduleRules + "[[project]]\nname = 'Y'\naliases = ['--']\n", `project "Y": empty alias`},
	} {
		if _, err := Parse([]byte(c.rules), nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("err %v, want %q", err, c.want)
		}
	}
}

func TestMarkPlan(t *testing.T) {
	vs := []SpanView{{ID: 1, Start: 0, End: 100}, {ID: 2, Start: 100, End: 300}}
	got := MarkPlan(vs, []PlanSlot{{Interval{50, 150}, "A"}, {Interval{250, 400}, "B"}})
	want := []struct {
		id, s, e int64
		p        string
	}{{1, 0, 50, ""}, {1, 50, 100, "A"}, {2, 100, 150, "A"}, {2, 150, 250, ""}, {2, 250, 300, "B"}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, w := range want {
		if got[i].ID != w.id || got[i].Start != w.s || got[i].End != w.e || got[i].Planned != w.p {
			t.Errorf("piece %d = %+v, want %+v", i, got[i], w)
		}
	}
}
