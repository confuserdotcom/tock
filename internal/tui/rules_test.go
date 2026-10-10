package tui

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"tock/internal/apptest"
	"tock/internal/classify"
)

func TestRulesTabLists(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "5")
	mustContain(t, m, "5 rules", "rules.toml · 3 rules", "title='^tock │'", "· / tock", "Entertainment",
		"J/K reorder")
	mustNotContain(t, m, "learned.toml")
}

// Phase 7 criterion 2: a reorder in the rules tab persists to rules.toml.
func TestRulesTabReorderPersists(t *testing.T) {
	m, e := seeded(t)
	m = press(t, m, "5", "j", "K")
	mustContain(t, m, "moved rules.toml#2 up")
	rs, err := classify.Load(e.Rules, e.Learned)
	if err != nil {
		t.Fatal(err)
	}
	if rs.Rules[0].Category != "Code" || rs.Rules[1].Project != "tock" {
		t.Fatalf("rules.toml order = %s, %s", rs.Rules[0].Describe(), rs.Rules[1].Describe())
	}
	if m.rules.cursor != 0 {
		t.Errorf("cursor = %d, want it to follow the rule to 0", m.rules.cursor)
	}
	if l := line(t, m, "▸ 1"); !strings.Contains(l, "Code") {
		t.Errorf("first row = %q", l)
	}
	m = press(t, m, "K")
	mustContain(t, m, "already first")
	m = press(t, m, "G", "J")
	mustContain(t, m, "already last")
}

func TestRulesTabLearnedSection(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	if err := os.WriteFile(e.Learned, []byte("[[rule]]\napp = '^Mail$'\ncategory = \"Communication\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, e)
	m = press(t, m, "5")
	mustContain(t, m, "learned.toml · checked first · 1 rule", "L1", "rules.toml · 3 rules")
	m = press(t, m, "J")
	mustContain(t, m, "already last")
	m = press(t, m, "j", "K")
	mustContain(t, m, "already first")
}

func TestRulesTabEditor(t *testing.T) {
	m, e := seeded(t)
	var got []string
	m.exec = func(c *exec.Cmd, cb tea.ExecCallback) tea.Cmd {
		got = c.Args
		return func() tea.Msg { return cb(nil) }
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "nano -w")
	m = press(t, m, "5", "e")
	if want := []string{"nano", "-w", e.Rules}; !slices.Equal(got, want) {
		t.Fatalf("editor args = %v, want %v", got, want)
	}
	t.Setenv("EDITOR", "")
	m = press(t, m, "e")
	if len(got) != 2 || got[0] != "vi" {
		t.Fatalf("default editor args = %v", got)
	}
}
