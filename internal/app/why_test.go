package app

import (
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
)

func TestWhyRuleTagAndNothing(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	from, to := apptest.At(t, "00:00", 0), apptest.At(t, "23:59", 0)

	w, err := a.Why(1, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(w) != 2 {
		t.Fatalf("lines %+v", w)
	}
	if w[0].Field != "category" || w[0].Label != "Code" || w[0].Source != "rule" || w[0].Text != `rules.toml, rule 2: bundle term` {
		t.Errorf("category %+v", w[0])
	}
	if w[1].Field != "project" || w[1].Label != "tock" || w[1].Text != `rules.toml, rule 1: bundle term, title ~ ^tock │` {
		t.Errorf("project %+v", w[1])
	}
	if w[0].Ref != "rules.toml#2" || w[1].Ref != "rules.toml#1" {
		t.Errorf("refs %q %q", w[0].Ref, w[1].Ref)
	}

	if err := a.Tag([]int64{5}, classify.Want{Category: "Study"}); err != nil {
		t.Fatal(err)
	}
	if w, _ = a.Why(5, from, to); len(w) == 0 || w[0].Text != "your tag" || w[0].Label != "Study" {
		t.Errorf("tagged %+v", w)
	}

	w, err = a.Why(5_000, from, to)
	if err != nil || len(w) != 0 {
		t.Errorf("unknown id: %+v %v", w, err)
	}
}

func TestWhyText(t *testing.T) {
	rs, err := classify.Parse([]byte(`
[[category]]
name = "Code"
kind = "deep"

[[rule]]
url = 'github\.com/acme'
category = "Code"
since = 2026-11-20
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	for src, want := range map[classify.Source]string{
		classify.SourceManual:   "your tag",
		classify.SourceCall:     "a call was on",
		classify.SourceCalendar: "a calendar event",
		classify.SourceKept:     "kept when its title was deleted",
		classify.SourceModule:   "a module named in the title",
		classify.SourceProject:  "the project's category",
		classify.SourcePlan:     "your plan",
	} {
		if got := whyText(rs, src, ""); got != want {
			t.Errorf("%s: %q, want %q", src, got, want)
		}
	}
	if got := whyText(rs, classify.SourceRule, "rules.toml#1"); !strings.HasPrefix(got, `rules.toml, rule 1: site ~ github\.com/acme, from 2026-11-20`) {
		t.Errorf("dated rule: %q", got)
	}
	if got := whyText(rs, classify.SourceRule, "rules.toml#9"); got != "rules.toml, rule 9 (changed since)" {
		t.Errorf("missing rule: %q", got)
	}
}
