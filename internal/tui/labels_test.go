package tui

import (
	"os"
	"strings"
	"testing"

	"tock/internal/apptest"
)

func TestLabelsTabManagesCategories(t *testing.T) {
	m, e := seeded(t)
	m = press(t, m, "7")
	mustContain(t, m, "Categories & modules", "● Code", "no category", "tock")

	// a: a new category, named, then its kind picked.
	m = press(t, m, "a")
	m = typeText(t, m, "Reading")
	m = press(t, m, "enter")
	mustContain(t, m, "Kind of Reading", "browse")
	m = clickOn(t, m, "neutral")
	m = clickOn(t, m, "neutral")
	mustContain(t, m, "added category Reading (neutral)", "● Reading")

	// K on Reading, then w: kind and afk window.
	for m.labels.tree[m.labels.rows()[m.labels.cursor].cat].Name != "Reading" {
		m = press(t, m, "j")
	}
	m = press(t, m, "K", "j", "enter")
	mustContain(t, m, "Reading is browse now")
	m = press(t, m, "w", "j", "j", "enter")
	mustContain(t, m, "Reading: afk 10m")

	// m: a module under it; r renames it.
	m = press(t, m, "m")
	m = typeText(t, m, "Camus")
	m = press(t, m, "enter")
	mustContain(t, m, "added module Camus under Reading", "Camus")
	m = press(t, m, "j", "e", "ctrl+u")
	m = typeText(t, m, "Novels")
	m = press(t, m, "enter")
	mustContain(t, m, "renamed Camus to Novels")
	m = press(t, m, "l")
	m = typeText(t, m, "roman, livre")
	m = press(t, m, "enter")
	mustContain(t, m, "roman, livre")

	b, err := os.ReadFile(e.Rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`name = "Reading"`, `kind = "browse"`, `afk = "10m"`, `name = "Novels"`, `category = "Reading"`, `aliases = ["roman", "livre"]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("rules.toml lacks %s:\n%s", want, b)
		}
	}

	// d deletes the module, then the empty category.
	m = press(t, m, "d", "j", "enter")
	mustContain(t, m, "deleted Novels")
	m = press(t, m, "k", "d", "j", "enter")
	mustContain(t, m, "deleted Reading")
	mustNotContain(t, m, "● Reading")
}

func TestLabelsDeleteInUseSaysMerge(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "7") // cursor on Code, which rules name
	m = press(t, m, "d", "j", "enter")
	mustContain(t, m, "Code is named by 1 rule or goal: merge it into another")
}

func TestLabelsChipsAreButtons(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "7")
	m = clickOn(t, m, "distract")
	if m.choose == nil || !strings.Contains(m.choose.title, "Kind of Entertainment") {
		t.Fatalf("choose = %+v", m.choose)
	}
	m = press(t, m, "esc")
	m = clickOn(t, m, "afk 5m")
	if m.choose == nil || !strings.Contains(m.choose.title, "AFK window of Code") {
		t.Fatalf("choose = %+v", m.choose)
	}
}

func TestPickerOffersModules(t *testing.T) {
	e := apptest.New(t, apptest.Rules+"\n[[project]]\nname = \"Analyse 1\"\ncategory = \"Study\"\naliases = [\"ana1\"]\n")
	e.Seed(t, apptest.DaySpans()...)
	m := newModel(t, e)
	m = clickOn(t, m, "XOR Equivalence Check")
	m = press(t, m, "t")
	m = typeText(t, m, "ana")
	mustContain(t, m, "Study / Analyse 1", "module · an")
	m = press(t, m, "enter")
	mustContain(t, m, "Study / Analyse 1")
	if r := m.today.selected(); r == nil || r.Label.Project != "Analyse 1" || r.Label.Category != "Study" {
		t.Fatalf("row %+v", r)
	}
	// p (project only) lists the row category's modules first.
	m = press(t, m, "p")
	if opts := m.picker.options(); len(opts) == 0 || opts[0].name != "Analyse 1" || opts[0].note != "module" {
		t.Fatalf("options %+v", opts)
	}
}

func TestCompactRows(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:00", End: "09:20", Bundle: "term", App: "WezTerm", Title: "a"},
		apptest.Span{ID: 2, Start: "09:20", End: "09:21", Bundle: "term", App: "WezTerm", Title: "b"},
		apptest.Span{ID: 3, Start: "09:21", End: "09:30", Bundle: "term", App: "WezTerm", Title: "c"},
		apptest.Span{ID: 4, Start: "09:30", End: "09:35", Bundle: "arc", App: "Arc", Title: "x", URL: "chatgpt.com/c/1"},
		apptest.Span{ID: 5, Start: "09:35", End: "09:40", Bundle: "arc", App: "Arc", Title: "y", URL: "labs.x.tv/lab/1"},
	)
	m := newModel(t, e)
	rs := m.today.rows()
	if len(rs) != 3 || rs[0].End-rs[0].Start != 30*60_000 || len(rs[0].IDs) != 3 || rs[0].Text != "a" {
		t.Fatalf("compact rows %+v", rs)
	}
	m = press(t, m, "c")
	if n := len(m.today.rows()); n != 5 {
		t.Fatalf("full rows = %d", n)
	}
}
