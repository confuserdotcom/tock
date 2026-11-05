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
	mustContain(t, m, "28m unlabelled · 3 keys · last 7 days")
	if l := line(t, m, "chatgpt.com"); !strings.HasPrefix(l, "▸  12m   2 spans  Arc · chatgpt.com") || !strings.Contains(l, `"Proofs"`) {
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
	mustContain(t, m, "Learn a rule", "Arc · chatgpt.com", "->  Study",
		"matches  2 spans (12m): 2 this key, 0 other unlabelled (0s), 0 relabelled", "[y] accept")
	m = press(t, m, "y")
	mustContain(t, m, "learned: Study now covers 2 spans (12m)", "inbox ²", "16m unlabelled · 2 keys")
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
	mustContain(t, m, "ignored: Arc · chatgpt.com   u undo", "inbox ²")
	m = press(t, m, "u")
	mustContain(t, m, "undid: ignore Arc · chatgpt.com", "inbox ³")
	m = press(t, m, "u")
	mustContain(t, m, "inbox ³")
}

// T on a labelled today row: every candidate would relabel the other
// terminal span that rules.toml labels Code, so only tagging once is
// offered.
func TestNoSafeRuleOffersTagOnce(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:00", End: "09:10", Bundle: "term", App: "Term", Title: "build 1"},
		apptest.Span{ID: 2, Start: "09:10", End: "09:20", Bundle: "term", App: "Term", Title: "build 2"},
		apptest.Span{ID: 3, Start: "09:20", End: "09:25", Bundle: "term", App: "Term", Title: "build log"},
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

// TestInboxNoiseFloor: keys under [inbox] min_minutes leave the list; the
// header counts them.
func TestInboxNoiseFloor(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:00", End: "09:10", Bundle: "arc", App: "Arc", Title: "Noctie", URL: "app.noctie.ai/x"},
		apptest.Span{ID: 2, Start: "09:10", End: "09:11", Bundle: "arc", App: "Arc", Title: "Kitea", URL: "kitea.com/x"},
		apptest.Span{ID: 3, Start: "09:11", End: "09:12", Bundle: "arc", App: "Arc", Title: "Steel", URL: "steelcase.com/x"},
		apptest.Span{ID: 4, Start: "09:12", End: "09:30", Bundle: "term", App: "Term", Title: "x"},
	)
	m := newModel(t, e)
	m = press(t, m, "3")
	mustContain(t, m, "10m unlabelled · 1 key · 2 small (2m) hidden · last 7 days", "inbox ¹")
	if strings.Contains(m.View().Content, "kitea") {
		t.Fatal("small key listed")
	}
}

func TestInboxFirstRun(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t)
	m := press(t, newModel(t, e), "3")
	mustContain(t, m, "Nothing recorded yet.")
	mustNotContain(t, m, "Inbox zero")
}
