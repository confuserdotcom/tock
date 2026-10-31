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
	mustContain(t, m, "rules", "rules.toml · 3 rules", "title='^tock │'", "· / tock", "Entertainment",
		"e edit rules.toml")
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

// A refused reorder leaves the cursor on the rule it was on.
func TestRulesTabFailedMoveKeepsCursor(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	// A blank line inside a rule is a layout SwapRules refuses.
	e.WriteRules(t, strings.Replace(apptest.Rules, "bundle = [\"term\"]\ncategory = \"Code\"", "bundle = [\"term\"]\n\ncategory = \"Code\"", 1))
	m := newModel(t, e)
	m = press(t, m, "5", "j", "K")
	mustContain(t, m, "can't reorder this layout safely")
	if m.rules.cursor != 1 {
		t.Fatalf("cursor = %d after a failed move, want 1", m.rules.cursor)
	}
}

// quietRules is apptest.Rules with the youtube rule (the third) marked
// interrupt = false.
func quietRules(t *testing.T) string {
	t.Helper()
	out, err := classify.SetRuleField(classify.RulesFile, []byte(apptest.Rules), 2, "interrupt", "false")
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRulesQuietFlag(t *testing.T) {
	e := apptest.New(t, quietRules(t))
	e.Seed(t, apptest.DaySpans()...)
	m := press(t, newModel(t, e), "5")
	if l := line(t, m, "youtube"); !strings.HasSuffix(strings.TrimRight(l, " "), "quiet") {
		t.Fatalf("a marked rule row must end with quiet: %q", l)
	}
	for _, l := range strings.Split(screen(m), "\n") {
		if strings.Contains(l, "quiet") && !strings.Contains(l, "youtube") {
			t.Fatalf("an unmarked row says quiet: %q", l)
		}
	}
	mustNotContain(t, m, "Quiet", "non-interrupting")

	// On a narrow screen the flag goes first: the rule text stays whole.
	narrow := step(t, m, windowSize(70, 30))
	if l := line(t, narrow, "youtube"); strings.Contains(l, "quiet") || !strings.Contains(l, "youtube") {
		t.Fatalf("at 70 wide the flag must be dropped before the rule text is cut: %q", l)
	}
}
