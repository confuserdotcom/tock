package app

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
)

const orderRules = `
[[category]]
name = "Code"
kind = "deep"

[[category]]
name = "Study"
kind = "deep"

# terminal
[[rule]]
bundle = ["term"]
category = "Code"

# tock titles
[[rule]]
title = '^tock'
category = "Study"
`

func TestMoveRuleRelabels(t *testing.T) {
	e := apptest.New(t, orderRules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	cat := func() string {
		rows, err := a.Export(e.Clock.Add(-6*time.Hour), e.Clock)
		if err != nil {
			t.Fatal(err)
		}
		return rows[0].Category
	}
	if got := cat(); got != "Code" {
		t.Fatalf("before: %q", got)
	}
	if err := a.MoveRule(classify.RulesFile, 2, true); err != nil {
		t.Fatal(err)
	}
	if got := cat(); got != "Study" {
		t.Fatalf("after: %q", got)
	}
	if a.Rules().Rules[0].Title != "^tock" {
		t.Fatalf("rules = %+v", a.Rules().Rules)
	}
	data, _ := os.ReadFile(e.Rules)
	if strings.Index(string(data), "# tock titles") > strings.Index(string(data), "# terminal") {
		t.Fatalf("file not reordered:\n%s", data)
	}
	if err := a.MoveRule(classify.RulesFile, 1, true); !errors.Is(err, classify.ErrRuleEdge) {
		t.Fatalf("edge: %v", err)
	}
	if err := a.MoveRule("other.toml", 1, true); err == nil {
		t.Fatal("unknown file accepted")
	}
	if a.RulePath(classify.LearnedFile) != e.Learned || a.RulePath(classify.RulesFile) != e.Rules {
		t.Fatal("RulePath")
	}
}
