package focus

import (
	"testing"

	"tock/internal/classify"
)

// mark names view i's category and, when ref is set, the rule that set it.
func mark(in *Input, i int, cat string, src classify.Source, ref string, quiet bool) {
	in.Labels[i].Category, in.Labels[i].CategorySource = cat, src
	in.Labels[i].CategoryRule, in.Labels[i].Quiet = ref, quiet
}

func TestTargetFrontView(t *testing.T) {
	cases := []struct {
		name    string
		src     classify.Source
		ref     string
		wantRef string
	}{
		{"rule", classify.SourceRule, "rules.toml#3", "rules.toml#3"},
		{"learned", classify.SourceLearned, "learned.toml#2", "learned.toml#2"},
		{"module", classify.SourceModule, "module:x", ""},
		{"kept", classify.SourceKept, "rules.toml#3", ""},
		{"manual", classify.SourceManual, "", ""},
		{"call", classify.SourceCall, "", ""},
		{"calendar", classify.SourceCalendar, "", ""},
	}
	for _, c := range cases {
		in := input(at(14, 7, 0),
			vw{at(14, 0, 0), at(14, 5, 0), classify.KindDeep, "", ""},
			vw{at(14, 5, 0), at(14, 7, 0), classify.KindDistract, "", ""})
		mark(&in, 1, "Entertainment", c.src, c.ref, false)
		cat, ref := distractTarget(in, running())
		if cat != "Entertainment" || ref != c.wantRef {
			t.Errorf("%s: target = (%q, %q), want (Entertainment, %q)", c.name, cat, ref, c.wantRef)
		}
	}
}

func TestTargetFallback(t *testing.T) {
	// The front view is deep; 4 min Entertainment (rule #3) and 4 min Games
	// (rule #5) are both alertable: the tie goes to the name.
	build := func(refA, refB string) Input {
		in := input(at(14, 12, 0),
			vw{at(14, 0, 0), at(14, 4, 0), classify.KindDistract, "", ""},
			vw{at(14, 4, 0), at(14, 8, 0), classify.KindDistract, "", ""},
			vw{at(14, 8, 0), at(14, 12, 0), classify.KindDeep, "", ""})
		mark(&in, 0, "Games", classify.SourceRule, refB, false)
		mark(&in, 1, "Entertainment", classify.SourceRule, refA, false)
		mark(&in, 2, "Code", classify.SourceRule, "rules.toml#1", false)
		return in
	}
	cat, ref := distractTarget(build("rules.toml#3", "rules.toml#5"), running())
	if cat != "Entertainment" || ref != "rules.toml#3" {
		t.Fatalf("tie: target = (%q, %q), want (Entertainment, rules.toml#3)", cat, ref)
	}
	// More time wins over the name.
	in := build("rules.toml#3", "rules.toml#5")
	in.Views[0].Start = at(14, 0, 0)
	in.Views[0].End = at(14, 4, 30)
	in.Views[1].Start = at(14, 4, 30)
	cat, ref = distractTarget(in, running())
	if cat != "Games" || ref != "rules.toml#5" {
		t.Fatalf("most time: target = (%q, %q), want (Games, rules.toml#5)", cat, ref)
	}
	// Two rules share the category: no rule is offered.
	in = build("rules.toml#3", "rules.toml#5")
	mark(&in, 0, "Entertainment", classify.SourceRule, "rules.toml#4", false)
	cat, ref = distractTarget(in, running())
	if cat != "Entertainment" || ref != "" {
		t.Fatalf("two rules: target = (%q, %q), want (Entertainment, \"\")", cat, ref)
	}
	// One slice by a rule, one by hand: no rule either.
	in = build("rules.toml#3", "rules.toml#5")
	mark(&in, 0, "Entertainment", classify.SourceManual, "", false)
	if cat, ref = distractTarget(in, running()); cat != "Entertainment" || ref != "" {
		t.Fatalf("rule and manual: target = (%q, %q), want (Entertainment, \"\")", cat, ref)
	}
}

func TestTargetNone(t *testing.T) {
	in := input(at(14, 12, 0),
		vw{at(14, 0, 0), at(14, 6, 0), classify.KindDistract, "", ""},
		vw{at(14, 6, 0), at(14, 12, 0), classify.KindDeep, "", ""})
	mark(&in, 0, "Entertainment", classify.SourceRule, "rules.toml#3", true)
	if cat, ref := distractTarget(in, running()); cat != "" || ref != "" {
		t.Fatalf("all quiet: target = (%q, %q), want empty", cat, ref)
	}
	// A quiet view in front falls through to the other category.
	in = input(at(14, 12, 0),
		vw{at(14, 0, 0), at(14, 3, 0), classify.KindDistract, "", ""},
		vw{at(14, 3, 0), at(14, 12, 0), classify.KindDistract, "", ""})
	mark(&in, 0, "Games", classify.SourceRule, "rules.toml#5", false)
	mark(&in, 1, "Entertainment", classify.SourceRule, "rules.toml#3", true)
	if cat, ref := distractTarget(in, running()); cat != "Games" || ref != "rules.toml#5" {
		t.Fatalf("quiet front: target = (%q, %q), want (Games, rules.toml#5)", cat, ref)
	}
}

func TestTargetSkipsMeetingAndIdle(t *testing.T) {
	in := input(at(14, 12, 0),
		vw{at(14, 0, 0), at(14, 4, 0), classify.KindDistract, "us.zoom.xos", ""}, // meeting
		vw{at(14, 4, 0), at(14, 8, 0), "idle", "", ""},
		vw{at(14, 8, 0), at(14, 12, 0), classify.KindDeep, "", ""})
	mark(&in, 0, "Entertainment", classify.SourceRule, "rules.toml#3", false)
	if cat, ref := distractTarget(in, running()); cat != "" || ref != "" {
		t.Fatalf("meeting/idle: target = (%q, %q), want empty", cat, ref)
	}
}
