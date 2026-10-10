package tui

import (
	"os"
	"strings"
	"testing"

	"tock/internal/apptest"
)

func TestInboxLists(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "3")
	mustContain(t, m, "Inbox · last 7 days · 3 keys · 28m unlabelled")
	if l := line(t, m, "chatgpt.com/c"); !strings.HasPrefix(l, "▸  12m   2 spans  Arc · chatgpt.com/c") || !strings.Contains(l, `"Proofs"`) {
		t.Fatalf("first item: %q", l)
	}
	mustContain(t, m, "10m    1 span  Mail · inbox")
}

// Phase 3 criterion 2: accepting a learned rule from the inbox writes it and
// the badge drops.
func TestInboxLearnWritesRuleAndBadgeDrops(t *testing.T) {
	m, e := seeded(t)
	m = press(t, m, "3", "enter")
	m = typeText(t, m, "stu")
	m = press(t, m, "enter", "enter")
	mustContain(t, m, "Learn a rule", "Arc · chatgpt.com/c", "->  Study",
		"matches  2 spans (12m): 2 this key, 0 other unlabelled (0s), 0 relabelled", "[y] accept")
	m = press(t, m, "y")
	mustContain(t, m, "learned: Study now covers 2 spans (12m)", "inbox ²", "2 keys · 16m unlabelled")
	body, err := os.ReadFile(e.Learned)
	if err != nil || !strings.Contains(string(body), "category = \"Study\"") {
		t.Fatalf("learned.toml: %s %v", body, err)
	}
	m = press(t, m, "1")
	if l := line(t, m, "11:26–11:35"); !strings.Contains(l, "Study") {
		t.Fatalf("today must show the learned label at once: %q", l)
	}
}

func TestLearnMoreSpecificAndCancel(t *testing.T) {
	m, e := seeded(t)
	m = press(t, m, "3", "enter")
	m = typeText(t, m, "stu")
	m = press(t, m, "enter", "enter")
	first := line(t, m, "->  Study")
	if !strings.Contains(m.footer(), "s more specific") {
		t.Fatalf("hint: %q", m.footer())
	}
	m = press(t, m, "s")
	if second := line(t, m, "->  Study"); second == first {
		t.Fatalf("s did not move on: %q", second)
	}
	m = press(t, m, "n")
	mustContain(t, m, "cancelled", "inbox ³")
	if _, err := os.Stat(e.Learned); !os.IsNotExist(err) {
		t.Fatal("cancel must not write learned.toml")
	}
}

func TestInboxTagOnce(t *testing.T) {
	m, e := seeded(t)
	m = press(t, m, "3", "o")
	m = typeText(t, m, "stu")
	m = press(t, m, "enter", "enter")
	mustContain(t, m, "tagged 2 spans (12m) → Study", "inbox ²")
	if _, err := os.Stat(e.Learned); !os.IsNotExist(err) {
		t.Fatal("o must not learn a rule")
	}
}

func TestInboxIgnoreAndUndo(t *testing.T) {
	m, _ := seeded(t)
	m = press(t, m, "3", "x")
	mustContain(t, m, "ignored: Arc · chatgpt.com/c (u undo)", "inbox ²")
	m = press(t, m, "u")
	mustContain(t, m, "showing again: Arc · chatgpt.com/c", "inbox ³")
	m = press(t, m, "u")
	mustContain(t, m, "inbox ³")
}

// T on a labelled today row: every candidate would relabel the other
// youtube spans that rules.toml labels Entertainment, so only tagging once
// is offered.
func TestNoSafeRuleOffersTagOnce(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:00", End: "09:10", Bundle: "arc", App: "Arc", Title: "A", URL: "www.youtube.com/watch"},
		apptest.Span{ID: 2, Start: "09:10", End: "09:20", Bundle: "arc", App: "Arc", Title: "B", URL: "www.youtube.com/watch"},
		apptest.Span{ID: 3, Start: "09:20", End: "09:30", Bundle: "arc", App: "Arc", Title: "Home", URL: "www.youtube.com"},
		apptest.Span{ID: 4, Start: "09:30", End: "09:40", Bundle: "chrome", App: "Chrome", Title: "C", URL: "www.youtube.com/watch"},
	)
	m := newModel(t, e)
	m = press(t, m, "g", "T")
	m = typeText(t, m, "stu")
	m = press(t, m, "enter", "enter")
	mustContain(t, m, "no safe rule", "[o] tag these 2 spans (20m) only")
	m = press(t, m, "y") // not offered
	mustContain(t, m, "no safe rule")
	m = press(t, m, "o")
	mustContain(t, m, "tagged 2 spans (20m) → Study")
	if l := line(t, m, "09:00–09:20"); !strings.Contains(l, "Study") {
		t.Fatalf("row: %q", l)
	}
	if _, err := os.Stat(e.Learned); !os.IsNotExist(err) {
		t.Fatal("no rule must be learned")
	}
}
