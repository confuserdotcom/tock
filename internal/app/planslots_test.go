package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/apptest"
)

const moduleRules = apptest.Rules + `
[[project]]
name = "Analyse 1"
category = "Study"
aliases = ["ana1"]

[[project]]
name = "Thermodynamique"
category = "Study"
aliases = ["thermo"]

[[rule]]
bundle = ["obs"]
category = "Study"
`

func TestPlanSlotsLabelStudyTime(t *testing.T) {
	e := apptest.New(t, moduleRules)
	todo := filepath.Join(t.TempDir(), "todo.txt")
	os.WriteFile(todo, []byte(
		"(A) Analyse 1 · TD 1 +ana1 @sam-09h00 due:2026-10-10\n"+
			"x 2026-10-10 Thermo · cours +thermo @10h00 due:2026-10-10\n"+
			"Lunch @12h00 due:2026-10-10\n"), 0o600)
	e.WriteConfig(t, "[plan]\nfile = '"+todo+"'\n")
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:10", End: "09:50", Bundle: "obs", App: "Obsidian", Title: "TD 1 - Licence - Obsidian"},
		apptest.Span{ID: 2, Start: "09:50", End: "10:30", Bundle: "obs", App: "Obsidian", Title: "Cours - Licence - Obsidian"},
		apptest.Span{ID: 3, Start: "10:30", End: "10:40", Bundle: "arc", App: "Arc", Title: "v", URL: "www.youtube.com/watch"},
		apptest.Span{ID: 4, Start: "12:10", End: "12:40", Bundle: "obs", App: "Obsidian", Title: "notes - Licence - Obsidian"},
	)
	a := open(t, e)
	d, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range d.Timeline {
		got = append(got, r.App+" "+r.Label.Category+"/"+r.Label.Project)
	}
	want := "Obsidian Study/Analyse 1|Obsidian Study/Analyse 1|Obsidian Study/Thermodynamique|Arc Entertainment/|Obsidian Study/"
	if strings.Join(got, "|") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, "|"), want)
	}
}
