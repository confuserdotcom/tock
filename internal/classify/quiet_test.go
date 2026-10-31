package classify

import (
	"strings"
	"testing"
)

const quietFixture = `
[[category]]
name = "Study"
kind = "deep"

[[category]]
name = "Social"
kind = "distract"
interrupt = false

[[category]]
name = "Video"
kind = "distract"

[[category]]
name = "Games"
kind = "distract"
interrupt = true

[[rule]]
bundle = ["quiet.rule"]
category = "Video"
interrupt = false

[[rule]]
bundle = ["loud.rule"]
category = "Video"

[[rule]]
bundle = ["in.social"]
category = "Social"

[[rule]]
bundle = ["in.games"]
category = "Games"
`

const modelRulesText = `
[[category]]
name = "Study"
kind = "deep"

[[category]]
name = "Entertainment"
kind = "distract"

[[category]]
name = "Code"
kind = "deep"

[[project]]
name = "Thermo"
category = "Study"

[[rule]]
bundle = ["chromium"]
category = "Entertainment"

[[rule]]
bundle = ["wez"]
title = 'nvim'
category = "Code"
`

func quietView(bundle string) SpanView {
	title := "t"
	return SpanView{ID: 1, End: 60_000, BundleID: bundle, App: bundle, Title: &title}
}

func TestInterruptKeyAccepted(t *testing.T) {
	rs := mustParse(t, quietFixture, "[[rule]]\nbundle = [\"learned.quiet\"]\ncategory = \"Video\"\ninterrupt = false\n\n[[rule]]\nbundle = [\"learned.loud\"]\ncategory = \"Video\"\n")
	quietCats := map[string]bool{"Study": false, "Social": true, "Video": false, "Games": false}
	for _, c := range rs.Categories {
		if c.Quiet != quietCats[c.Name] {
			t.Errorf("category %s Quiet = %v, want %v", c.Name, c.Quiet, quietCats[c.Name])
		}
	}
	wantRules := []bool{true, false, false, false}
	for i, r := range rs.Rules {
		if r.Quiet != wantRules[i] {
			t.Errorf("rules.toml#%d Quiet = %v, want %v", r.Index, r.Quiet, wantRules[i])
		}
	}
	if len(rs.Learned) != 2 || !rs.Learned[0].Quiet || rs.Learned[1].Quiet {
		t.Errorf("learned Quiet = %+v", rs.Learned)
	}

	_, err := Parse([]byte(strings.Replace(quietFixture, "interrupt = false\n\n[[rule]]\nbundle = [\"loud", "interupt = false\n\n[[rule]]\nbundle = [\"loud", 1)), nil)
	if err == nil || !strings.Contains(err.Error(), "rules.toml") || !strings.Contains(err.Error(), "interupt") {
		t.Errorf("typo err = %v, want unknown key naming rules.toml", err)
	}
	_, err = Parse([]byte(quietFixture), []byte("[[rule]]\nbundle = [\"x\"]\ncategory = \"Video\"\ninterupt = false\n"))
	if err == nil || !strings.Contains(err.Error(), "learned.toml") {
		t.Errorf("learned typo err = %v, want unknown key naming learned.toml", err)
	}
}

func TestLabelQuietFromCategory(t *testing.T) {
	rs := mustParse(t, quietFixture, "")
	l := rs.Label(quietView("in.social"), nil)
	if !l.Quiet || l.Category != "Social" || l.Kind != KindDistract {
		t.Fatalf("label = %+v, want quiet Social distract", l)
	}
	if l := rs.Label(quietView("in.games"), nil); l.Quiet || l.Category != "Games" {
		t.Fatalf("interrupt = true label = %+v, want not quiet", l)
	}
	if l := rs.Label(quietView("unknown.app"), nil); l.Quiet {
		t.Fatalf("unlabelled view quiet: %+v", l)
	}
}

func TestLabelQuietFromRule(t *testing.T) {
	rs := mustParse(t, quietFixture, "")
	q := rs.Label(quietView("quiet.rule"), nil)
	if !q.Quiet || q.Category != "Video" || q.Kind != KindDistract {
		t.Fatalf("marked rule label = %+v, want quiet Video", q)
	}
	if l := rs.Label(quietView("loud.rule"), nil); l.Quiet || l.Category != "Video" {
		t.Fatalf("unmarked rule label = %+v, want not quiet Video", l)
	}
}

func TestLabelQuietLearned(t *testing.T) {
	rs := mustParse(t, quietFixture, "[[rule]]\nbundle = [\"learned.quiet\"]\ncategory = \"Video\"\ninterrupt = false\n")
	l := rs.Label(quietView("learned.quiet"), nil)
	if !l.Quiet || l.CategorySource != SourceLearned {
		t.Fatalf("learned label = %+v, want quiet learned", l)
	}
}

func TestLabelQuietOverrides(t *testing.T) {
	rs := mustParse(t, quietFixture, "")
	// a manual override to a marked category is quiet by the category's mark
	l := rs.Label(quietView("loud.rule"), &Override{Category: "Social"})
	if !l.Quiet || l.CategorySource != SourceManual {
		t.Fatalf("manual to marked = %+v, want quiet", l)
	}
	// a manual override away from a marked rule's category is not quiet
	l = rs.Label(quietView("quiet.rule"), &Override{Category: "Games"})
	if l.Quiet || l.Category != "Games" {
		t.Fatalf("manual to unmarked = %+v, want not quiet", l)
	}
	// the rule's mark does not leak onto the call category
	rs = mustParse(t, quietFixture+"\n[calls]\ncategory = \"Video\"\n", "")
	v := quietView("quiet.rule")
	v.InCall = true
	l = rs.Label(v, nil)
	if l.CategorySource != SourceCall || l.Quiet {
		t.Fatalf("call label = %+v, want call source and not quiet", l)
	}
	// a kept category is quiet only through the category
	v = quietView("quiet.rule")
	v.Kept = &Kept{Category: "Video"}
	if l := rs.Label(v, nil); l.CategorySource != SourceKept || l.Quiet {
		t.Fatalf("kept label = %+v, want kept and not quiet", l)
	}
	v.Kept = &Kept{Category: "Social"}
	if l := rs.Label(v, nil); l.CategorySource != SourceKept || !l.Quiet {
		t.Fatalf("kept to marked = %+v, want quiet", l)
	}
	// idle spans stay empty
	idle := SpanView{ID: 2, End: 1, BundleID: "tock.idle"}
	if l := rs.Label(idle, nil); l.Quiet {
		t.Fatalf("idle label quiet: %+v", l)
	}
}

func TestQuietKeepsLabelFields(t *testing.T) {
	marked := mustParse(t, quietFixture, "")
	plain := mustParse(t, strings.ReplaceAll(quietFixture, "interrupt = false\n", ""), "")
	for _, b := range []string{"quiet.rule", "loud.rule", "in.social", "in.games"} {
		a, p := marked.Label(quietView(b), nil), plain.Label(quietView(b), nil)
		a.Quiet = false
		if a != p {
			t.Errorf("%s: marking changed the label: %+v vs %+v", b, a, p)
		}
	}
}

func TestApplyModelQuiet(t *testing.T) {
	ex := []Example{
		{View: tv("zathura", "Thermodynamique MPSI.pdf", 0), Category: "Study", Weight: 3},
		{View: tv("obsidian", "Exercice thermodynamique - TD", 0), Category: "Study", Weight: 3},
		{View: tv("obsidian", "Exercice 3 de thermodynamique", 0), Category: "Study", Weight: 3},
		{View: tv("chromium", "Funny cats - YouTube", 0), Category: "Entertainment", Weight: 3},
		{View: tv("wez", "nvim main.go", 0), Category: "Code", Weight: 3},
	}
	run := func(mark string) Label {
		rs := mustParse(t, strings.Replace(modelRulesText, `name = "Study"
kind = "deep"
`, `name = "Study"
kind = "deep"
`+mark, 1), "")
		views := []SpanView{tv("zathura", "Exercice thermodynamique PCSI.pdf", 60_000)}
		labels := rs.LabelAll(views, nil)
		rs.ApplyModel(Train(ex, nil), views, labels)
		if labels[0].Category != "Study" || labels[0].CategorySource != SourceModel {
			t.Fatalf("model did not fill: %+v", labels[0])
		}
		return labels[0]
	}
	if l := run("interrupt = false\n"); !l.Quiet {
		t.Errorf("model into marked category = %+v, want quiet", l)
	}
	if l := run(""); l.Quiet {
		t.Errorf("model into unmarked category = %+v, want not quiet", l)
	}
}

func TestRuleByRefAndFingerprint(t *testing.T) {
	rs := mustParse(t, quietFixture, "[[rule]]\nbundle = [\"learned.one\"]\ncategory = \"Video\"\n")
	r, ok := rs.RuleByRef("rules.toml#2")
	if !ok || r.File != RulesFile || r.Index != 2 || r.Bundle[0] != "loud.rule" {
		t.Fatalf("rules.toml#2 = %+v %v", r, ok)
	}
	if r, ok := rs.RuleByRef("learned.toml#1"); !ok || r.Bundle[0] != "learned.one" {
		t.Fatalf("learned.toml#1 = %+v %v", r, ok)
	}
	if _, ok := rs.RuleByRef("rules.toml#99"); ok {
		t.Fatal("rules.toml#99 found")
	}
	if _, ok := rs.RuleByRef("garbage"); ok {
		t.Fatal("garbage ref found")
	}

	if got, want := r.Fingerprint(), "bundle=[loud.rule] -> Video"; got != want {
		t.Fatalf("fingerprint = %q, want %q", got, want)
	}
	other := mustParse(t, strings.Replace(quietFixture, "bundle = [\"loud.rule\"]\ncategory = \"Video\"", "bundle = [\"loud.rule\"]\ncategory = \"Social\"", 1), "")
	o, _ := other.RuleByRef("rules.toml#2")
	if o.Describe() != r.Describe() || o.Fingerprint() == r.Fingerprint() {
		t.Fatalf("fingerprint must differ when only the category changes: %q vs %q", o.Fingerprint(), r.Fingerprint())
	}
}
