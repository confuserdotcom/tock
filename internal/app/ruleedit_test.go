package app

import (
	"errors"
	"os"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
)

func refOf(r classify.Rule) RuleRef {
	return RuleRef{File: r.File, Index: r.Index, Fingerprint: r.Fingerprint()}
}

func TestPreviewRule(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	p, err := a.PreviewRule(nil, RuleSpec{URL: `chatgpt\.com`, Label: "study"})
	if err != nil {
		t.Fatal(err)
	}
	if p.MatchedMS != 12*60_000 || p.ChangedMS != 12*60_000 {
		t.Fatalf("matched %d changed %d, want 12m each", p.MatchedMS, p.ChangedMS)
	}
	if len(p.Examples) != 1 || p.Examples[0].Text != "chatgpt.com" || p.Examples[0].From != "" || p.Examples[0].To != "Study" || p.Examples[0].MS != 12*60_000 {
		t.Fatalf("examples %+v", p.Examples)
	}
	if b, _ := os.ReadFile(e.Rules); strings.Contains(string(b), "chatgpt") {
		t.Fatal("preview wrote rules.toml")
	}
}

func TestAddEditRemoveRule(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if err := a.AddRule(RuleSpec{URL: `chatgpt\.com`, Label: "Study"}); err != nil {
		t.Fatal(err)
	}
	r := a.Rules().Rules[0]
	if r.URL != `chatgpt\.com` || r.Category != "Study" || r.File != classify.RulesFile || r.Index != 1 {
		t.Fatalf("first rule %+v", r)
	}
	if err := a.EditRule(refOf(r), RuleSpec{URL: `chatgpt\.com`, Title: "Proof", Label: "Code/tock"}); err != nil {
		t.Fatal(err)
	}
	r = a.Rules().Rules[0]
	if r.Title != "Proof" || r.Category != "Code" || r.Project != "tock" {
		t.Fatalf("edited %+v", r)
	}
	if err := a.EditRule(RuleRef{File: r.File, Index: 1, Fingerprint: "stale"}, RuleSpec{URL: "x", Label: "Code"}); !errors.Is(err, ErrRuleChanged) {
		t.Fatalf("stale fingerprint: %v", err)
	}
	if err := a.RemoveRule(refOf(r)); err != nil {
		t.Fatal(err)
	}
	if len(a.Rules().Rules) != 3 {
		t.Fatalf("rules after remove: %d", len(a.Rules().Rules))
	}
}

func TestRuleSpecChecks(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	for _, s := range []RuleSpec{
		{Label: "Code"},             // nothing to match
		{App: "Zed"},                // no label
		{App: "Zed", Label: "Nope"}, // unknown category
		{URL: "([", Label: "Code"},  // bad regex
	} {
		if err := a.AddRule(s); err == nil {
			t.Errorf("AddRule(%+v) = nil", s)
		}
	}
}

func TestRemoveLearnedRuleUndo(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	learned := "# learned\n[[rule]]\nurl = 'labs\\.x\\.tv'\ncategory = \"Study\"\n"
	if err := os.WriteFile(e.Learned, []byte(learned), 0o600); err != nil {
		t.Fatal(err)
	}
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if len(a.Rules().Learned) != 1 {
		t.Fatalf("learned = %+v", a.Rules().Learned)
	}
	err := a.WithRulesUndo("removed a learned rule", func() error { return a.RemoveRule(refOf(a.Rules().Learned[0])) })
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Rules().Learned) != 0 {
		t.Fatal("learned rule still there")
	}
	if _, err := a.Undo(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(e.Learned); string(b) != learned {
		t.Fatalf("learned.toml after undo:\n%s", b)
	}
}
