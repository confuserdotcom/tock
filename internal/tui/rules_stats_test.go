package tui

import (
	"strings"
	"testing"

	"tock/internal/apptest"
)

func TestRulesTabStats(t *testing.T) {
	e := apptest.New(t, apptest.Rules+"\n[[rule]]\nurl = '^never'\ncategory = \"Study\"\n\n[[rule]]\nbundle = [\"term\"]\ncategory = \"Study\"\n")
	e.Seed(t, apptest.DaySpans()...)
	m := press(t, newModel(t, e), "5")
	if l := line(t, m, "url='^never'"); !strings.Contains(l, "unused") {
		t.Fatalf("unused rule line: %q", l)
	}
	if l := line(t, m, "5    bundle=[term]"); !strings.Contains(l, "shadowed") {
		t.Fatalf("shadowed rule line: %q", l)
	}
	if l := line(t, m, "title='^tock │'"); !strings.Contains(l, "1h40") {
		t.Fatalf("used rule line: %q", l)
	}
}
